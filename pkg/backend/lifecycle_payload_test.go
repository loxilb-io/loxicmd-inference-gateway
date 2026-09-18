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
	"encoding/json"
	"strings"
	"testing"
)

func lifecyclePayload(command string) map[string]any {
	base := map[string]any{"schemaVersion": payloadSchema, "command": command}
	switch {
	case strings.HasSuffix(command, " plan"):
		base["result"] = "PLANNED"
		base["planHash"] = strings.Repeat("a", 64)
		base["expiresAt"] = "2026-09-19T00:00:00Z"
		base["compatible"] = true
		base["affectedPlanes"] = []string{"state", "management"}
		codes := map[string][]string{
			"restore plan":       {"ARCHIVE_AUTHENTICATION", "ARCHIVE_CHECKSUM", "SCHEMA_COMPATIBILITY", "DISK_CAPACITY", "INSTALLATION_COMPATIBILITY", "PRE_RESTORE_BACKUP", "ROLLBACK_FEASIBILITY"},
			"update plan":        {"BUNDLE_SIGNATURE", "PRODUCT_LOCK", "SBOM", "MIGRATION", "BACKUP_GATE"},
			"rollback plan":      {"APPROVED_RELEASE", "BACKUP_GATE", "DOWNGRADE_COMPATIBILITY", "POSTFLIGHT"},
			"factory-reset plan": {"BACKUP_GATE", "SSH_PRESERVATION", "NETWORK_PRESERVATION", "NCP_AGENT_PRESERVATION"},
		}[command]
		checks := make([]map[string]any, 0, len(codes))
		for _, code := range codes {
			checks = append(checks, map[string]any{"code": code, "result": "PASS"})
		}
		base["checks"] = checks
		if command == "factory-reset plan" {
			base["delete"] = []string{"application-state", "installation-identity", "credentials"}
			base["preserve"] = []string{"ssh-access", "network-config", "ncp-agent"}
		}
	case strings.HasSuffix(command, " execute"):
		base["result"] = "ACCEPTED"
		base["operationId"] = "op-lifecycle-01"
		base["planHash"] = strings.Repeat("a", 64)
		base["status"] = "PENDING"
		base["acceptedAt"] = "2026-09-18T00:00:00Z"
	case strings.HasSuffix(command, " status"):
		base["operationId"] = "op-lifecycle-01"
		base["status"] = "RUNNING"
		base["phase"] = "POSTFLIGHT"
		base["observedAt"] = "2026-09-18T00:00:01Z"
	}
	return base
}

func marshalLifecyclePayload(t *testing.T, document map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLifecyclePayloadsAcceptExactPositiveDocuments(t *testing.T) {
	for _, command := range []string{
		"restore plan", "restore execute", "update plan", "update execute", "update status",
		"rollback plan", "rollback execute", "rollback status", "factory-reset plan", "factory-reset execute",
	} {
		t.Run(command, func(t *testing.T) {
			raw := marshalLifecyclePayload(t, lifecyclePayload(command))
			validated, err := ValidatePayload(tuple(command), raw)
			if err != nil {
				t.Fatal(err)
			}
			if string(validated.Document) != string(raw) {
				t.Fatal("validated payload was not preserved byte-for-byte")
			}
		})
	}
}

func TestLifecyclePayloadRedTwinsFailClosed(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"wrong command":        func(d map[string]any) { d["command"] = "update execute" },
		"unknown key":          func(d map[string]any) { d["unexpected"] = true },
		"secret key":           func(d map[string]any) { d["password"] = "CLI_WP00_SYNTHETIC_SECRET_DO_NOT_EMIT" },
		"invalid plan hash":    func(d map[string]any) { d["planHash"] = "short" },
		"failed compatibility": func(d map[string]any) { d["compatible"] = false },
		"invalid check": func(d map[string]any) {
			d["checks"].([]map[string]any)[0]["code"] = "bad"
		},
		"missing required check": func(d map[string]any) {
			d["checks"] = d["checks"].([]map[string]any)[1:]
		},
		"null field": func(d map[string]any) { d["expiresAt"] = nil },
	} {
		t.Run(name, func(t *testing.T) {
			document := lifecyclePayload("restore plan")
			mutate(document)
			if validated, err := ValidatePayload(tuple("restore plan"), marshalLifecyclePayload(t, document)); err == nil || validated != nil {
				t.Fatalf("red twin escaped: validated=%#v err=%v", validated, err)
			}
		})
	}

	for name, command := range map[string]string{
		"execute wrong status": "restore execute",
		"status wrong phase":   "update status",
	} {
		t.Run(name, func(t *testing.T) {
			document := lifecyclePayload(command)
			if strings.HasSuffix(command, " execute") {
				document["status"] = "SUCCEEDED"
			} else {
				document["phase"] = "bad"
			}
			if validated, err := ValidatePayload(tuple(command), marshalLifecyclePayload(t, document)); err == nil || validated != nil {
				t.Fatalf("red twin escaped: validated=%#v err=%v", validated, err)
			}
		})
	}

	t.Run("factory reset loses required preservation", func(t *testing.T) {
		document := lifecyclePayload("factory-reset plan")
		document["preserve"] = []string{"ssh-access", "network-config"}
		if validated, err := ValidatePayload(tuple("factory-reset plan"), marshalLifecyclePayload(t, document)); err == nil || validated != nil {
			t.Fatalf("unsafe factory plan escaped: validated=%#v err=%v", validated, err)
		}
	})
}
