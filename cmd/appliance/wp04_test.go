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
package appliance

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const validDiagnosticsPayload = `{"schemaVersion":"appliance-backend-payload/v1","command":"diagnostics create","result":"CREATED","operationId":"op-diagnostics-01","archivePath":"/tmp/support.tar.gz","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sizeBytes":4096,"mode":"0600","redactionProfile":"strict-v1","includedFiles":["/var/log/loxilb/gateway.log"],"secretScan":{"result":"PASS","hits":0},"expiresAt":"2026-09-16T00:00:00Z"}`

const validLogsPayload = `{"schemaVersion":"appliance-backend-payload/v1","command":"logs","component":"gateway","redacted":true,"window":{"since":"30m","lines":50},"entries":[{"timestamp":"2026-09-15T00:00:00Z","severity":"INFO","correlationOrOperationId":"cli-01","message":"paired redacted line one"}],"truncated":false,"observedAt":"2026-09-15T00:00:01Z"}`

func decodeSingleCommandResult(t *testing.T, stdout, command string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("stdout is not a CommandResult: %v\n%s", err, stdout)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout contains more than one JSON document: err=%v extra=%#v\n%s", err, extra, stdout)
	}
	if result["kind"] != "CommandResult" || result["command"] != command || result["success"] != true {
		t.Fatalf("unexpected CommandResult: %s", stdout)
	}
	return result
}

// TestCLIWP04DiagnosticsOutputComposition reproduces Product CLI-WP04-001
// byte for byte. Both the short and long global output selectors must compose
// with the diagnostics archive path instead of failing before backend exec.
func TestCLIWP04DiagnosticsOutputComposition(t *testing.T) {
	recordDir := t.TempDir()
	script := withValidHandshake(strings.ReplaceAll(`printf '%s\n' "$@" > "$RECORD_DIR/argv"
printf 'progress: collecting diagnostics\nwarning: journal window was truncated\n' >&2
printf '%s\n' '`+validDiagnosticsPayload+`'
`, "$RECORD_DIR", recordDir))
	binary, _ := buildCLIWithBackend(t, script)

	for name, args := range map[string][]string{
		"short global selector": {"-o", "json", "appliance", "diagnostics", "create", "--redact", "--output", "/tmp/support.tar.gz"},
		"long global selector":  {"--output", "json", "appliance", "diagnostics", "create", "--redact", "--output", "/tmp/support.tar.gz"},
	} {
		t.Run(name, func(t *testing.T) {
			status, stdout, stderr := runAppliance(t, binary, args...)
			if status != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			result := decodeSingleCommandResult(t, stdout, "appliance.diagnostics.create")
			data, ok := result["data"].(map[string]any)
			if !ok {
				t.Fatalf("CommandResult data is missing: %s", stdout)
			}
			backend, ok := data["backend"].(map[string]any)
			if !ok || backend["archivePath"] != "/tmp/support.tar.gz" {
				t.Fatalf("diagnostics payload lost the archive path: %s", stdout)
			}
			if stderr != "progress: collecting diagnostics\nwarning: journal window was truncated\n" {
				t.Fatalf("backend stderr was not preserved exactly: %q", stderr)
			}
			if strings.Contains(stdout, "progress:") || strings.Contains(stdout, "warning:") {
				t.Fatalf("backend stderr contaminated JSON stdout: %q", stdout)
			}

			argvBytes, err := os.ReadFile(filepath.Join(recordDir, "argv"))
			if err != nil {
				t.Fatal(err)
			}
			argv := strings.Split(strings.TrimSpace(string(argvBytes)), "\n")
			if len(argv) != 8 || !reflect.DeepEqual(argv[:6], []string{
				"diagnostics", "create", "--redact", "--output", "/tmp/support.tar.gz", "--correlation-id",
			}) || !strings.HasPrefix(argv[6], "cli-") || argv[7] != "--json" {
				t.Fatalf("unexpected backend argv: %#v", argv)
			}
		})
	}
}

// TestCLIWP04SuccessfulBackendStderrSeparation mirrors Product's paired logs
// acceptance: a successful JSON payload stays alone on stdout while progress
// from the backend remains byte-identical on user stderr.
func TestCLIWP04SuccessfulBackendStderrSeparation(t *testing.T) {
	binary, _ := buildCLIWithBackend(t, withValidHandshake(`printf 'paired redacted line one\npaired warning line\n' >&2
printf '%s\n' '`+validLogsPayload+`'`))

	status, stdout, stderr := runAppliance(t, binary,
		"-o", "json", "appliance", "logs", "gateway", "--redact", "--since", "30m", "--lines", "50")
	if status != 0 {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	decodeSingleCommandResult(t, stdout, "appliance.logs")
	if stderr != "paired redacted line one\npaired warning line\n" {
		t.Fatalf("backend stderr was not preserved exactly: %q", stderr)
	}
	if strings.Contains(stdout, "paired warning line") {
		t.Fatalf("backend stderr contaminated JSON stdout: %q", stdout)
	}
}
