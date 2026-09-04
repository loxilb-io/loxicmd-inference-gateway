/*
 * Copyright (c) 2025 LoxiLB Authors
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
package lifecycle

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// buildCLI builds the actual loxicmd binary once for the exit-status tests.
// Returning an error from RunE is only half the contract: what automation sees
// is the process exit status, and that is decided by main and cobra together.
func buildCLI(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping packaged-binary test in short mode")
	}
	if runtime.GOOS != "linux" {
		t.Skip("loxicmd links Linux netlink; the packaged binary only builds there")
	}
	binary := filepath.Join(t.TempDir(), "loxicmd")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI failed: %v\n%s", err, out)
	}
	return binary
}

// runCLI runs the binary against the fake gateway and returns its exit status.
func runCLI(t *testing.T, binary string, gw *fakeGateway, args ...string) (int, string, string) {
	t.Helper()
	options := gw.options()
	full := append([]string{
		"-s", options.ServerIP,
		"-p", strconv.Itoa(options.ServerPort),
	}, args...)
	cmd := exec.Command(binary, full...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	status := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		status = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running the CLI failed: %v", err)
	}
	return status, stdout.String(), stderr.String()
}

// TestPackagedBinaryExitStatus proves the whole chain: a gateway failure
// reaches the shell as a non-zero exit status, and a success as zero. Every
// one of these invocations exited 0 before the lifecycle commands returned
// their errors.
func TestPackagedBinaryExitStatus(t *testing.T) {
	binary := buildCLI(t)

	t.Run("success exits zero", func(t *testing.T) {
		gw := newFakeGateway(t, jsonResponse(http.StatusOK, durablePersistBody))
		status, stdout, stderr := runCLI(t, binary, gw, "create", "persist")
		if status != 0 {
			t.Fatalf("status = %d\nstdout: %s\nstderr: %s", status, stdout, stderr)
		}
		if !strings.Contains(stdout, "Configuration persisted to") {
			t.Fatalf("unexpected stdout: %s", stdout)
		}
	})

	for name, tc := range map[string]struct {
		status int
		body   string
		args   []string
	}{
		"persist rejected":  {http.StatusConflict, `{"message":"busy"}`, []string{"create", "persist"}},
		"persist not ok":    {http.StatusOK, `{"result":"failed"}`, []string{"create", "persist"}},
		"persist undecoded": {http.StatusOK, `not json`, []string{"create", "persist"}},
		"save --api failed": {http.StatusServiceUnavailable, `{"message":"Maintenance mode"}`, []string{"save", "--api"}},
		"snapshot rejected": {http.StatusInternalServerError, `{"message":"capture failed"}`, []string{"get", "snapshot"}},
	} {
		t.Run(name+" exits non-zero", func(t *testing.T) {
			gw := newFakeGateway(t, jsonResponse(tc.status, tc.body))
			status, stdout, stderr := runCLI(t, binary, gw, tc.args...)
			if status == 0 {
				t.Fatalf("failure exited 0\nstdout: %s\nstderr: %s", stdout, stderr)
			}
			if !strings.Contains(stderr, "Error:") {
				t.Fatalf("failure printed nothing on stderr: %q", stderr)
			}
		})
	}

	t.Run("restore of a corrupt document exits non-zero", func(t *testing.T) {
		gw := newFakeGateway(t, jsonResponse(http.StatusOK, dryRunBody))
		path := filepath.Join(t.TempDir(), "snapshot.json")
		if err := os.WriteFile(path, []byte("{truncated"), 0600); err != nil {
			t.Fatal(err)
		}
		status, _, stderr := runCLI(t, binary, gw, "create", "restore", "-f", path)
		if status == 0 {
			t.Fatal("an unusable document exited 0")
		}
		if !strings.Contains(stderr, "not valid JSON") {
			t.Fatalf("unexpected stderr: %s", stderr)
		}
	})

	t.Run("json failure envelope reaches stdout with a non-zero status", func(t *testing.T) {
		gw := newFakeGateway(t, jsonResponse(http.StatusConflict, `{"message":"busy"}`))
		status, stdout, _ := runCLI(t, binary, gw, "create", "persist", "-o", "json")
		if status == 0 {
			t.Fatal("failure exited 0")
		}
		var report map[string]any
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("stdout is not JSON (%v): %s", err, stdout)
		}
		if report["reason"] != "operation-in-progress" || report["result"] != "error" {
			t.Fatalf("unexpected envelope: %s", stdout)
		}
	})

	t.Run("save --api --all is refused before anything runs", func(t *testing.T) {
		gw := newFakeGateway(t, jsonResponse(http.StatusOK, durablePersistBody))
		status, _, stderr := runCLI(t, binary, gw, "save", "--api", "--all")
		if status == 0 {
			t.Fatal("the combination was accepted")
		}
		if !strings.Contains(stderr, "--all") {
			t.Fatalf("unexpected stderr: %s", stderr)
		}
		if len(gw.requests) != 0 {
			t.Fatalf("a refused combination still called the gateway: %+v", gw.requests)
		}
	})

	t.Run("a port above the old int16 limit is usable", func(t *testing.T) {
		// The CLI used to declare --port as an int16, so any gateway
		// listening above 32767 - the whole ephemeral range - was
		// unreachable.
		gw := newFakeGateway(t, jsonResponse(http.StatusOK, durablePersistBody))
		if gw.options().ServerPort <= 32767 {
			t.Skip("the test server did not land above the old limit")
		}
		status, stdout, stderr := runCLI(t, binary, gw, "create", "persist")
		if status != 0 {
			t.Fatalf("status = %d\nstdout: %s\nstderr: %s", status, stdout, stderr)
		}
	})
}
