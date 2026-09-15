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
package appliance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/backend"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

func wp03OperationError(t *testing.T, exit int, code, componentCode, correlationID string) []byte {
	t.Helper()
	document := map[string]any{
		"schemaVersion": "appliance-backend-payload/v1",
		"command":       "status",
		"result":        "ERROR",
		"exit":          exit,
		"code":          code,
		"origin":        "backend",
		"componentCode": componentCode,
		"retryable":     exit == 5,
	}
	if correlationID != "" {
		document["correlationId"] = correlationID
	}
	if exit == 8 {
		document["result"] = "PARTIAL"
		document["operationId"] = "op-status-partial-01"
		document["recoveryGuidance"] = "Inspect status dependencies before retrying."
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCLIWP03DispatcherPreservesStructuredTaxonomy(t *testing.T) {
	for exit, code := range map[int]string{
		2: "INVALID_ARGUMENT",
		3: "AUTH",
		4: "PRECONDITION",
		5: "UNAVAILABLE",
		6: "CONTRACT_MISMATCH",
		7: "FAILED",
		8: "PARTIAL",
	} {
		t.Run(fmt.Sprintf("exit-%d", exit), func(t *testing.T) {
			res := &backend.Result{
				Stdout:        wp03OperationError(t, exit, code, "STATUS_DECIDED_ERROR", "cli-wp03-01"),
				ExitCode:      exit,
				CorrelationID: "cli-wp03-01",
			}
			validated, err := resolveBackendOutcome("status", res, false, true)
			if validated == nil || validated.OperationError == nil {
				t.Fatalf("validated operation error was not preserved: %#v", validated)
			}
			classified := exitcode.Classify(err)
			if classified.Code != exitcode.Code(exit) || classified.ComponentCode != "STATUS_DECIDED_ERROR" {
				t.Fatalf("exit %d flattened to %+v", exit, classified)
			}
		})
	}
}

func TestCLIWP03DispatcherRejectsOutcomeMismatches(t *testing.T) {
	validSuccess := []byte(validStatusPayload)
	validError := wp03OperationError(t, 5, "UNAVAILABLE", "STATUS_DEPENDENCY_UNAVAILABLE", "cli-wp03-01")
	for name, res := range map[string]*backend.Result{
		"schema-invalid-json": {
			Stdout: []byte(`{"release":"v0.9.8.9-rc.1"}`), ExitCode: 0, CorrelationID: "cli-wp03-01",
		},
		"error-document-on-success": {
			Stdout: validError, ExitCode: 0, CorrelationID: "cli-wp03-01",
		},
		"success-document-on-error": {
			Stdout: validSuccess, ExitCode: 4, CorrelationID: "cli-wp03-01",
		},
		"document-process-exit-mismatch": {
			Stdout: validError, ExitCode: 4, CorrelationID: "cli-wp03-01",
		},
		"correlation-mismatch": {
			Stdout: validError, ExitCode: 5, CorrelationID: "cli-wp03-other",
		},
		"second-json-document": {
			Stdout: append(append([]byte(nil), validSuccess...), []byte(`{}`)...), ExitCode: 0, CorrelationID: "cli-wp03-01",
		},
	} {
		t.Run(name, func(t *testing.T) {
			validated, err := resolveBackendOutcome("status", res, false, true)
			if validated != nil {
				t.Fatalf("invalid backend bytes were authorized: %s", validated.Document)
			}
			var payloadErr *backend.PayloadValidationError
			if !errors.As(err, &payloadErr) {
				t.Fatalf("error type = %T, want PayloadValidationError", err)
			}
			classified := exitcode.Classify(err)
			if classified.Code != exitcode.ContractMismatch || classified.ComponentCode != backend.CodeBackendPayloadInvalid {
				t.Fatalf("classification = %+v", classified)
			}
			if payloadErr.ObservedBackendExit == nil || *payloadErr.ObservedBackendExit != res.ExitCode || payloadErr.CorrelationID != res.CorrelationID {
				t.Fatalf("execution evidence lost: %+v", payloadErr)
			}
		})
	}
}

func TestCLIWP03DispatcherActivePathRejectsSchemaInvalidDocument(t *testing.T) {
	binary, _ := buildCLIWithBackend(t, withValidHandshake(`
case "$*" in
*--json*) echo '{"release":"v0.9.8.9-rc.1"}' ;;
*) echo 'profile missing' >&2; exit 4 ;;
esac
`))

	t.Run("json-valid is not schema-valid", func(t *testing.T) {
		status, stdout, stderr := runAppliance(t, binary, "appliance", "status", "-o", "json")
		if status != 6 {
			t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
		}
		data := dataOf(t, envelopeOf(t, stdout))
		if data["componentCode"] != backend.CodeBackendPayloadInvalid {
			t.Fatalf("payload violation not classified: %s", stdout)
		}
		if _, present := data["backend"]; present {
			t.Fatalf("unvalidated backend document leaked into envelope: %s", stdout)
		}
	})

	t.Run("human mode preserves public backend exit", func(t *testing.T) {
		status, _, stderr := runAppliance(t, binary, "appliance", "status")
		if status != 4 || !strings.Contains(stderr, "profile missing") {
			t.Fatalf("status=%d stderr=%q, want exit 4 preserved", status, stderr)
		}
	})
}

func TestCLIWP03DispatcherActivePathPreservesStructuredError(t *testing.T) {
	document := string(wp03OperationError(t, 5, "UNAVAILABLE", "STATUS_DEPENDENCY_UNAVAILABLE", ""))
	binary, _ := buildCLIWithBackend(t, withValidHandshake("echo '"+document+"'\nexit 5\n"))

	status, stdout, stderr := runAppliance(t, binary, "appliance", "status", "-o", "json")
	if status != 5 {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	doc := envelopeOf(t, stdout)
	if doc["code"] != "UNAVAILABLE" {
		t.Fatalf("envelope taxonomy flattened: %s", stdout)
	}
	data := dataOf(t, doc)
	if data["componentCode"] != "STATUS_DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("component code lost: %s", stdout)
	}
	backendDoc, ok := data["backend"].(map[string]any)
	if !ok || backendDoc["exit"] != float64(5) {
		t.Fatalf("validated backend error was not preserved: %s", stdout)
	}
}

func TestCLIWP03MutatingDispatcherPreservesPartialOperationID(t *testing.T) {
	document := `{"schemaVersion":"appliance-backend-payload/v1","command":"backup create","result":"PARTIAL","exit":8,"code":"PARTIAL","origin":"backend","componentCode":"BACKUP_ARCHIVE_INCOMPLETE","retryable":false,"operationId":"op-backup-partial-03","recoveryGuidance":"Inspect and remove the incomplete archive before retrying."}`
	binary, _ := buildCLIWithBackend(t, withValidHandshake("echo '"+document+"'\nexit 8\n"))
	key := filepath.Join(t.TempDir(), "backup.key")
	if err := os.WriteFile(key, []byte("fixture-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	status, stdout, stderr := runAppliance(t, binary, "appliance", "backup", "create", "/tmp/wp03.tar", "--key-file", key, "-o", "json")
	if status != 8 {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	doc := envelopeOf(t, stdout)
	data := dataOf(t, doc)
	if doc["code"] != "PARTIAL" || data["operationId"] != "op-backup-partial-03" ||
		data["componentCode"] != "BACKUP_ARCHIVE_INCOMPLETE" {
		t.Fatalf("partial operation evidence lost: %s", stdout)
	}
}
