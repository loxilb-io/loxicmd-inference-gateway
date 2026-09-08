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
package lifecycle

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

const maintenanceOnBody = `{"state":"maintenance","operation_id":"maint-1800000000-1",` +
	`"refusing_new_config":true,"refusing_new_inference":false,"in_flight_streams":3,` +
	`"entered_at":"2027-01-15T10:00:00.000Z","elapsed_seconds":42,"drain_timeout_seconds":300,` +
	`"drain_deadline_exceeded":false,"cancellable":true}`

func TestMaintenanceGetRendersGatewayTruth(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, maintenanceOnBody))
	var out bytes.Buffer
	if err := MaintenanceGet(gw.options(), &out, Options{}); err != nil {
		t.Fatalf("get maintenance failed: %v", err)
	}
	if got := gw.requests[0]; got.Method != http.MethodGet || !strings.HasSuffix(got.Path, "/maintenance") {
		t.Fatalf("CLI sent %s %s, want GET .../maintenance", got.Method, got.Path)
	}
	text := out.String()
	for _, want := range []string{
		"Maintenance: maintenance",
		"Operation: maint-1800000000-1",
		"Refusing new config: true",
		"Refusing new inference: false",
		"In-flight streams: 3",
		"Elapsed: 42s of a 300s drain window",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("human output missing %q:\n%s", want, text)
		}
	}
}

func TestMaintenanceSetSendsDeclaredWindowAndReportsJSON(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, maintenanceOnBody))
	var out bytes.Buffer
	err := MaintenanceSet(gw.options(), &out, Options{JSON: true},
		MaintenanceSetOptions{Enable: true, DrainTimeoutSeconds: 300})
	if err != nil {
		t.Fatalf("set maintenance on failed: %v", err)
	}
	var sent api.MaintenanceRequest
	if jerr := json.Unmarshal([]byte(gw.requests[0].Body), &sent); jerr != nil {
		t.Fatalf("request body is not the contract: %v (%s)", jerr, gw.requests[0].Body)
	}
	if !sent.Enabled || sent.DrainTimeoutSeconds != 300 {
		t.Fatalf("sent %+v, want enabled with a 300s window", sent)
	}
	doc := decodeEnvelope(t, out.Bytes())
	if !doc.Success || doc.Command != "set.maintenance.on" {
		t.Fatalf("envelope = %v/%s, want success/set.maintenance.on", doc.Success, doc.Command)
	}
	if doc.Data.Maintenance == nil || doc.Data.Maintenance.State != "maintenance" {
		t.Fatalf("envelope carries no maintenance state: %+v", doc.Data.Maintenance)
	}
	// The gateway reported the episode's operation id; the envelope's
	// correlationId is where automation expects it.
	if doc.CorrelationID != "maint-1800000000-1" {
		t.Fatalf("correlationId = %q, want the gateway's operation id", doc.CorrelationID)
	}
}

func TestMaintenanceLeaveCarriesEndedEpisode(t *testing.T) {
	leaveBody := `{"state":"active","operation_id":"maint-1800000000-1",` +
		`"refusing_new_config":false,"refusing_new_inference":false,"in_flight_streams":0,` +
		`"elapsed_seconds":0,"drain_deadline_exceeded":false,"cancellable":true}`
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, leaveBody))
	var out bytes.Buffer
	err := MaintenanceSet(gw.options(), &out, Options{},
		MaintenanceSetOptions{Enable: false})
	if err != nil {
		t.Fatalf("set maintenance off failed: %v", err)
	}
	if !strings.Contains(out.String(), "Maintenance: active") ||
		!strings.Contains(out.String(), "Operation: maint-1800000000-1") {
		t.Fatalf("leave output does not carry the ended episode:\n%s", out.String())
	}
}

// The load-bearing contract clause: a state-changing call whose outcome is
// unknown must fail with recovery-required — never success, and never a
// generic request failure automation might blindly retry.
func TestMaintenanceSetUnconfirmedIsRecoveryRequired(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, maintenanceOnBody))
	restOptions := gw.options()
	gw.server.Close() // every request from here on breaks at the transport

	var out bytes.Buffer
	err := MaintenanceSet(restOptions, &out, Options{},
		MaintenanceSetOptions{Enable: true})
	requireReason(t, err, api.ReasonRecoveryRequired)
	if !strings.Contains(err.Error(), "get maintenance") {
		t.Fatalf("recovery-required error does not tell the operator how to verify: %v", err)
	}
}

// A read has no unknown-state problem: the same transport failure on GET is
// a plain request failure, not recovery-required.
func TestMaintenanceGetTransportFailureIsRequestFailed(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, maintenanceOnBody))
	restOptions := gw.options()
	gw.server.Close()

	var out bytes.Buffer
	err := MaintenanceGet(restOptions, &out, Options{})
	requireReason(t, err, api.ReasonRequestFailed)
}

func TestMaintenanceStatusErrorsKeepTheirCodes(t *testing.T) {
	cases := []struct {
		status int
		reason string
	}{
		{http.StatusUnauthorized, api.ReasonUnauthorized},
		{http.StatusServiceUnavailable, api.ReasonMaintenance},
		{http.StatusInternalServerError, api.ReasonServerError},
	}
	for _, c := range cases {
		gw := newFakeGateway(t, jsonResponse(c.status,
			`{"code":`+strconv.Itoa(c.status)+`,"message":"x","result":"y"}`))
		var out bytes.Buffer
		err := MaintenanceSet(gw.options(), &out, Options{},
			MaintenanceSetOptions{Enable: true})
		requireReason(t, err, c.reason)
		if api.HTTPStatusOf(err) != c.status {
			t.Fatalf("status %d not preserved: %v", c.status, err)
		}
	}
}

func TestMaintenanceUndecodableSuccessIsDecodeFailed(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, `not json at all`))
	var out bytes.Buffer
	err := MaintenanceGet(gw.options(), &out, Options{})
	requireReason(t, err, api.ReasonDecodeFailed)
}

// A JSON-mode failure must land in the envelope on stdout, not as prose on
// a stream automation is not reading.
func TestMaintenanceJSONFailureIsAnEnvelope(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusServiceUnavailable,
		`{"code":503,"message":"Maintenance mode","result":"booting"}`))
	var out bytes.Buffer
	err := MaintenanceGet(gw.options(), &out, Options{JSON: true})
	if err == nil {
		t.Fatal("503 reported as success")
	}
	doc := decodeEnvelope(t, out.Bytes())
	if doc.Success || doc.Code != "PRECONDITION" ||
		doc.Data.ComponentCode != api.ReasonMaintenance || doc.Data.HTTPStatus != 503 {
		t.Fatalf("failure envelope = %+v, want PRECONDITION/maintenance-mode/503", doc)
	}
}
