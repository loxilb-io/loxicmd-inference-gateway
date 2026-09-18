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
	"reflect"
	"strings"
	"testing"
)

const lifecycleContractCommands = `,
{"name":"restore plan","readOnly":true,"capabilities":["json-output","archive","key-file","plan-hash"]},
{"name":"restore execute","readOnly":false,"capabilities":["json-output","plan-hash","one-time-challenge","operation-receipt"]},
{"name":"update plan","readOnly":true,"capabilities":["json-output","signed-bundle","plan-hash"]},
{"name":"update execute","readOnly":false,"capabilities":["json-output","plan-hash","one-time-challenge","operation-receipt"]},
{"name":"update status","readOnly":true,"capabilities":["json-output","operation-status"]},
{"name":"rollback plan","readOnly":true,"capabilities":["json-output","approved-release","archive","key-file","plan-hash"]},
{"name":"rollback execute","readOnly":false,"capabilities":["json-output","plan-hash","one-time-challenge","operation-receipt"]},
{"name":"rollback status","readOnly":true,"capabilities":["json-output","operation-status"]},
{"name":"factory-reset plan","readOnly":true,"capabilities":["json-output","preservation-plan","plan-hash"]},
{"name":"factory-reset execute","readOnly":false,"capabilities":["json-output","plan-hash","one-time-challenge","operation-receipt"]}`

func validLifecycleBackendContract() string {
	return strings.TrimSuffix(validApplianceBackendContract, "]}") + lifecycleContractCommands + "]}"
}

func withLifecycleHandshake(operationScript string) string {
	return `if [ "$1" = "contract-version" ]; then
cat <<'JSON'
` + validLifecycleBackendContract() + `
JSON
exit 0
fi
` + operationScript
}

const planHashFixture = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func lifecycleInputFiles(t *testing.T) (archive, bundle, key string) {
	t.Helper()
	dir := t.TempDir()
	archive = filepath.Join(dir, "backup.tar.age")
	bundle = filepath.Join(dir, "update.bundle")
	key = filepath.Join(dir, "backup.key")
	for path, mode := range map[string]os.FileMode{archive: 0o600, bundle: 0o600, key: 0o600} {
		if err := os.WriteFile(path, []byte("fixture"), mode); err != nil {
			t.Fatal(err)
		}
	}
	return archive, bundle, key
}

func TestLifecyclePlanAndExecuteDispatch(t *testing.T) {
	archive, bundle, key := lifecycleInputFiles(t)
	record := filepath.Join(t.TempDir(), "argv")
	script := strings.ReplaceAll(withLifecycleHandshake(`printf '%s\n' "$@" > "$RECORD"
case "$1 $2" in
  "restore plan"|"update plan"|"rollback plan")
    printf '{"schemaVersion":"appliance-backend-payload/v1","command":"%s %s","result":"PLANNED","planHash":"`+planHashFixture+`","expiresAt":"2026-09-19T00:00:00Z","compatible":true,"affectedPlanes":["state","management"],"checks":[{"code":"ARCHIVE_AUTHENTICATION","result":"PASS"},{"code":"ARCHIVE_CHECKSUM","result":"PASS"},{"code":"SCHEMA_COMPATIBILITY","result":"PASS"},{"code":"DISK_CAPACITY","result":"PASS"},{"code":"INSTALLATION_COMPATIBILITY","result":"PASS"},{"code":"PRE_RESTORE_BACKUP","result":"PASS"},{"code":"ROLLBACK_FEASIBILITY","result":"PASS"},{"code":"BUNDLE_SIGNATURE","result":"PASS"},{"code":"PRODUCT_LOCK","result":"PASS"},{"code":"SBOM","result":"PASS"},{"code":"MIGRATION","result":"PASS"},{"code":"BACKUP_GATE","result":"PASS"},{"code":"APPROVED_RELEASE","result":"PASS"},{"code":"DOWNGRADE_COMPATIBILITY","result":"PASS"},{"code":"POSTFLIGHT","result":"PASS"},{"code":"SSH_PRESERVATION","result":"PASS"},{"code":"NETWORK_PRESERVATION","result":"PASS"},{"code":"NCP_AGENT_PRESERVATION","result":"PASS"}]}\n' "$1" "$2" ;;
  "factory-reset plan")
    printf '{"schemaVersion":"appliance-backend-payload/v1","command":"factory-reset plan","result":"PLANNED","planHash":"`+planHashFixture+`","expiresAt":"2026-09-19T00:00:00Z","compatible":true,"affectedPlanes":["state","management","host"],"checks":[{"code":"BACKUP_GATE","result":"PASS"},{"code":"SSH_PRESERVATION","result":"PASS"},{"code":"NETWORK_PRESERVATION","result":"PASS"},{"code":"NCP_AGENT_PRESERVATION","result":"PASS"}],"delete":["application-state","installation-identity","credentials"],"preserve":["ssh-access","network-config","ncp-agent"]}\n' ;;
  "restore execute"|"update execute"|"rollback execute"|"factory-reset execute")
    printf '{"schemaVersion":"appliance-backend-payload/v1","command":"%s %s","result":"ACCEPTED","operationId":"op-lifecycle-01","planHash":"`+planHashFixture+`","status":"PENDING","acceptedAt":"2026-09-18T00:00:00Z"}\n' "$1" "$2" ;;
  "update status"|"rollback status")
    printf '{"schemaVersion":"appliance-backend-payload/v1","command":"%s %s","operationId":"op-lifecycle-01","status":"RUNNING","phase":"POSTFLIGHT","observedAt":"2026-09-18T00:00:01Z"}\n' "$1" "$2" ;;
esac
`), "$RECORD", record)
	binary, _ := buildCLIWithBackend(t, script)

	for name, tc := range map[string]struct {
		args       []string
		command    string
		argvPrefix []string
	}{
		"restore plan":       {[]string{"-o", "json", "appliance", "restore", "plan", archive, "--key-file", key}, "appliance.restore.plan", []string{"restore", "plan", archive, "--key-file", key}},
		"update plan":        {[]string{"-o", "json", "appliance", "update", "plan", bundle}, "appliance.update.plan", []string{"update", "plan", bundle}},
		"rollback plan":      {[]string{"-o", "json", "appliance", "rollback", "plan", "v0.9.8.9-rc.1", "--archive", archive, "--key-file", key}, "appliance.rollback.plan", []string{"rollback", "plan", "v0.9.8.9-rc.1", "--archive", archive, "--key-file", key}},
		"factory reset plan": {[]string{"-o", "json", "appliance", "factory-reset", "plan"}, "appliance.factory-reset.plan", []string{"factory-reset", "plan"}},
		"update status":      {[]string{"-o", "json", "appliance", "update", "status", "op-lifecycle-01"}, "appliance.update.status", []string{"update", "status", "op-lifecycle-01"}},
		"restore execute":    {[]string{"-o", "json", "appliance", "restore", "execute", "--plan-hash", planHashFixture, "--confirm", "challenge-1234"}, "appliance.restore.execute", []string{"restore", "execute", "--plan-hash", planHashFixture, "--confirm", "challenge-1234"}},
	} {
		t.Run(name, func(t *testing.T) {
			status, stdout, stderr := runAppliance(t, binary, tc.args...)
			var result struct {
				Command string `json:"command"`
				Success bool   `json:"success"`
			}
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("stdout is not the result envelope (%v): %s", err, stdout)
			}
			if status != 0 || stderr != "" || result.Command != tc.command || !result.Success {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			raw, err := os.ReadFile(record)
			if err != nil {
				t.Fatal(err)
			}
			argv := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(argv) < len(tc.argvPrefix)+3 || !reflect.DeepEqual(argv[:len(tc.argvPrefix)], tc.argvPrefix) ||
				argv[len(argv)-1] != "--json" || argv[len(argv)-3] != "--correlation-id" || !strings.HasPrefix(argv[len(argv)-2], "cli-") {
				t.Fatalf("argv=%#v, want prefix %#v plus correlation/json", argv, tc.argvPrefix)
			}
		})
	}
}

func TestLifecycleInvalidInputNeverCallsBackend(t *testing.T) {
	archive, bundle, key := lifecycleInputFiles(t)
	record := filepath.Join(t.TempDir(), "called")
	binary, _ := buildCLIWithBackend(t, strings.ReplaceAll(`touch "$RECORD"
exit 99
`, "$RECORD", record))

	for name, args := range map[string][]string{
		"relative restore archive": {"appliance", "restore", "plan", "relative.tar", "--key-file", key},
		"relative update bundle":   {"appliance", "update", "plan", "relative.bundle"},
		"flag shaped release":      {"appliance", "rollback", "plan", "--bad", "--archive", archive, "--key-file", key},
		"missing rollback archive": {"appliance", "rollback", "plan", "v1", "--key-file", key},
		"bad plan hash":            {"appliance", "update", "execute", "--plan-hash", "nope", "--confirm", "challenge-1234"},
		"bad challenge":            {"appliance", "factory-reset", "execute", "--plan-hash", planHashFixture, "--confirm", "short"},
		"bad operation id":         {"appliance", "rollback", "status", "../escape"},
		"directory bundle":         {"appliance", "update", "plan", filepath.Dir(bundle)},
	} {
		t.Run(name, func(t *testing.T) {
			status, _, _ := runAppliance(t, binary, args...)
			if status == 0 {
				t.Fatal("invalid input succeeded")
			}
			if _, err := os.Stat(record); !os.IsNotExist(err) {
				t.Fatalf("invalid input reached the backend: %v", err)
			}
		})
	}
}
