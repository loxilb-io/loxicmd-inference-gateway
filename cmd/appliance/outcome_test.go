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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// outcomeBackend answers the handshake, advertises the mutating command used
// below, and hangs on it so the invocation has to be stopped from outside.
const outcomeBackend = `
if [ "$1" = "contract-version" ]; then
cat <<JSON
{"apiVersion":"loxilb.io/appliance-backend/v1","kind":"BackendContract",
 "backendVersion":"fake-1.0","productRelease":"qa","schemaVersion":1,
 "commands":[{"name":"status","readOnly":true,"capabilities":[]},
             {"name":"backup create","readOnly":false,"capabilities":[]}]}
JSON
exit 0
fi
exec sleep 60
`

func envelopeOf(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout)
	}
	return doc
}

func dataOf(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	d, ok := doc["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope data is %T, want an object", doc["data"])
	}
	return d
}

// TestKilledMutationIsPartialWithARecoveryHandle is the case the exit-code
// taxonomy singles out: an operation whose outcome is unknown resolves DOWNWARD
// to safety. A backup killed mid-flight may have written part of an archive, so
// reporting FAILED -- which automation is told it may retry once the cause is
// addressed -- invites compounding the damage. PARTIAL is the code that must
// never be retried blindly, and it is only actionable with an id to recover by.
func TestKilledMutationIsPartialWithARecoveryHandle(t *testing.T) {
	binary, _ := buildCLIWithBackend(t, outcomeBackend)
	keyDir := t.TempDir()
	key := filepath.Join(keyDir, "backup.key")
	if err := os.WriteFile(key, []byte("fixture-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	status, stdout, stderr := runAppliance(t, binary,
		"-t", "1", "appliance", "backup", "create", "/tmp/outcome.tar", "--key-file", key, "-o", "json")
	if status != 8 {
		t.Fatalf("status=%d, want 8 (PARTIAL). stderr=%q", status, stderr)
	}
	doc := envelopeOf(t, stdout)
	if doc["code"] != "PARTIAL" {
		t.Errorf("envelope code %v, want PARTIAL", doc["code"])
	}
	data := dataOf(t, doc)
	if data["componentCode"] != "backend-timeout" {
		t.Errorf("componentCode %v, want backend-timeout", data["componentCode"])
	}
	if id, _ := data["operationId"].(string); id == "" {
		t.Error("no data.operationId: exit 8 tells the caller to recover rather than retry, " +
			"which is not something they can act on without a handle")
	}
	// The message has to say the outcome is unknown. "failed" would be a
	// claim the CLI is in no position to make.
	if !strings.Contains(stderr, "UNKNOWN") || !strings.Contains(stderr, "do not retry blindly") {
		t.Errorf("stderr does not convey an unknown outcome: %q", stderr)
	}
}

// TestKilledReadOnlyIsNotPartial keeps the downgrade honest in the other
// direction. A read-only subcommand cannot have changed anything, so the same
// death is the backend being unresponsive -- retryable -- and calling it PARTIAL
// would send an operator hunting for state that could not have moved.
func TestKilledReadOnlyIsNotPartial(t *testing.T) {
	binary, _ := buildCLIWithBackend(t, outcomeBackend)

	status, stdout, stderr := runAppliance(t, binary, "-t", "1", "appliance", "status", "-o", "json")
	if status != 5 {
		t.Fatalf("status=%d, want 5 (UNAVAILABLE) for a read-only command. stderr=%q", status, stderr)
	}
	doc := envelopeOf(t, stdout)
	if doc["code"] != "UNAVAILABLE" {
		t.Errorf("envelope code %v, want UNAVAILABLE", doc["code"])
	}
	if _, present := dataOf(t, doc)["operationId"]; present {
		t.Error("a read-only command published an operationId; there is nothing to recover")
	}
}

// TestDecidedFailureStaysFailed guards the other side of the split. A backend
// that exits non-zero has decided something, and turning every backend refusal
// into PARTIAL would make the code meaningless -- exit 8 has to keep meaning
// "nobody knows", or automation cannot use it to stop.
func TestDecidedFailureStaysFailed(t *testing.T) {
	binary, _ := buildCLIWithBackend(t, `
if [ "$1" = "contract-version" ]; then echo '{"apiVersion":"loxilb.io/appliance-backend/v1","kind":"BackendContract","backendVersion":"f","productRelease":"q","schemaVersion":1,"commands":[{"name":"status","readOnly":true,"capabilities":[]}]}'; exit 0; fi
echo "the backend decided to refuse" >&2
exit 42
`)
	status, stdout, _ := runAppliance(t, binary, "appliance", "status", "-o", "json")
	if status != 7 {
		t.Fatalf("status=%d, want 7 (FAILED) for a decided refusal", status)
	}
	data := dataOf(t, envelopeOf(t, stdout))
	if data["componentCode"] != "backend-exit-42" {
		t.Errorf("componentCode %v, want the backend's own exit preserved", data["componentCode"])
	}
	if _, present := data["operationId"]; present {
		t.Error("a decided failure published an operationId; the backend already said it did not happen")
	}
}

// TestUnexecutableBackendIsAuthNotUnavailable separates two failures that used
// to share a bucket. UNAVAILABLE tells automation to retry with backoff, which
// an under-privileged caller can do forever without ever succeeding; AUTH is
// the row the taxonomy defines for insufficient OS privilege, and it tells the
// operator to change who they are instead of waiting.
func TestUnexecutableBackendIsAuthNotUnavailable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("loxicmd links Linux netlink; the packaged binary only builds there")
	}
	if os.Geteuid() == 0 {
		t.Skip("root is exempt from the permission check this leg needs")
	}
	binary, backendDir := buildCLIWithBackend(t, "exit 0\n")
	if err := os.Chmod(filepath.Join(backendDir, "loxilb-appliance-backend"), 0o600); err != nil {
		t.Fatal(err)
	}

	status, stdout, stderr := runAppliance(t, binary, "appliance", "status", "-o", "json")
	if status != 3 {
		t.Fatalf("status=%d, want 3 (AUTH). stderr=%q", status, stderr)
	}
	data := dataOf(t, envelopeOf(t, stdout))
	if data["componentCode"] != "BACKEND_FORBIDDEN" {
		t.Errorf("componentCode %v, want BACKEND_FORBIDDEN", data["componentCode"])
	}
	if data["origin"] != "os" {
		t.Errorf("origin %v, want os — the refusal came from the kernel, not the backend", data["origin"])
	}
}
