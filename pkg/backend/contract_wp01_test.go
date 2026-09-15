/*
 * Copyright (c) 2026 LoxiLB Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

type wp01BackendBehavior struct {
	handshakeStdout string
	handshakeStderr string
	handshakeExit   int
	operationStdout string
	operationStderr string
	operationExit   int
}

func installWP01Backend(t *testing.T, behavior wp01BackendBehavior) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "loxilb-appliance-backend")
	invocations := filepath.Join(dir, "invocations")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %s
if [ "$1" = "contract-version" ]; then
  printf '%%s' %s
  printf '%%s' %s >&2
  exit %d
fi
printf '%%s' %s
printf '%%s' %s >&2
exit %d
`, strconv.Quote(invocations), strconv.Quote(behavior.handshakeStdout), strconv.Quote(behavior.handshakeStderr), behavior.handshakeExit,
		strconv.Quote(behavior.operationStdout), strconv.Quote(behavior.operationStderr), behavior.operationExit)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	setExecutablePath(t, path)
	return invocations
}

func invocationCounts(t *testing.T, path string) (handshake, operation int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.HasPrefix(line, "contract-version ") {
			handshake++
		} else if line != "" {
			operation++
		}
	}
	return handshake, operation
}

func typedHandshakeError(exit int, code, component string, retryable bool, message string) string {
	messageField := ""
	if message != "" {
		messageField = `,"message":` + strconv.Quote(message)
	}
	return fmt.Sprintf(`{"schemaVersion":"backend-contract-error/v1","exit":%d,"code":%q,"componentCode":%q,"retryable":%t%s}`,
		exit, code, component, retryable, messageField)
}

func TestCLIWP01HandshakeAcceptsExactContract(t *testing.T) {
	installWP01Backend(t, wp01BackendBehavior{handshakeStdout: validContract})
	contract, err := Handshake(context.Background())
	if err != nil {
		t.Fatalf("Handshake() error = %v", err)
	}
	if len(contract.Commands) != 10 || contract.SchemaVersion != payloadSchema {
		t.Fatalf("contract = %+v", contract)
	}
	command, ok := contract.Lookup("diagnostics create")
	if !ok || command.ReadOnly || !equalStrings(command.Capabilities, []string{"json-output", "redaction", "explicit-output", "operation-receipt"}) {
		t.Fatalf("lookup lost exact metadata: %+v, %t", command, ok)
	}
}

func TestCLIWP01HandshakeApprovedFixtureIdentity(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	rawManifest, err := os.ReadFile(filepath.Join(root, "testdata/backend-contract/cli-contract-candidate.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Bundle struct {
			AggregateSHA256 string `json:"aggregateSha256"`
			Files           []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
			} `json:"files"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Bundle.AggregateSHA256 != "sha256:eb6c0d40639ce4a73726101ffc27e378c755c308056708c03972d48cbbbb50ea" {
		t.Fatalf("CP-CLI aggregate = %q", manifest.Bundle.AggregateSHA256)
	}
	want := map[string]string{}
	for _, item := range manifest.Bundle.Files {
		want[item.Path] = item.SHA256
	}
	for _, path := range []string{
		"contracts/backend-contract.schema.json",
		"contracts/backend-errors/v1/backend-contract-error.schema.json",
		"testdata/backend-contract/fixtures.json",
	} {
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		got := "sha256:" + hex.EncodeToString(digest[:])
		if got != want[path] {
			t.Fatalf("%s digest = %s, manifest = %s", path, got, want[path])
		}
	}
}

func TestCLIWP01HandshakeRejectsDocumentShapeMutants(t *testing.T) {
	missingReadOnly := strings.Replace(validContract, `"name":"public-address configure","readOnly":false,`, `"name":"public-address configure",`, 1)
	nullCapabilities := strings.Replace(validContract, `"capabilities":["json-output","key-file"]`, `"capabilities":null`, 1)
	duplicateCommand := strings.Replace(validContract, `"name":"backup verify"`, `"name":"backup create"`, 1)
	duplicateCapability := strings.Replace(validContract, `"capabilities":["json-output","key-file"]`, `"capabilities":["json-output","key-file","key-file"]`, 1)
	unknownCommand := strings.Replace(validContract, `"name":"backup verify"`, `"name":"restore"`, 1)
	malformedCapability := strings.Replace(validContract, `"capabilities":["json-output"]`, `"capabilities":["JSON_OUTPUT"]`, 1)
	missingBackendVersion := strings.Replace(validContract, `"backendVersion":"0.1.0",`, "", 1)
	emptyProductRelease := strings.Replace(validContract, `"productRelease":"v0.9.8.9-rc.1"`, `"productRelease":""`, 1)
	missingCommands := validContract[:strings.Index(validContract, `,"commands"`)] + "}"
	for name, document := range map[string]string{
		"second document":      validContract + `{}`,
		"trailing prose":       validContract + ` trailing`,
		"null document":        `null`,
		"missing commands":     missingCommands,
		"null commands":        strings.Replace(validContract, `"commands":[`, `"commands":null,"ignored":[`, 1),
		"missing readOnly":     missingReadOnly,
		"null capabilities":    nullCapabilities,
		"duplicate command":    duplicateCommand,
		"duplicate capability": duplicateCapability,
		"unknown command":      unknownCommand,
		"malformed capability": malformedCapability,
		"missing version":      missingBackendVersion,
		"empty release":        emptyProductRelease,
	} {
		t.Run(name, func(t *testing.T) {
			installWP01Backend(t, wp01BackendBehavior{handshakeStdout: document})
			_, err := Handshake(context.Background())
			requireCLIError(t, err, exitcode.ContractMismatch, codeBackendContractInvalid)
		})
	}
}

func TestCLIWP01HandshakeMapsTypedFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		exit          int
		code          string
		componentCode string
		retryable     bool
	}{
		"release marker missing": {4, "PRECONDITION", codeBackendReleaseMarkerMissing, false},
		"release marker invalid": {6, "CONTRACT_MISMATCH", codeBackendReleaseMarkerInvalid, false},
		"contract invalid":       {6, "CONTRACT_MISMATCH", codeBackendContractInvalid, false},
		"dependency unavailable": {5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true},
	} {
		t.Run(name, func(t *testing.T) {
			document := typedHandshakeError(tc.exit, tc.code, tc.componentCode, tc.retryable, "bounded safe message")
			installWP01Backend(t, wp01BackendBehavior{handshakeStdout: document, handshakeStderr: "mutable prose", handshakeExit: tc.exit})
			_, err := Handshake(context.Background())
			var typed *BackendContractError
			if !errors.As(err, &typed) {
				t.Fatalf("error type = %T, want *BackendContractError (%v)", err, err)
			}
			if typed.Exit != tc.exit || typed.Code != tc.code || typed.ComponentCode != tc.componentCode || typed.Retryable != tc.retryable {
				t.Fatalf("typed error = %+v", typed)
			}
			requireCLIError(t, err, exitcode.Code(tc.exit), tc.componentCode)
		})
	}
}

func TestCLIWP01HandshakeRejectsTypedFailureMutants(t *testing.T) {
	valid := typedHandshakeError(5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true, "dependency unavailable")
	for name, tc := range map[string]struct {
		document    string
		processExit int
	}{
		"second document":    {valid + `{}`, 5},
		"missing required":   {strings.Replace(valid, `,"retryable":true`, "", 1), 5},
		"unknown field":      {strings.Replace(valid, `,"retryable":true`, `,"retryable":true,"extra":1`, 1), 5},
		"wrong tuple":        {strings.Replace(valid, `"retryable":true`, `"retryable":false`, 1), 5},
		"exit mismatch":      {valid, 6},
		"empty message":      {strings.TrimSuffix(valid, "}") + `,"message":""}`, 5},
		"control character":  {typedHandshakeError(5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true, "bad\nmessage"), 5},
		"credential pattern": {typedHandshakeError(5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true, "token=example"), 5},
		"synthetic sentinel": {typedHandshakeError(5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true, "CLI_WP00_SYNTHETIC_SECRET_DO_NOT_EMIT"), 5},
		"message too large":  {typedHandshakeError(5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true, strings.Repeat("x", 513)), 5},
	} {
		t.Run(name, func(t *testing.T) {
			installWP01Backend(t, wp01BackendBehavior{handshakeStdout: tc.document, handshakeExit: tc.processExit})
			_, err := Handshake(context.Background())
			requireCLIError(t, err, exitcode.ContractMismatch, codeBackendContractInvalid)
		})
	}
}

func TestCLIWP01HandshakeIgnoresStderrMutation(t *testing.T) {
	document := typedHandshakeError(5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true, "typed message")
	for _, stderr := range []string{"", "release marker missing", "token=must-not-parse", strings.Repeat("x", 2048)} {
		installWP01Backend(t, wp01BackendBehavior{handshakeStdout: document, handshakeStderr: stderr, handshakeExit: 5})
		_, err := Handshake(context.Background())
		var typed *BackendContractError
		if !errors.As(err, &typed) || typed.ComponentCode != codeBackendHandshakeUnavailable || !typed.Retryable {
			t.Fatalf("stderr changed typed classification: %v", err)
		}
	}
}

func TestCLIWP01CapabilityAllowsExactMetadata(t *testing.T) {
	invocations := installWP01Backend(t, wp01BackendBehavior{handshakeStdout: validContract, operationStdout: `{"result":"ok"}`})
	_, err := InvokeMutating(context.Background(), &Request{Subcommand: "backup create", Args: []string{"/root/a.tar", "--key-file", "/root/k"}, JSON: true})
	if err != nil {
		t.Fatalf("InvokeMutating() error = %v", err)
	}
	handshake, operation := invocationCounts(t, invocations)
	if handshake != 1 || operation != 1 {
		t.Fatalf("spawn counts = handshake %d, operation %d", handshake, operation)
	}
}

func TestCLIWP01CapabilityRejectsMetadataMutantsWithoutOperationSpawn(t *testing.T) {
	for name, contract := range map[string]string{
		"missing command":    strings.Replace(validContract, `,{"name":"backup verify","readOnly":false,"capabilities":["json-output","key-file"]}`, "", 1),
		"readOnly mismatch":  strings.Replace(validContract, `"name":"backup create","readOnly":false`, `"name":"backup create","readOnly":true`, 1),
		"missing capability": strings.Replace(validContract, `"capabilities":["json-output","key-file","operation-receipt"]`, `"capabilities":["json-output","key-file"]`, 1),
		"unknown capability": strings.Replace(validContract, `"capabilities":["json-output","key-file"]`, `"capabilities":["json-output","key-file","unknown"]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			invocations := installWP01Backend(t, wp01BackendBehavior{handshakeStdout: contract, operationStdout: `{}`})
			_, err := InvokeMutating(context.Background(), &Request{Subcommand: "backup create", Args: []string{"/root/a.tar", "--key-file", "/root/k"}})
			requireCLIError(t, err, exitcode.ContractMismatch, codeBackendContractInvalid)
			handshake, operation := invocationCounts(t, invocations)
			if handshake != 1 || operation != 0 {
				t.Fatalf("spawn counts = handshake %d, operation %d", handshake, operation)
			}
		})
	}
}

func TestCLIWP01CapabilityRejectsInvalidArgvWithoutOperationSpawn(t *testing.T) {
	invocations := installWP01Backend(t, wp01BackendBehavior{handshakeStdout: validContract})
	_, err := InvokeMutating(context.Background(), &Request{Subcommand: "backup create", Args: []string{"/root/a.tar", "--unknown"}})
	requireCLIError(t, err, exitcode.ContractMismatch, "argv-not-allowlisted")
	handshake, operation := invocationCounts(t, invocations)
	if handshake != 0 || operation != 0 {
		t.Fatalf("invalid argv spawned: handshake %d, operation %d", handshake, operation)
	}
}

func TestCLIWP01CapabilityRejectsSecretMismatchWithoutOperationSpawn(t *testing.T) {
	invocations := installWP01Backend(t, wp01BackendBehavior{handshakeStdout: validContract})
	_, err := InvokeMutating(context.Background(), &Request{Subcommand: "logs", Args: []string{"gateway", "--redact"}, Secret: strings.NewReader("synthetic")})
	requireCLIError(t, err, exitcode.ContractMismatch, "argv-not-allowlisted")
	handshake, operation := invocationCounts(t, invocations)
	if handshake != 0 || operation != 0 {
		t.Fatalf("secret mismatch spawned: handshake %d, operation %d", handshake, operation)
	}
}

func TestCLIWP01DegradedRunsReadOnlyOnceAndDiscardsJSON(t *testing.T) {
	document := typedHandshakeError(5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true, "dependency unavailable")
	invocations := installWP01Backend(t, wp01BackendBehavior{handshakeStdout: document, handshakeExit: 5, operationStdout: `{"unvalidated":true}`, operationStderr: "untrusted", operationExit: 0})
	result, err := Invoke(context.Background(), "status", true)
	requireCLIError(t, err, exitcode.Unavailable, codeBackendHandshakeUnavailable)
	if result == nil || result.Degraded == nil || !result.Degraded.Degraded || !result.Degraded.OperationAttempted {
		t.Fatalf("degraded result missing: %+v", result)
	}
	if len(result.Stdout) != 0 || len(result.Stderr) != 0 {
		t.Fatalf("unvalidated JSON streams escaped: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	handshake, operation := invocationCounts(t, invocations)
	if handshake != 1 || operation != 1 {
		t.Fatalf("spawn counts = handshake %d, operation %d", handshake, operation)
	}
}

func TestCLIWP01DegradedDoesNotRunWhenExecutableUnavailableOrForbidden(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		setExecutablePath(t, filepath.Join(t.TempDir(), "absent"))
		result, err := Invoke(context.Background(), "status", true)
		if result != nil {
			t.Fatalf("result = %+v, want nil", result)
		}
		requireCLIError(t, err, exitcode.Unavailable, CodeBackendUnavailable)
	})
	t.Run("forbidden", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "backend")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		setExecutablePath(t, path)
		result, err := Invoke(context.Background(), "status", true)
		if result != nil {
			t.Fatalf("result = %+v, want nil", result)
		}
		requireCLIError(t, err, exitcode.Auth, CodeBackendForbidden)
	})
}

func TestCLIWP01DegradedPreservesTypedHandshakeStatus(t *testing.T) {
	for name, tc := range map[string]struct {
		exit          int
		code          string
		componentCode string
		retryable     bool
	}{
		"marker missing":         {4, "PRECONDITION", codeBackendReleaseMarkerMissing, false},
		"marker invalid":         {6, "CONTRACT_MISMATCH", codeBackendReleaseMarkerInvalid, false},
		"contract invalid":       {6, "CONTRACT_MISMATCH", codeBackendContractInvalid, false},
		"dependency unavailable": {5, "UNAVAILABLE", codeBackendHandshakeUnavailable, true},
	} {
		t.Run(name, func(t *testing.T) {
			document := typedHandshakeError(tc.exit, tc.code, tc.componentCode, tc.retryable, "typed")
			installWP01Backend(t, wp01BackendBehavior{handshakeStdout: document, handshakeExit: tc.exit, operationStdout: `{"ignored":true}`})
			result, err := Invoke(context.Background(), "status", true)
			requireCLIError(t, err, exitcode.Code(tc.exit), tc.componentCode)
			if result.Degraded.HandshakeStatus != tc.exit || result.Degraded.Reason != tc.componentCode {
				t.Fatalf("degraded status = %+v", result.Degraded)
			}
		})
	}
}

func TestCLIWP01DegradedAcceptsGatewayNotReadyObservation(t *testing.T) {
	status := `{"schemaVersion":"appliance-backend-payload/v1","command":"status","overallStatus":"NOT_READY","productRelease":"v0.9.8.9-rc.1","initialized":true,"planes":[{"name":"gateway","live":false,"ready":false,"reasonCode":"GATEWAY_DOWN"}],"networkProfile":{"name":"dual-nic","configured":true},"activeOperations":[],"localGatewayRegistration":{"registered":true,"installationId":"install-01","instanceId":"gateway-01"},"observedAt":"2026-09-15T00:00:00Z"}`
	invocations := installWP01Backend(t, wp01BackendBehavior{handshakeStdout: validContract, operationStdout: status})
	result, err := Invoke(context.Background(), "status", true)
	if err != nil {
		t.Fatalf("NOT_READY observation became a CLI failure: %v", err)
	}
	if result.Degraded != nil || !strings.Contains(string(result.Stdout), `"overallStatus":"NOT_READY"`) {
		t.Fatalf("valid observation lost: %+v", result)
	}
	handshake, operation := invocationCounts(t, invocations)
	if handshake != 1 || operation != 1 {
		t.Fatalf("spawn counts = handshake %d, operation %d", handshake, operation)
	}
}
