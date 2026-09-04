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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

const durablePersistBody = `{"result":"ok","path":"/etc/loxilb/snapshot.json",` +
	`"checksum":"sha256:abc","schema_version":"1.5","generation":7,` +
	`"included_domains":["loadbalancer","endpoint"],"excluded_domains":["conntrack"],` +
	`"external_dependencies":[{"type":"api-key-db","id":"keys","required":true,"status":"ready"}]}`

const legacyPersistBody = `{"result":"ok","path":"/etc/loxilb/snapshot.json","checksum":"sha256:abc"}`

func TestPersistSucceedsOnDurableContract(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, durablePersistBody))
	var out, errOut bytes.Buffer

	if err := Persist(gw.options(), &out, &errOut, Options{}, "create persist"); err != nil {
		t.Fatalf("persist failed: %v", err)
	}
	if len(gw.requests) != 1 || gw.requests[0].Method != http.MethodPost ||
		!strings.HasSuffix(gw.requests[0].Path, "/config/persist") {
		t.Fatalf("unexpected request: %+v", gw.requests)
	}
	text := out.String()
	for _, want := range []string{"/etc/loxilb/snapshot.json", "sha256:abc", "schema 1.5", "generation 7",
		"Captured: loadbalancer, endpoint", "Not captured: conntrack", "api-key-db/keys (required): ready"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestPersistReportsLegacyContractWithoutClaimingCoverage(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, legacyPersistBody))
	var out, errOut bytes.Buffer

	if err := Persist(gw.options(), &out, &errOut, Options{}, "create persist"); err != nil {
		t.Fatalf("persist failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, legacyContractNote) {
		t.Fatalf("legacy answer did not carry the note:\n%s", text)
	}
	// The gateway reported no coverage, so the CLI must not print one.
	if strings.Contains(text, "Captured:") || strings.Contains(text, "generation") {
		t.Fatalf("legacy answer claimed coverage it never received:\n%s", text)
	}
}

func TestPersistStrictRefusesLegacyContract(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, legacyPersistBody))
	var out, errOut bytes.Buffer

	err := Persist(gw.options(), &out, &errOut, Options{Strict: true}, "create persist")
	requireReason(t, err, api.ReasonContractLegacy)
	if !strings.Contains(errOut.String(), "Error:") {
		t.Fatalf("failure was not reported on stderr: %q", errOut.String())
	}
}

func TestPersistFailureClasses(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		reason string
	}{
		"result missing":     {http.StatusOK, `{"path":"/etc/loxilb/snapshot.json"}`, api.ReasonResultNotOK},
		"result not ok":      {http.StatusOK, `{"result":"failed"}`, api.ReasonResultNotOK},
		"undecodable body":   {http.StatusOK, `{"result":`, api.ReasonDecodeFailed},
		"body is not json":   {http.StatusOK, `persisted!`, api.ReasonDecodeFailed},
		"unauthorized":       {http.StatusUnauthorized, `{"message":"Invalid authentication credentials"}`, api.ReasonUnauthorized},
		"gate busy":          {http.StatusConflict, `{"message":"Another snapshot or restore operation is in progress"}`, api.ReasonBusy},
		"maintenance mode":   {http.StatusServiceUnavailable, `{"message":"Maintenance mode"}`, api.ReasonMaintenance},
		"internal error":     {http.StatusInternalServerError, `{"message":"Internal service error"}`, api.ReasonServerError},
		"unexpected 4xx":     {http.StatusBadRequest, `{"message":"Invalid parameters"}`, api.ReasonBadRequest},
		"empty 200 response": {http.StatusOK, ``, api.ReasonDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			gw := newFakeGateway(t, jsonResponse(tc.status, tc.body))
			var out, errOut bytes.Buffer
			err := Persist(gw.options(), &out, &errOut, Options{}, "create persist")
			requireReason(t, err, tc.reason)
			if strings.Contains(out.String(), "Configuration persisted") {
				t.Fatalf("a failure printed a success line:\n%s", out.String())
			}
		})
	}
}

func TestPersistUnreachableGateway(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, durablePersistBody))
	options := gw.options()
	gw.server.Close() // nothing is listening any more
	var out, errOut bytes.Buffer

	err := Persist(options, &out, &errOut, Options{}, "create persist")
	requireReason(t, err, api.ReasonRequestFailed)
}

func TestPersistJSONEnvelope(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		gw := newFakeGateway(t, jsonResponse(http.StatusOK, durablePersistBody))
		var out, errOut bytes.Buffer
		if err := Persist(gw.options(), &out, &errOut, Options{JSON: true}, "create persist"); err != nil {
			t.Fatalf("persist failed: %v", err)
		}
		report := decodeReport(t, out.Bytes())
		if report.Result != "ok" || report.Reason != api.ReasonOK ||
			report.Contract != api.ContractDurable || report.Command != "create persist" {
			t.Fatalf("unexpected envelope: %+v", report)
		}
		if report.Persist == nil || report.Persist.Generation == nil || *report.Persist.Generation != 7 {
			t.Fatalf("envelope did not carry the persisted identity: %+v", report.Persist)
		}
	})

	t.Run("failure is machine readable", func(t *testing.T) {
		gw := newFakeGateway(t, jsonResponse(http.StatusConflict, `{"message":"busy"}`))
		var out, errOut bytes.Buffer
		err := Persist(gw.options(), &out, &errOut, Options{JSON: true}, "create persist")
		requireReason(t, err, api.ReasonBusy)
		report := decodeReport(t, out.Bytes())
		if report.Result != "error" || report.Reason != api.ReasonBusy || report.HTTPStatus != http.StatusConflict {
			t.Fatalf("unexpected failure envelope: %+v", report)
		}
		// Automation asked for JSON, so the failure belongs on the
		// stream it is reading.
		if errOut.Len() != 0 {
			t.Fatalf("json mode also wrote prose to stderr: %q", errOut.String())
		}
	})
}

// TestPersistAliasIsTheSameCall proves 'save --api' and 'create persist' send
// the same request and reach the same verdict; only the reported command name
// differs.
func TestPersistAliasIsTheSameCall(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, durablePersistBody))
	var canonical, alias bytes.Buffer
	var errOut bytes.Buffer

	if err := Persist(gw.options(), &canonical, &errOut, Options{}, "create persist"); err != nil {
		t.Fatal(err)
	}
	if err := Persist(gw.options(), &alias, &errOut, Options{}, "save --api"); err != nil {
		t.Fatal(err)
	}
	if canonical.String() != alias.String() {
		t.Fatalf("alias output differs:\ncanonical: %s\nalias:     %s", canonical.String(), alias.String())
	}
	if len(gw.requests) != 2 || gw.requests[0].Path != gw.requests[1].Path ||
		gw.requests[0].Method != gw.requests[1].Method {
		t.Fatalf("alias sent a different request: %+v", gw.requests)
	}
}

func decodeReport(t *testing.T, b []byte) api.LifecycleReport {
	t.Helper()
	var report api.LifecycleReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("output is not a JSON report (%v): %s", err, string(b))
	}
	return report
}
