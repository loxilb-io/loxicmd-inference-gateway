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
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// documentFile writes a snapshot document a restore can post.
func documentFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, snapshotDocument(t, "1.5", 7), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const dryRunBody = `{"mode":"dry-run","compatible":true,"schema_version":"1.5",` +
	`"snapshot_gateway_version":"v0.9.9","current_gateway_version":"v0.9.9",` +
	`"snapshot_generation":7,` +
	`"plan":[{"domain":"loadbalancer","to_delete":2,"to_apply":3}],` +
	`"external_dependencies":[{"type":"api-key-db","required":true,"status":"verified"}]}`

const commitBody = `{"mode":"commit","compatible":true,"schema_version":"1.5","result":"ok",` +
	`"persisted":true,"persisted_generation":8,` +
	`"pre_restore_snapshot_persisted":"/etc/loxilb/snapshot.json.pre-restore"}`

func TestRestoreDryRunSucceeds(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, dryRunBody))
	var out bytes.Buffer

	err := Restore(gw.options(), &out, Options{}, RestoreOptions{File: documentFile(t)})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if got := gw.requests[0].Query.Get("mode"); got != api.RestoreModeDryRun {
		t.Fatalf("mode = %q, want dry-run", got)
	}
	text := out.String()
	for _, want := range []string{"nothing was changed", "schema 1.5", "generation 7",
		"loadbalancer", "delete 2, apply 3", "api-key-db (required): verified"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRestoreCommitSucceedsAndReportsDurability(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, commitBody))
	var out bytes.Buffer

	err := Restore(gw.options(), &out, Options{},
		RestoreOptions{File: documentFile(t), Commit: true, Components: "loadbalancer,endpoint"})
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if got := gw.requests[0].Query.Get("mode"); got != api.RestoreModeCommit {
		t.Fatalf("mode = %q, want commit", got)
	}
	if got := gw.requests[0].Query.Get("components"); got != "loadbalancer,endpoint" {
		t.Fatalf("components = %q", got)
	}
	if !strings.Contains(gw.requests[0].Body, "loxilb-snapshot") {
		t.Fatalf("document was not posted: %s", gw.requests[0].Body)
	}
	text := out.String()
	for _, want := range []string{"Restore committed", "Persisted: yes", "generation 8",
		"snapshot.json.pre-restore"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

// TestRestoreCommitThatWasNotPersistedFails is the case the durable contract
// exists for: the configuration was applied but not written through, so the
// next restart loses it. Reporting that as success is the defect.
func TestRestoreCommitThatWasNotPersistedFails(t *testing.T) {
	body := `{"mode":"commit","compatible":true,"result":"ok","schema_version":"1.5","persisted":false,` +
		`"errors":["persist after restore failed: no space left on device"]}`
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, body))
	var out bytes.Buffer

	err := Restore(gw.options(), &out, Options{}, RestoreOptions{File: documentFile(t), Commit: true})
	requireReason(t, err, api.ReasonNotPersisted)
	if !strings.Contains(err.Error(), "will not survive a restart") {
		t.Fatalf("failure does not say what is at stake: %v", err)
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		t.Fatalf("failure dropped the gateway's own detail: %v", err)
	}
}

func TestRestoreFailureClasses(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		commit bool
		reason string
	}{
		"incompatible document": {http.StatusBadRequest,
			`{"mode":"dry-run","compatible":false,"schema_version":"2.0","errors":["schema 2.0 is newer than this gateway"]}`,
			false, api.ReasonIncompatible},
		"required dependency unverifiable": {http.StatusBadRequest,
			`{"mode":"dry-run","compatible":true,"schema_version":"1.5","external_dependencies":[{"type":"api-key-db","id":"keys","required":true,"status":"failed"}],"errors":["api key store not configured"]}`,
			false, api.ReasonDependencyFailed},
		"rolled back": {http.StatusInternalServerError,
			`{"mode":"commit","compatible":true,"schema_version":"1.5","result":"rolled-back","errors":["apply failed at endpoint"]}`,
			true, api.ReasonRolledBack},
		"rollback failed": {http.StatusInternalServerError,
			`{"mode":"commit","compatible":true,"schema_version":"1.5","result":"ROLLBACK-FAILED","errors":["rollback could not restore endpoints"]}`,
			true, api.ReasonRollbackFailed},
		"errors without a result": {http.StatusBadRequest,
			`{"mode":"dry-run","compatible":true,"schema_version":"1.5","errors":["document declares an unknown domain"]}`,
			false, api.ReasonValidationFailed},
		"commit without a result": {http.StatusOK,
			`{"mode":"commit","compatible":true,"schema_version":"1.5"}`,
			true, api.ReasonResultNotOK},
		"gate busy": {http.StatusConflict, `{"message":"Another snapshot or restore operation is in progress"}`,
			false, api.ReasonBusy},
		"maintenance mode": {http.StatusServiceUnavailable, `{"message":"Maintenance mode"}`,
			false, api.ReasonMaintenance},
		"unauthorized": {http.StatusUnauthorized, `{"message":"Invalid authentication credentials"}`,
			false, api.ReasonUnauthorized},
		"bad parameters": {http.StatusBadRequest, `{"message":"Invalid parameters","result":"mode must be dry-run or commit"}`,
			false, api.ReasonBadRequest},
		"body is not a restore result": {http.StatusOK, `{"unrelated":true}`, false, api.ReasonDecodeFailed},
		"body is not json":             {http.StatusOK, `restored`, false, api.ReasonDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			gw := newFakeGateway(t, jsonResponse(tc.status, tc.body))
			var out bytes.Buffer
			err := Restore(gw.options(), &out, Options{},
				RestoreOptions{File: documentFile(t), Commit: tc.commit})
			requireReason(t, err, tc.reason)
			if strings.Contains(out.String(), "Restore committed") {
				t.Fatalf("a failure printed a success line:\n%s", out.String())
			}
		})
	}
}

// TestRestoreSuccessfulBodyBehindAFailureStatus guards the inverse of the
// status-blind decode: a body that reports nothing wrong must not turn a 500
// into success.
func TestRestoreSuccessfulBodyBehindAFailureStatus(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusInternalServerError,
		`{"mode":"dry-run","compatible":true,"schema_version":"1.5"}`))
	var out bytes.Buffer

	err := Restore(gw.options(), &out, Options{}, RestoreOptions{File: documentFile(t)})
	requireReason(t, err, api.ReasonServerError)
}

func TestRestoreLocalInputFailures(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, dryRunBody))

	t.Run("no file", func(t *testing.T) {
		var out bytes.Buffer
		err := Restore(gw.options(), &out, Options{}, RestoreOptions{})
		requireReason(t, err, api.ReasonInvalidArguments)
	})

	t.Run("missing file", func(t *testing.T) {
		var out bytes.Buffer
		err := Restore(gw.options(), &out, Options{},
			RestoreOptions{File: filepath.Join(t.TempDir(), "absent.json")})
		requireReason(t, err, api.ReasonFileRead)
	})

	t.Run("not json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "snapshot.json")
		if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := Restore(gw.options(), &out, Options{}, RestoreOptions{File: path})
		requireReason(t, err, api.ReasonInvalidJSON)
	})

	// None of the local failures may reach the gateway.
	if len(gw.requests) != 0 {
		t.Fatalf("a local failure still sent a request: %+v", gw.requests)
	}
}

func TestRestoreStrictRefusesLegacyCommit(t *testing.T) {
	// A committed restore from an older gateway: applied, but silent about
	// whether the applied state was made durable.
	body := `{"mode":"commit","compatible":true,"schema_version":"1.5","result":"ok"}`
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, body))

	var out bytes.Buffer
	if err := Restore(gw.options(), &out, Options{},
		RestoreOptions{File: documentFile(t), Commit: true}); err != nil {
		t.Fatalf("non-strict commit should succeed: %v", err)
	}
	if strings.Contains(out.String(), "Persisted: yes") {
		t.Fatalf("claimed durability the gateway never reported:\n%s", out.String())
	}
	if !strings.Contains(out.String(), legacyContractNote.Message) {
		t.Fatalf("missing legacy note:\n%s", out.String())
	}

	out.Reset()
	err := Restore(gw.options(), &out, Options{Strict: true},
		RestoreOptions{File: documentFile(t), Commit: true})
	requireReason(t, err, api.ReasonContractLegacy)
}

func TestRestoreJSONFailureEnvelopeCarriesTheBody(t *testing.T) {
	body := `{"mode":"commit","compatible":true,"schema_version":"1.5","result":"rolled-back",` +
		`"errors":["apply failed at endpoint"]}`
	gw := newFakeGateway(t, jsonResponse(http.StatusInternalServerError, body))
	var out bytes.Buffer

	err := Restore(gw.options(), &out, Options{JSON: true},
		RestoreOptions{File: documentFile(t), Commit: true})
	requireReason(t, err, api.ReasonRolledBack)

	doc := decodeEnvelope(t, out.Bytes())
	if doc.Data.Restore == nil || doc.Data.Restore.Result != api.RestoreResultRolledBack {
		t.Fatalf("failure envelope dropped the evidence: %s", out.String())
	}
	if len(doc.Data.Restore.Errors) == 0 {
		t.Fatalf("failure envelope dropped the gateway's errors: %s", out.String())
	}
	if doc.Data.HTTPStatus != http.StatusInternalServerError {
		t.Fatalf("httpStatus = %d", doc.Data.HTTPStatus)
	}
}
