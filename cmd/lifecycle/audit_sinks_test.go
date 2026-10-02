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

const auditNamedSinkBody = `{"name":"siem2","address":"siem2.example.com:6514",` +
	`"ca_bundle_path":"/etc/loxilb/siem2-ca.pem","facility":13,"enterprise_number":32473,` +
	`"filter":{"streams":["data"],"services":["chat"],"outcome":"failed","data_sample":10},` +
	`"state":"connected","cursor":{"segment_uuid":"seg-1","seq":1024},` +
	`"xseq_high":88,"xseq_epoch":1735689600,"submitted":90,"filtered":934,"resent":2,"lag_drops":1}`

// A secondary sink the tests can set: everything a sink needs and nothing
// selected away.
func secondarySink() AuditSinkSetOptions {
	return AuditSinkSetOptions{
		Address: "siem2.example.com:6514", CABundlePath: "/etc/loxilb/siem2-ca.pem",
		Facility: DefaultSyslogFacility, EnterpriseNumber: 32473,
	}
}

func TestAuditNamedSinkGetReadsTheSinkByName(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, auditNamedSinkBody))
	var out bytes.Buffer
	if err := AuditNamedSinkGet(gw.options(), &out, false, "siem2"); err != nil {
		t.Fatalf("get audit-sink siem2 failed: %v", err)
	}
	got := gw.requests[0]
	if got.Method != http.MethodGet || !strings.HasSuffix(got.Path, "/audit/sinks/siem2") {
		t.Fatalf("CLI sent %s %s, want GET .../audit/sinks/siem2", got.Method, got.Path)
	}
	text := out.String()
	for _, want := range []string{
		"Audit sink siem2: secondary, connected",
		"Receiver: siem2.example.com:6514",
		"Enterprise number: 32473",
		"Selects: streams data; data records of services chat; outcome failed; one data record in 10",
		"Past record: seq 1024 in segment seg-1",
		"Export sequence: high 88, epoch 1735689600",
		"not a delivery count",
		"Sent again after a lost session: 2",
		"Kept from this sink by its filter: 934",
		"Segments LOST to retention before they were sent: 1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendering is missing %q:\n%s", want, text)
		}
	}
}

func TestAuditNamedSinkGetJSONIsVerbatimBody(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, auditNamedSinkBody))
	var out bytes.Buffer
	if err := AuditNamedSinkGet(gw.options(), &out, true, "siem2"); err != nil {
		t.Fatalf("get audit-sink siem2 failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != auditNamedSinkBody {
		t.Fatalf("JSON mode rewrote the body:\n%s", out.String())
	}
}

// A sink with no filter is sent everything, and the rendering says so
// rather than leaving the line out.
func TestAuditNamedSinkGetWithoutFilterSelectsEveryRecord(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK,
		`{"name":"siem2","address":"siem2:6514","ca_bundle_path":"/ca.pem","enterprise_number":32473,"state":"starting"}`))
	var out bytes.Buffer
	if err := AuditNamedSinkGet(gw.options(), &out, false, "siem2"); err != nil {
		t.Fatalf("get audit-sink siem2 failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Selects: every record") || !strings.Contains(text, "Past record: none yet") {
		t.Fatalf("an unfiltered sink that has sent nothing was rendered as:\n%s", text)
	}
}

// A configured sink always has a receiver. A body without one is not a
// sink, and must not be printed as an empty one.
func TestAuditNamedSinkGetBodyWithoutReceiverIsDecodeFailure(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":"siem2","state":"connected"}`, `not json`} {
		gw := newFakeGateway(t, jsonResponse(http.StatusOK, body))
		var out bytes.Buffer
		requireReason(t, AuditNamedSinkGet(gw.options(), &out, false, "siem2"), api.ReasonDecodeFailed)
		if out.Len() != 0 {
			t.Fatalf("body %s was rendered anyway:\n%s", body, out.String())
		}
	}
}

func TestAuditNamedSinkGetUnknownNameIsTheGatewaysNotFound(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusNotFound, `{"code":404,"message":"no audit sink of that name"}`))
	var out bytes.Buffer
	err := AuditNamedSinkGet(gw.options(), &out, false, "siem2")
	if err == nil || !strings.Contains(err.Error(), "no audit sink of that name") {
		t.Fatalf("a 404 was reported as %v", err)
	}
}

func TestAuditNamedSinkSetPutsTheWholeSinkUnderItsName(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
	so := secondarySink()
	so.ServerName = "siem2.corp.example.com"
	so.MaxFrameBytes = 8192
	so.Streams = []string{"data", "mgmt"}
	so.Services = []string{"chat"}
	so.Outcome = "failed"
	so.DataSample = 10
	var out bytes.Buffer
	if err := AuditNamedSinkSet(gw.options(), &out, false, "siem2", so); err != nil {
		t.Fatalf("set audit-sink siem2 failed: %v", err)
	}
	got := gw.requests[0]
	if got.Method != http.MethodPut || !strings.HasSuffix(got.Path, "/audit/sinks/siem2") {
		t.Fatalf("CLI sent %s %s, want PUT .../audit/sinks/siem2", got.Method, got.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(got.Body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %s", got.Body)
	}
	for field, want := range map[string]any{
		"address": "siem2.example.com:6514", "ca_bundle_path": "/etc/loxilb/siem2-ca.pem",
		"server_name": "siem2.corp.example.com", "max_frame_bytes": 8192.0,
		"facility": 13.0, "enterprise_number": 32473.0,
	} {
		if sent[field] != want {
			t.Fatalf("%s was sent as %v, want %v: %s", field, sent[field], want, got.Body)
		}
	}
	filter, _ := json.Marshal(sent["filter"])
	if string(filter) != `{"data_sample":10,"outcome":"failed","services":["chat"],"streams":["data","mgmt"]}` {
		t.Fatalf("the filter was sent as %s", filter)
	}
	// The name travels in the path and the state is the gateway's to
	// report; neither belongs in a configuration.
	for _, field := range []string{"name", "enabled", "state", "cursor", "xseq_high", "xseq_epoch",
		"submitted", "filtered", "resent", "poison", "truncated", "write_errors", "lag_drops", "last_error"} {
		if _, ok := sent[field]; ok {
			t.Fatalf("the body sends %q: %s", field, got.Body)
		}
	}
	if !strings.Contains(out.String(), "Audit sink siem2 set") ||
		!strings.Contains(out.String(), "one data record in 10") {
		t.Fatalf("unexpected rendering:\n%s", out.String())
	}
}

// No filter flag means no filter: nothing is sent that could be read as a
// selection, and sampling one in one is the same as not sampling.
func TestAuditNamedSinkSetWithoutSelectionSendsNoFilter(t *testing.T) {
	for _, sample := range []int64{0, 1} {
		gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
		so := secondarySink()
		so.DataSample = sample
		var out bytes.Buffer
		if err := AuditNamedSinkSet(gw.options(), &out, false, "siem2", so); err != nil {
			t.Fatalf("set audit-sink siem2 failed: %v", err)
		}
		if strings.Contains(gw.requests[0].Body, "filter") {
			t.Fatalf("--data-sample %d sent a filter: %s", sample, gw.requests[0].Body)
		}
		if !strings.Contains(out.String(), "every record") {
			t.Fatalf("unexpected rendering:\n%s", out.String())
		}
	}
}

// The gateway answers each of these with a 400 that carries no reason, so
// each is refused here, before any request, with the flag named.
func TestAuditNamedSinkSetRefusesLocally(t *testing.T) {
	with := func(f func(*AuditSinkSetOptions)) AuditSinkSetOptions {
		so := secondarySink()
		f(&so)
		return so
	}
	cases := []struct {
		label string
		name  string
		so    AuditSinkSetOptions
		want  string
	}{
		{"reserved name", "compliance", secondarySink(), "reserved"},
		{"upper case name", "Siem2", secondarySink(), "sink name"},
		{"name with a slash", "a/b", secondarySink(), "sink name"},
		{"empty name", "", secondarySink(), "sink name"},
		{"name of 65", strings.Repeat("a", 65), secondarySink(), "sink name"},
		{"disable", "siem2", with(func(so *AuditSinkSetOptions) { so.Disable = true }), "delete audit-sink siem2"},
		{"no address", "siem2", with(func(so *AuditSinkSetOptions) { so.Address = "" }), "--address"},
		{"no ca bundle", "siem2", with(func(so *AuditSinkSetOptions) { so.CABundlePath = "" }), "--ca-bundle"},
		{"half a keypair", "siem2", with(func(so *AuditSinkSetOptions) { so.ClientKeyPath = "/k.pem" }), "--client-cert"},
		{"no enterprise number", "siem2", with(func(so *AuditSinkSetOptions) { so.EnterpriseNumber = 0 }), "--enterprise-number"},
		{"enterprise number over 32 bits", "siem2", with(func(so *AuditSinkSetOptions) { so.EnterpriseNumber = MaxEnterpriseNumber + 1 }), "--enterprise-number"},
		{"unknown stream", "siem2", with(func(so *AuditSinkSetOptions) { so.Streams = []string{"data", "system"} }), "--stream"},
		{"empty service", "siem2", with(func(so *AuditSinkSetOptions) { so.Services = []string{" "} }), "--service"},
		{"unknown outcome", "siem2", with(func(so *AuditSinkSetOptions) { so.Outcome = "error" }), "--outcome"},
		{"negative sample", "siem2", with(func(so *AuditSinkSetOptions) { so.DataSample = -1 }), "--data-sample"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
			var out bytes.Buffer
			err := AuditNamedSinkSet(gw.options(), &out, false, tc.name, tc.so)
			requireReason(t, err, api.ReasonInvalidArguments)
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("the refusal does not say %q: %v", tc.want, err)
			}
			if len(gw.requests) != 0 {
				t.Fatalf("a refused configuration still reached the gateway: %v", gw.requests)
			}
		})
	}
}

// The limits themselves are accepted: the longest name and both ends of the
// enterprise number's range.
func TestAuditNamedSinkSetAcceptsTheLimits(t *testing.T) {
	for _, pen := range []int64{1, MaxEnterpriseNumber} {
		gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
		so := secondarySink()
		so.EnterpriseNumber = pen
		var out bytes.Buffer
		name := strings.Repeat("a", 61) + "-_9"
		if err := AuditNamedSinkSet(gw.options(), &out, false, name, so); err != nil {
			t.Fatalf("enterprise number %d under a 64 character name was refused: %v", pen, err)
		}
		if len(gw.requests) != 1 {
			t.Fatalf("%d requests, want 1", len(gw.requests))
		}
	}
}

// The compliance sink is sent every record, unnumbered. A flag that selects
// or numbers must not be dropped silently when the name was forgotten: the
// operator would have replaced the compliance sink believing a secondary
// had been made.
func TestAuditSinkSetRefusesSecondaryFlagsWithoutAName(t *testing.T) {
	base := AuditSinkSetOptions{Address: "siem:6514", CABundlePath: "/ca.pem", Facility: DefaultSyslogFacility}
	cases := map[string]func(*AuditSinkSetOptions){
		"--enterprise-number": func(so *AuditSinkSetOptions) { so.EnterpriseNumber = 32473 },
		"--stream":            func(so *AuditSinkSetOptions) { so.Streams = []string{"mgmt"} },
		"--service":           func(so *AuditSinkSetOptions) { so.Services = []string{"chat"} },
		"--outcome":           func(so *AuditSinkSetOptions) { so.Outcome = "ok" },
		"--data-sample":       func(so *AuditSinkSetOptions) { so.DataSample = 10 },
	}
	for flag, set := range cases {
		for _, disable := range []bool{false, true} {
			gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
			so := base
			so.Disable = disable
			set(&so)
			var out bytes.Buffer
			err := AuditSinkSet(gw.options(), &out, false, so)
			requireReason(t, err, api.ReasonInvalidArguments)
			if !strings.Contains(err.Error(), flag) {
				t.Fatalf("the refusal does not name %s: %v", flag, err)
			}
			if len(gw.requests) != 0 {
				t.Fatalf("%s reached the compliance sink's endpoint: %v", flag, gw.requests)
			}
		}
	}
}

func TestAuditNamedSinkSetGatewayRefusalIsAStatusError(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusBadRequest, ""))
	var out bytes.Buffer
	err := AuditNamedSinkSet(gw.options(), &out, false, "siem2", secondarySink())
	if err == nil {
		t.Fatal("a 400 was reported as success")
	}
	if api.ReasonOf(err) == api.ReasonInvalidArguments {
		t.Fatalf("the gateway's refusal was reported as a local argument error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("a refused change printed:\n%s", out.String())
	}
}

// A change whose outcome this process cannot know is never a success.
func TestAuditNamedSinkChangeWithoutAnAnswerIsRecoveryRequired(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
	opts := gw.options()
	gw.server.Close()
	var out bytes.Buffer
	requireReason(t, AuditNamedSinkSet(opts, &out, false, "siem2", secondarySink()), api.ReasonRecoveryRequired)
	requireReason(t, AuditNamedSinkDelete(opts, &out, false, "siem2"), api.ReasonRecoveryRequired)
	if out.Len() != 0 {
		t.Fatalf("an unconfirmed change printed:\n%s", out.String())
	}
}

func TestAuditNamedSinkDeleteRemovesTheSinkByName(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusNoContent, ""))
	var out bytes.Buffer
	if err := AuditNamedSinkDelete(gw.options(), &out, false, "siem2"); err != nil {
		t.Fatalf("delete audit-sink siem2 failed: %v", err)
	}
	got := gw.requests[0]
	if got.Method != http.MethodDelete || !strings.HasSuffix(got.Path, "/audit/sinks/siem2") {
		t.Fatalf("CLI sent %s %s, want DELETE .../audit/sinks/siem2", got.Method, got.Path)
	}
	if !strings.Contains(out.String(), "Audit sink siem2 removed") {
		t.Fatalf("unexpected rendering:\n%s", out.String())
	}
}

func TestAuditNamedSinkDeleteUnknownNameIsNotASuccess(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusNotFound, `{"code":404,"message":"no audit sink of that name"}`))
	var out bytes.Buffer
	if err := AuditNamedSinkDelete(gw.options(), &out, false, "siem2"); err == nil {
		t.Fatal("a 404 was reported as success")
	}
	if out.Len() != 0 {
		t.Fatalf("a refused removal printed:\n%s", out.String())
	}
}

// The compliance sink has no name here; asking for it by the reserved one
// must not turn into a request for a secondary called compliance.
func TestAuditNamedSinkReadAndRemoveRefuseTheReservedName(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, auditNamedSinkBody))
	var out bytes.Buffer
	requireReason(t, AuditNamedSinkGet(gw.options(), &out, false, "compliance"), api.ReasonInvalidArguments)
	requireReason(t, AuditNamedSinkDelete(gw.options(), &out, false, "compliance"), api.ReasonInvalidArguments)
	requireReason(t, AuditNamedSinkDelete(gw.options(), &out, false, "../sink"), api.ReasonInvalidArguments)
	if len(gw.requests) != 0 {
		t.Fatalf("a refused name still reached the gateway: %v", gw.requests)
	}
}

const auditStatusSinksBody = `{"available":true,"running":true,"seq_high":1024,"compliance_sink":true,` +
	`"sinks":[{"name":"compliance","compliance":true,"state":"connected",` +
	`"cursor":{"segment_uuid":"seg-2","seq":1020},"in_active_segment":true,"lag_records":4},` +
	`{"name":"siem2","state":"disconnected","cursor":{"segment_uuid":"seg-1","seq":600},"lag_drops":2},` +
	`{"name":"fresh","state":"starting","in_active_segment":true}]}`

func TestAuditStatusRendersEverySinksProgress(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, auditStatusSinksBody))
	var out bytes.Buffer
	if err := AuditStatusGet(gw.options(), &out, false); err != nil {
		t.Fatalf("get audit-status failed: %v", err)
	}
	text := out.String()
	for _, want := range []string{
		"    compliance (compliance): connected, past seq 1020 in seg-2, 4 records behind\n",
		// lag_records is not measured for a sink in a sealed segment: its
		// zero must not be printed as "0 records behind".
		"    siem2: disconnected, past seq 600 in seg-1, still reading sealed segments, 2 segments LOST to retention before they were sent\n",
		"    fresh: starting, 0 records behind\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendering is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "NOT CONFIGURED") {
		t.Fatalf("a configured compliance sink was reported missing:\n%s", text)
	}
}

// compliance_sink false is the answer an operator acts on. A gateway that
// does not send the field at all never said it, and is not reported as
// having no compliance sink.
func TestAuditStatusSaysWhenNoComplianceSinkIsConfigured(t *testing.T) {
	for body, want := range map[string]bool{
		`{"available":true,"running":true,"compliance_sink":false}`: true,
		`{"available":true,"running":true,"compliance_sink":true}`:  false,
		`{"available":true,"running":true}`:                         false,
	} {
		gw := newFakeGateway(t, jsonResponse(http.StatusOK, body))
		var out bytes.Buffer
		if err := AuditStatusGet(gw.options(), &out, false); err != nil {
			t.Fatalf("get audit-status failed: %v", err)
		}
		if got := strings.Contains(out.String(), "Compliance sink: NOT CONFIGURED"); got != want {
			t.Fatalf("body %s: missing-sink line present=%v, want %v:\n%s", body, got, want, out.String())
		}
	}
}
