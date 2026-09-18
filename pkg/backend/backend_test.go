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
	`"backendVersion":"0.1.0","productRelease":"v0.9.8.9-rc.1","schemaVersion":"appliance-backend-payload/v1",` +
	`"commands":[` +
	`{"name":"status","readOnly":true,"capabilities":["json-output"]},` +
	`{"name":"network validate","readOnly":true,"capabilities":["json-output"]},` +
	`{"name":"public-address configure","readOnly":false,"capabilities":["json-output","no-restart","operation-receipt"]},` +
	`{"name":"gateway register-local","readOnly":false,"capabilities":["json-output","secret-stdin","operation-receipt"]},` +
	`{"name":"credentials bootstrap","readOnly":false,"capabilities":["console-only"]},` +
	`{"name":"diagnostics create","readOnly":false,"capabilities":["json-output","redaction","explicit-output","operation-receipt"]},` +
	`{"name":"logs","readOnly":false,"capabilities":["json-output","redaction","bounded-window"]},` +
	`{"name":"backup key-create","readOnly":false,"capabilities":["json-output","key-file","operation-receipt"]},` +
	`{"name":"backup create","readOnly":false,"capabilities":["json-output","key-file","operation-receipt"]},` +
	`{"name":"backup verify","readOnly":false,"capabilities":["json-output","key-file"]}]}`

const lifecycleContractSuffix = `,` +
	`{"name":"restore plan","readOnly":true,"capabilities":["json-output","archive","key-file","plan-hash"]},` +
	`{"name":"restore execute","readOnly":false,"capabilities":["json-output","plan-hash","one-time-challenge","operation-receipt"]},` +
	`{"name":"update plan","readOnly":true,"capabilities":["json-output","signed-bundle","plan-hash"]},` +
	`{"name":"update execute","readOnly":false,"capabilities":["json-output","plan-hash","one-time-challenge","operation-receipt"]},` +
	`{"name":"update status","readOnly":true,"capabilities":["json-output","operation-status"]},` +
	`{"name":"rollback plan","readOnly":true,"capabilities":["json-output","approved-release","archive","key-file","plan-hash"]},` +
	`{"name":"rollback execute","readOnly":false,"capabilities":["json-output","plan-hash","one-time-challenge","operation-receipt"]},` +
	`{"name":"rollback status","readOnly":true,"capabilities":["json-output","operation-status"]},` +
	`{"name":"factory-reset plan","readOnly":true,"capabilities":["json-output","preservation-plan","plan-hash"]},` +
	`{"name":"factory-reset execute","readOnly":false,"capabilities":["json-output","plan-hash","one-time-challenge","operation-receipt"]}`

func validLifecycleContract() string {
	return strings.TrimSuffix(validContract, "]}") + lifecycleContractSuffix + "]}"
}

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
		"cat > \"" + dir + "/stdin\"\n" +
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
	dir := fakeBackend(t, contractThenEcho(validContract))
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
	fakeBackend(t, `if [ "$1" = "contract-version" ]; then echo '`+validContract+`'; exit 0; fi
echo partial-out
echo boom >&2
exit 3`)
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
	if contract.BackendVersion != "0.1.0" || contract.SchemaVersion != payloadSchema {
		t.Fatalf("contract fields lost: %+v", contract)
	}
	if !contract.Supports("status") || !contract.Supports("network validate") {
		t.Fatalf("advertised commands lost: %+v", contract.Commands)
	}
	if !contract.Supports("backup create") || contract.Supports("restore") {
		t.Fatal("exact command availability was not preserved")
	}
}

func TestHandshakeAcceptsCompleteLifecycleContract(t *testing.T) {
	fakeBackend(t, fmt.Sprintf("echo '%s'", validLifecycleContract()))
	contract, err := Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !contract.Supports("restore plan") || !contract.Supports("factory-reset execute") {
		t.Fatalf("lifecycle capabilities lost: %+v", contract.Commands)
	}
}

func TestLifecycleReadOnlyAndMutatingArgvAreGated(t *testing.T) {
	dir := fakeBackend(t, contractThenEcho(validLifecycleContract()))
	plan, err := InvokeReadOnly(context.Background(), &Request{
		Subcommand: "restore plan", Args: []string{"/root/backup.tar", "--key-file", "/root/backup.key"}, JSON: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	if got := strings.Split(strings.TrimSpace(string(argv)), "\n"); strings.Join(got[:5], " ") != "restore plan /root/backup.tar --key-file /root/backup.key" || got[len(got)-1] != "--json" {
		t.Fatalf("restore plan argv = %#v", got)
	}

	execute, err := InvokeMutating(context.Background(), &Request{
		Subcommand: "update execute", Args: []string{"--plan-hash", strings.Repeat("a", 64), "--confirm", "challenge-1234"}, JSON: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CorrelationID == "" || execute.CorrelationID == "" {
		t.Fatal("lifecycle invocation lost a correlation ID")
	}
}

func TestLifecycleCapabilityDriftFailsBeforeOperation(t *testing.T) {
	drifted := strings.Replace(validLifecycleContract(), `"one-time-challenge","operation-receipt"`, `"operation-receipt","one-time-challenge"`, 1)
	dir := fakeBackend(t, contractThenEcho(drifted))
	_, err := InvokeMutating(context.Background(), &Request{
		Subcommand: "restore execute", Args: []string{"--plan-hash", strings.Repeat("a", 64), "--confirm", "challenge-1234"},
	})
	requireCLIError(t, err, exitcode.ContractMismatch, codeBackendContractInvalid)
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	if strings.Contains(string(argv), "restore\nexecute") {
		t.Fatalf("capability drift still reached operation: %q", argv)
	}
}

func TestLifecycleInvalidArgvFailsBeforeHandshake(t *testing.T) {
	for name, req := range map[string]*Request{
		"missing restore archive": {Subcommand: "restore plan", Args: []string{"--key-file", "/root/key"}},
		"unknown execute flag":    {Subcommand: "update execute", Args: []string{"--plan-hash", strings.Repeat("a", 64), "--force"}},
		"too many status ids":     {Subcommand: "rollback status", Args: []string{"op-1", "op-2"}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := fakeBackend(t, contractThenEcho(validLifecycleContract()))
			var err error
			if strings.Contains(req.Subcommand, "execute") {
				_, err = InvokeMutating(context.Background(), req)
			} else {
				_, err = InvokeReadOnly(context.Background(), req)
			}
			requireCLIError(t, err, exitcode.ContractMismatch, "argv-not-allowlisted")
			if _, statErr := os.Stat(filepath.Join(dir, "argv")); !os.IsNotExist(statErr) {
				t.Fatalf("invalid argv still spawned backend: %v", statErr)
			}
		})
	}
}

func TestHandshakeRefusesContractViolations(t *testing.T) {
	for name, tc := range map[string]struct {
		document      string
		componentCode string
	}{
		"not json": {"contract says hello", codeBackendContractInvalid},
		"wrong kind": {strings.Replace(validContract, `"BackendContract"`, `"SomethingElse"`, 1),
			codeBackendContractInvalid},
		"unknown field": {strings.Replace(validContract, `"schemaVersion":"appliance-backend-payload/v1"`, `"schemaVersion":"appliance-backend-payload/v1","extra":true`, 1),
			codeBackendContractInvalid},
		"wrong schema version": {strings.Replace(validContract, `"schemaVersion":"appliance-backend-payload/v1"`, `"schemaVersion":"appliance-backend-payload/v2"`, 1),
			codeBackendContractInvalid},
		"malformed command name": {strings.Replace(validContract, `"name":"status"`, `"name":"Status!"`, 1),
			codeBackendContractInvalid},
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
	ce := requireCLIError(t, err, exitcode.ContractMismatch, codeBackendContractInvalid)
	if strings.Contains(ce.Message, "not-today") {
		t.Fatalf("handshake classification parsed mutable stderr prose: %v", ce)
	}
}

// mutatingContract advertises the mutating slice this CLI ships, so gate
// tests can exercise both sides of Supports.
const mutatingContract = `{"apiVersion":"loxilb.io/appliance-backend/v1","kind":"BackendContract",` +
	`"backendVersion":"0.1.0","productRelease":"v0.9.8.9-rc.1","schemaVersion":"appliance-backend-payload/v1",` +
	`"commands":[{"name":"gateway register-local","readOnly":false,"capabilities":[]},` +
	`{"name":"logs","readOnly":false,"capabilities":["redaction"]}]}`

// contractThenEcho answers contract-version with the given contract and
// every other invocation with a fixed JSON document.
func contractThenEcho(contract string) string {
	return `if [ "$1" = "contract-version" ]; then echo '` + contract + `'; else echo '{"done":true}'; fi`
}

func TestArgvRuleShapes(t *testing.T) {
	rule := argvRule{positionals: 1, flags: []string{"--redact", "--since=", "--lines="}}
	for name, tc := range map[string]struct {
		args []string
		ok   bool
	}{
		"empty":                     {nil, true},
		"positional plus flags":     {[]string{"gateway", "--redact", "--since", "1h"}, true},
		"valued flag without value": {[]string{"gateway", "--since"}, false},
		"unknown flag":              {[]string{"gateway", "--follow"}, false},
		"too many positionals":      {[]string{"gateway", "oam"}, false},
		"flag not in rule as value": {[]string{"--redact", "--lines", "10"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			err := rule.validate(tc.args)
			if (err == nil) != tc.ok {
				t.Fatalf("validate(%v) = %v, want ok=%v", tc.args, err, tc.ok)
			}
		})
	}
}

func TestMutatingRunsTheHandshakeFirstAndRefusesUnadvertised(t *testing.T) {
	dir := fakeBackend(t, contractThenEcho(mutatingContract))
	_, err := InvokeMutating(context.Background(), &Request{
		Subcommand: "backup create", Args: []string{"/root/a.tar", "--key-file", "/root/k"}})
	requireCLIError(t, err, exitcode.ContractMismatch, codeBackendContractInvalid)
	// The handshake ran; the refused subcommand itself never spawned.
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	if !strings.Contains(string(argv), "contract-version") {
		t.Fatalf("no handshake before the mutating call; argv file: %q", argv)
	}
	if strings.Contains(string(argv), "backup") {
		t.Fatal("an unadvertised mutating subcommand still spawned")
	}
}

func TestMutatingSecretTravelsOnStdinOnly(t *testing.T) {
	dir := fakeBackend(t, contractThenEcho(validContract))
	secret := "correcthorsebatterystaple"
	res, err := InvokeMutating(context.Background(), &Request{
		Subcommand: "gateway register-local",
		Args:       []string{"--username", "admin", "--password-stdin"},
		Secret:     strings.NewReader(secret),
	})
	if err != nil {
		t.Fatalf("register-local failed: %v", err)
	}
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if !strings.Contains(string(stdin), secret) {
		t.Fatalf("the secret did not reach the backend's stdin: %q", stdin)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	if strings.Contains(string(argv), secret) || strings.Contains(string(env), secret) {
		t.Fatalf("the secret leaked outside stdin\nargv: %q\nenv: %q", argv, env)
	}
	if !strings.Contains(string(argv), "--username\nadmin\n--password-stdin") {
		t.Fatalf("register-local argv shape wrong: %q", argv)
	}
	if res.CorrelationID == "" {
		t.Fatal("mutating invocation lost its correlation id")
	}
}

func TestMutatingRefusesSecretForNonSecretSubcommand(t *testing.T) {
	dir := fakeBackend(t, contractThenEcho(validContract))
	_, err := InvokeMutating(context.Background(), &Request{
		Subcommand: "logs", Args: []string{"gateway", "--redact"},
		Secret: strings.NewReader("sneaky"),
	})
	requireCLIError(t, err, exitcode.ContractMismatch, "argv-not-allowlisted")
	if argv, _ := os.ReadFile(filepath.Join(dir, "argv")); strings.Contains(string(argv), "logs") {
		t.Fatal("a refused secret-bearing invocation still spawned")
	}
}

func TestReadOnlyEntryRefusesMutatingSubcommands(t *testing.T) {
	dir := fakeBackend(t, contractThenEcho(validContract))
	_, err := Invoke(context.Background(), "logs", false)
	requireCLIError(t, err, exitcode.ContractMismatch, "argv-not-allowlisted")
	if argv, _ := os.ReadFile(filepath.Join(dir, "argv")); strings.Contains(string(argv), "logs") {
		t.Fatal("a mutating subcommand spawned through the read-only entry point")
	}
}

// TestOperationIDPrefersTheBackendsOwn pins the precedence. The backend's id is
// the one an operator can look up in the product's own records; the correlation
// id is the CLI's, guaranteed searchable in the host journal, and stands in only
// when the backend named none. Getting this backwards would hand back an id
// that leads nowhere useful while a better one was available.
func TestOperationIDPrefersTheBackendsOwn(t *testing.T) {
	withBackendOwn := &Result{
		Stdout:        []byte(`{"operationId":"op-from-backend"}`),
		CorrelationID: "cli-fallback",
	}
	if got := withBackendOwn.OperationID(); got != "op-from-backend" {
		t.Errorf("OperationID() = %q, want the backend's own", got)
	}

	for name, res := range map[string]*Result{
		"no json":        {Stdout: []byte("plain prose"), CorrelationID: "cli-fallback"},
		"json no id":     {Stdout: []byte(`{"other":true}`), CorrelationID: "cli-fallback"},
		"empty id":       {Stdout: []byte(`{"operationId":""}`), CorrelationID: "cli-fallback"},
		"no stdout":      {CorrelationID: "cli-fallback"},
		"truncated json": {Stdout: []byte(`{"operationId":`), CorrelationID: "cli-fallback"},
	} {
		if got := res.OperationID(); got != "cli-fallback" {
			t.Errorf("%s: OperationID() = %q, want the correlation id", name, got)
		}
	}
}
