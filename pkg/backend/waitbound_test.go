/*
 * Copyright (c) 2026 LoxiLB Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A backend that spawns a child is the normal case, not an exotic one: the
// real host backend shells out to tar, pg_dump, systemctl and journalctl. The
// child inherits the stdout/stderr pipes this package hands the backend, so
// nothing about the invocation can be bounded by killing the direct process
// alone -- Wait also waits for the pipes to close.
//
// These tests are the reason pkg/backend kills the process GROUP and carries a
// WaitDelay. Each one is written so a regression FAILS the suite rather than
// hanging it: the invocation runs on a goroutine and the assertion is a select
// against a timer.

// backendScript installs a fake backend and points this package at it.
func backendScript(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "loxilb-appliance-backend")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	saved := executablePath
	executablePath = path
	t.Cleanup(func() { executablePath = saved })
}

// runBounded runs fn and reports whether it finished within limit.
func runBounded(t *testing.T, limit time.Duration, fn func()) bool {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
		return true
	case <-time.After(limit):
		return false
	}
}

// TestInvokeIsBoundedWhenTheBackendLeavesAChildHoldingThePipes covers the case
// that needs no deadline at all: the backend exits promptly but backgrounds a
// child, which keeps the inherited stdout open. Without a WaitDelay, Wait
// blocks on that pipe for as long as the grandchild lives -- forever, for a
// daemonized one.
func TestInvokeIsBoundedWhenTheBackendLeavesAChildHoldingThePipes(t *testing.T) {
	backendScript(t, "echo started\nsleep 30 &\nexit 0\n")

	start := time.Now()
	finished := runBounded(t, 15*time.Second, func() {
		//nolint:errcheck // the verdict under test is that this RETURNS.
		_, _ = Invoke(context.Background(), "status", false)
	})
	if !finished {
		t.Fatal("Invoke never returned: the backend exited but its child still holds " +
			"the stdout pipe, so cmd.Wait blocks. A backend that daemonizes anything " +
			"would hang the CLI with no deadline to rescue it.")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Invoke took %s; the wait must be bounded well inside the child's lifetime", elapsed)
	}
}

// TestInvokeHonorsTheDeadlineWhenTheBackendForks is the --timeout contract:
// requestContext derives the child context from the global --timeout precisely
// so a wedged backend cannot hang automation. exec.CommandContext cancels by
// killing the direct child only, so a backend that forks and waits survives
// its own death through the grandchild's grip on the pipes.
func TestInvokeHonorsTheDeadlineWhenTheBackendForks(t *testing.T) {
	backendScript(t, "echo started\nsleep 30\n")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	finished := runBounded(t, 15*time.Second, func() {
		//nolint:errcheck // the verdict under test is that this RETURNS.
		_, _ = Invoke(ctx, "status", false)
	})
	if !finished {
		t.Fatal("Invoke outlived its context: --timeout does not bound an invocation " +
			"whose backend forked a child, which is every real backend that shells out")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Invoke returned after %s for a 1s deadline", elapsed)
	}
}

// TestInvokeLeavesNoOrphanStillMutatingTheHost is the safety half. Bounding the
// wait is not enough on its own: a tar or pg_dump left running after the CLI
// has reported failure keeps writing the very artifact the caller was just told
// did not happen. Killing the process group is what makes the reported outcome
// true.
func TestInvokeLeavesNoOrphanStillMutatingTheHost(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "orphan-still-running")
	// The grandchild waits, then proves it outlived the invocation by writing
	// the marker. If the group was killed it never gets to.
	backendScript(t, "echo started\n(sleep 3; touch "+marker+") &\nsleep 30\n")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if !runBounded(t, 15*time.Second, func() {
		//nolint:errcheck // bounded return is asserted by the tests above.
		_, _ = Invoke(ctx, "status", false)
	}) {
		t.Fatal("Invoke never returned")
	}

	// Outlive the grandchild's own timer before judging.
	time.Sleep(5 * time.Second)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a backend grandchild outlived the cancelled invocation and kept " +
			"running: an orphaned tar/pg_dump can still be mutating host state " +
			"after the CLI has told the caller the operation failed")
	}
}
