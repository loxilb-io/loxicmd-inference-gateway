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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

// validContract is a handshake document that satisfies every schema rule.
const validContract = `{"apiVersion":"loxilb.io/appliance-backend/v1","kind":"BackendContract",` +
	`"backendVersion":"0.1.0","productRelease":"v0.9.8.9-rc.1","schemaVersion":1,` +
	`"commands":[{"name":"status","readOnly":true,"capabilities":["json-output"]},` +
	`{"name":"network validate","readOnly":true,"capabilities":[]}]}`

// fakeBackend writes an executable script standing in for the host backend
// and points the adapter at it. The script records its argv and environment
// so the tests can prove what the adapter actually spawned.
func fakeBackend(t *testing.T, script string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	path := filepath.Join(dir, "loxilb-appliance-backend")
	full := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"" + dir + "/argv\"\n" +
		"env > \"" + dir + "/env\"\n" +
		script
	if err := os.WriteFile(path, []byte(full), 0o755); err != nil {
		t.Fatal(err)
	}
	setExecutablePath(t, path)
	return dir
}

func requireCLIError(t *testing.T, err error, code exitcode.Code, componentCode string) *exitcode.CLIError {
	t.Helper()
	var ce *exitcode.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error is not classified: %v", err)
	}
	if ce.Code != code || ce.ComponentCode != componentCode {
		t.Fatalf("classified as %d/%q, want %d/%q (%v)", ce.Code, ce.ComponentCode, code, componentCode, err)
	}
	return ce
}

func TestAbsentBackendIsUnavailable(t *testing.T) {
	setExecutablePath(t, filepath.Join(t.TempDir(), "no-such-backend"))
	_, err := Invoke(context.Background(), "status", false)
	ce := requireCLIError(t, err, exitcode.Unavailable, CodeBackendUnavailable)
	if ce.Origin != "backend" {
		t.Fatalf("origin = %q, want backend", ce.Origin)
	}
	if _, herr := Handshake(context.Background()); herr == nil {
		t.Fatal("handshake against an absent backend succeeded")
	}
}

func TestInvokeSpawnsAllowlistedArgvWithScrubbedEnv(t *testing.T) {
	dir := fakeBackend(t, `echo '{"ok":true}'`)
	// A canary in the caller's environment must never reach the child:
	// the invocation contract fixes the child environment CLI-side.
	t.Setenv("LOXICMD_TEST_CANARY", "must-not-be-forwarded")

	res, err := Invoke(context.Background(), "network validate", true)
	if err != nil {
		t.Fatalf("invoke failed: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	got := strings.Split(strings.TrimSpace(string(argv)), "\n")
	want := []string{"network", "validate", "--correlation-id", res.CorrelationID, "--json"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("spawned argv %q, want %q", got, want)
	}
	if !strings.HasPrefix(res.CorrelationID, "cli-") || len(res.CorrelationID) != len("cli-")+16 {
		t.Fatalf("correlation id %q is not the documented shape", res.CorrelationID)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	if strings.Contains(string(env), "LOXICMD_TEST_CANARY") {
		t.Fatalf("the caller's environment leaked into the backend:\n%s", env)
	}
}

func TestInvokeRejectsUnlistedSubcommandBeforeSpawn(t *testing.T) {
	dir := fakeBackend(t, "")
	_, err := Invoke(context.Background(), "backup create", false)
	requireCLIError(t, err, exitcode.ContractMismatch, "argv-not-allowlisted")
	if _, statErr := os.Stat(filepath.Join(dir, "argv")); !os.IsNotExist(statErr) {
		t.Fatal("a rejected subcommand still spawned the backend")
	}
}

func TestInvokePreservesStreamsAndExit(t *testing.T) {
	fakeBackend(t, "echo partial-out\necho boom >&2\nexit 3")
	res, err := Invoke(context.Background(), "status", false)
	if err != nil {
		t.Fatalf("a backend refusal must be a result, not an exec error: %v", err)
	}
	if res.ExitCode != 3 || !strings.Contains(string(res.Stdout), "partial-out") ||
		!strings.Contains(string(res.Stderr), "boom") {
		t.Fatalf("streams/exit not preserved: %+v", res)
	}
}

func TestHandshakeAcceptsAValidContract(t *testing.T) {
	fakeBackend(t, fmt.Sprintf("echo '%s'", validContract))
	contract, err := Handshake(context.Background())
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	if contract.BackendVersion != "0.1.0" || contract.SchemaVersion != 1 {
		t.Fatalf("contract fields lost: %+v", contract)
	}
	if !contract.Supports("status") || !contract.Supports("network validate") {
		t.Fatalf("advertised commands lost: %+v", contract.Commands)
	}
	if contract.Supports("backup create") {
		t.Fatal("an unadvertised command reported as supported")
	}
}

func TestHandshakeRefusesContractViolations(t *testing.T) {
	for name, tc := range map[string]struct {
		document      string
		componentCode string
	}{
		"not json": {"contract says hello", "contract-invalid"},
		"wrong kind": {strings.Replace(validContract, `"BackendContract"`, `"SomethingElse"`, 1),
			"contract-invalid"},
		"unknown field": {strings.Replace(validContract, `"schemaVersion":1`, `"schemaVersion":1,"extra":true`, 1),
			"contract-invalid"},
		"zero schema version": {strings.Replace(validContract, `"schemaVersion":1`, `"schemaVersion":0`, 1),
			"contract-invalid"},
		"malformed command name": {strings.Replace(validContract, `"name":"status"`, `"name":"Status!"`, 1),
			"contract-invalid"},
		"unsupported major": {strings.Replace(validContract, "appliance-backend/v1", "appliance-backend/v2", 1),
			"contract-major-unsupported"},
	} {
		t.Run(name, func(t *testing.T) {
			fakeBackend(t, fmt.Sprintf("echo '%s'", tc.document))
			_, err := Handshake(context.Background())
			requireCLIError(t, err, exitcode.ContractMismatch, tc.componentCode)
		})
	}
}

func TestHandshakeFailureExitIsUnavailable(t *testing.T) {
	fakeBackend(t, "echo not-today >&2\nexit 9")
	_, err := Handshake(context.Background())
	ce := requireCLIError(t, err, exitcode.Unavailable, CodeBackendUnavailable)
	if !strings.Contains(ce.Message, "not-today") {
		t.Fatalf("handshake failure dropped the backend's stderr: %v", ce)
	}
}
