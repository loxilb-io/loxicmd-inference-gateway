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
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

const auditStatusRunningBody = `{"available":true,"running":true,"boot_id":"boot-abc",` +
	`"seq_high":1024,"last_write":"2025-01-01T00:00:02Z",` +
	`"accepted":{"mgmt":1000,"data":24},` +
	`"dropped":[{"stream":"data","reason":"queue_full","count":3}],` +
	`"result_drops":1,"rotations":4,"sealed_bytes":1048576,` +
	`"segment":{"uuid":"seg-1","opened":"2025-01-01T00:00:00Z","records":24,"bytes":8192},` +
	`"retention":{"max_age_seconds":2592000,"max_bytes":10485760,"reserve_bytes":1048576},` +
	`"projected_retention_days":30.5}`

const auditSinkOnBody = `{"enabled":true,"address":"siem.example.com:6514",` +
	`"ca_bundle_path":"/etc/loxilb/siem-ca.pem","facility":13,"max_frame_bytes":8192,` +
	`"connected":true,"submitted":1024}`

func TestAuditStatusGetReadsStatusEndpoint(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, auditStatusRunningBody))
	var out bytes.Buffer
	if err := AuditStatusGet(gw.options(), &out, false); err != nil {
		t.Fatalf("get audit-status failed: %v", err)
	}
	got := gw.requests[0]
	if got.Method != http.MethodGet || !strings.HasSuffix(got.Path, "/audit/status") {
		t.Fatalf("CLI sent %s %s, want GET .../audit/status", got.Method, got.Path)
	}
	text := out.String()
	for _, want := range []string{
		"available, running",
		"Boot id: boot-abc",
		"Accepted: data 24, mgmt 1000",
		"data/queue_full: 3",
		"Active segment: seg-1",
		"Projected retention: 30.5 days",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendering is missing %q:\n%s", want, text)
		}
	}
	// A lost result is a change whose outcome is not in the trail. It must
	// never be folded into the generic counter line.
	if !strings.Contains(text, "Result records LOST after the change was applied: 1") {
		t.Fatalf("a lost result record was not called out:\n%s", text)
	}
}

// The unavailable answer is the one an operator acts on: every other field
// describes nothing, so the rendering must say that rather than print the
// zeros as though they were measurements.
func TestAuditStatusGetUnavailableDescribesNothingElse(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, `{"available":false,"seq_high":0}`))
	var out bytes.Buffer
	if err := AuditStatusGet(gw.options(), &out, false); err != nil {
		t.Fatalf("get audit-status failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "NOT AVAILABLE") {
		t.Fatalf("an unavailable writer was not reported as such:\n%s", text)
	}
	if strings.Contains(text, "Sequence high") {
		t.Fatalf("fields that describe nothing were printed anyway:\n%s", text)
	}
}

// available is sent on every answer, including the unavailable one. A body
// without it is not a status: rendering "not available" for a response that
// never said so would invent the single fact the command exists to report.
func TestAuditStatusGetBodyWithoutAvailableIsDecodeFailure(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, `{"running":true,"seq_high":5}`))
	var out bytes.Buffer
	requireReason(t, AuditStatusGet(gw.options(), &out, false), api.ReasonDecodeFailed)
}

func TestAuditStatusGetJSONIsVerbatimBody(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, auditStatusRunningBody))
	var out bytes.Buffer
	if err := AuditStatusGet(gw.options(), &out, true); err != nil {
		t.Fatalf("get audit-status failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != auditStatusRunningBody {
		t.Fatalf("JSON mode rewrote the body:\n%s", out.String())
	}
}

func TestAuditSinkGetRendersSessionState(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, auditSinkOnBody))
	var out bytes.Buffer
	if err := AuditSinkGet(gw.options(), &out, false); err != nil {
		t.Fatalf("get audit-sink failed: %v", err)
	}
	got := gw.requests[0]
	if got.Method != http.MethodGet || !strings.HasSuffix(got.Path, "/audit/sink") {
		t.Fatalf("CLI sent %s %s, want GET .../audit/sink", got.Method, got.Path)
	}
	text := out.String()
	if !strings.Contains(text, "enabled, connected") ||
		!strings.Contains(text, "Receiver: siem.example.com:6514") {
		t.Fatalf("unexpected rendering:\n%s", text)
	}
	// Syslog over TLS has no acknowledgement, so this count must not read
	// as a delivery count.
	if !strings.Contains(text, "not a delivery count") {
		t.Fatalf("the submitted count was presented as deliveries:\n%s", text)
	}
}

func TestAuditSinkGetUnconfiguredIsNotAnError(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, `{}`))
	var out bytes.Buffer
	if err := AuditSinkGet(gw.options(), &out, false); err != nil {
		t.Fatalf("an unconfigured sink is a legitimate answer, got: %v", err)
	}
	if !strings.Contains(out.String(), "not configured") {
		t.Fatalf("unexpected rendering of an unconfigured sink:\n%s", out.String())
	}
}

// The endpoint REPLACES the configuration, so the CLI must send a whole one
// every time. This is the test that would fail if someone later made the
// command patch: the body must carry all eight configurable fields, with the
// defaults for the flags that were not given.
func TestAuditSinkSetSendsCompleteConfiguration(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
	var out bytes.Buffer
	so := AuditSinkSetOptions{
		Address:      "siem.example.com:6514",
		CABundlePath: "/etc/loxilb/siem-ca.pem",
		Facility:     DefaultSyslogFacility,
	}
	if err := AuditSinkSet(gw.options(), &out, false, so); err != nil {
		t.Fatalf("set audit-sink failed: %v", err)
	}
	got := gw.requests[0]
	if got.Method != http.MethodPost || !strings.HasSuffix(got.Path, "/audit/sink") {
		t.Fatalf("CLI sent %s %s, want POST .../audit/sink", got.Method, got.Path)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got.Body), &sent); err != nil {
		t.Fatalf("the CLI sent a body that is not JSON: %q", got.Body)
	}
	for _, field := range []string{
		"enabled", "address", "ca_bundle_path", "server_name",
		"client_cert_path", "client_key_path", "max_frame_bytes", "facility",
	} {
		if _, ok := sent[field]; !ok {
			t.Fatalf("the replacement body omits %q, so the gateway would clear it: %s", field, got.Body)
		}
	}
	// The default facility travels as the value the operator will read
	// back, not as a zero the gateway would silently substitute 13 for.
	if string(sent["facility"]) != "13" {
		t.Fatalf("facility was sent as %s, want an explicit 13: %s", sent["facility"], got.Body)
	}
	// The read-only session counters a GET returns are not desired state
	// and must not be echoed back.
	for _, field := range []string{"connected", "submitted", "truncated", "write_errors", "last_error"} {
		if _, ok := sent[field]; ok {
			t.Fatalf("the replacement body sends the read-only field %q: %s", field, got.Body)
		}
	}
}

func TestAuditSinkSetDisableSendsEnabledFalse(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
	var out bytes.Buffer
	if err := AuditSinkSet(gw.options(), &out, false, AuditSinkSetOptions{Disable: true}); err != nil {
		t.Fatalf("set audit-sink --disable failed: %v", err)
	}
	if !strings.Contains(gw.requests[0].Body, `"enabled":false`) {
		t.Fatalf("--disable did not send enabled false: %s", gw.requests[0].Body)
	}
	if !strings.Contains(out.String(), "removed") {
		t.Fatalf("unexpected rendering of a removal:\n%s", out.String())
	}
}

// Each of these is refused before any request: a sink the gateway would
// reject, refused with the flag named.
func TestAuditSinkSetRefusesIncompleteConfigurationLocally(t *testing.T) {
	cases := []struct {
		name string
		so   AuditSinkSetOptions
		want string
	}{
		{"no address", AuditSinkSetOptions{CABundlePath: "/ca.pem"}, "--address"},
		{"no ca bundle", AuditSinkSetOptions{Address: "siem:6514"}, "--ca-bundle"},
		{"half a keypair", AuditSinkSetOptions{
			Address: "siem:6514", CABundlePath: "/ca.pem", ClientCertPath: "/gw.pem",
		}, "--client-key"},
		{"facility too high", AuditSinkSetOptions{
			Address: "siem:6514", CABundlePath: "/ca.pem", Facility: MaxSyslogFacility + 1,
		}, "--facility"},
		{"negative facility", AuditSinkSetOptions{
			Address: "siem:6514", CABundlePath: "/ca.pem", Facility: -1,
		}, "--facility"},
		{"negative frame cap", AuditSinkSetOptions{
			Address: "siem:6514", CABundlePath: "/ca.pem", MaxFrameBytes: -1,
		}, "--max-frame-bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
			var out bytes.Buffer
			err := AuditSinkSet(gw.options(), &out, false, tc.so)
			requireReason(t, err, api.ReasonInvalidArguments)
			// A refusal an operator cannot act on is not much of a
			// refusal: it has to name the flag.
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("the refusal does not name %s: %v", tc.want, err)
			}
			if len(gw.requests) != 0 {
				t.Fatalf("a refused configuration still reached the gateway: %v", gw.requests)
			}
		})
	}
}

// A gateway refusal is carried through as the status error it is, not
// reported as a local argument problem.
func TestAuditSinkSetGatewayRefusalIsAStatusError(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusBadRequest, `{"error":"unreadable CA bundle"}`))
	var out bytes.Buffer
	err := AuditSinkSet(gw.options(), &out, false, AuditSinkSetOptions{
		Address: "siem:6514", CABundlePath: "/ca.pem", Facility: DefaultSyslogFacility,
	})
	if err == nil {
		t.Fatal("a 400 was reported as success")
	}
	if api.ReasonOf(err) == api.ReasonInvalidArguments {
		t.Fatalf("the gateway's refusal was reported as a local argument error: %v", err)
	}
}

// joinCounts must order its keys, or a golden pinning the rendering moves
// with Go's map iteration.
func TestJoinCountsIsStablyOrdered(t *testing.T) {
	m := map[string]int64{"system": 3, "mgmt": 1, "data": 2}
	const want = "data 2, mgmt 1, system 3"
	for i := 0; i < 8; i++ {
		if got := joinCounts(m); got != want {
			t.Fatalf("joinCounts is not stably ordered: got %q, want %q", got, want)
		}
	}
}

// Zero means "no limit" in the contract, so it must not be printed as a 0
// that reads like a limit of nothing.
func TestBoundHelpersSayUnboundedForZero(t *testing.T) {
	if got := boundSeconds(0); got != "unbounded" {
		t.Fatalf("boundSeconds(0) = %q", got)
	}
	if got := boundBytes(0); got != "unbounded" {
		t.Fatalf("boundBytes(0) = %q", got)
	}
	if got := boundSeconds(60); got != "60s" {
		t.Fatalf("boundSeconds(60) = %q", got)
	}
	if got := boundBytes(4096); got != "4096 bytes" {
		t.Fatalf("boundBytes(4096) = %q", got)
	}
}
