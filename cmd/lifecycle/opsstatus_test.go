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
	"net/http"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

const readyBody = `{"ready":true,"reasons":[],` +
	`"ebpf_attachments":[{"name":"eno1","mode":"tc","attached":true},` +
	`{"name":"eno2","mode":"tc","attached":false}]}`

const notReadyBody = `{"ready":false,"reasons":["dependency keystore: connection refused"],` +
	`"ebpf_attachments":[]}`

func TestReadyGetRendersVerdictAndAttachments(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, readyBody))
	var out bytes.Buffer
	if err := ReadyGet(gw.options(), &out, false); err != nil {
		t.Fatalf("get ready failed on a ready gateway: %v", err)
	}
	if got := gw.requests[0]; got.Method != http.MethodGet || !strings.HasSuffix(got.Path, "/status/ready") {
		t.Fatalf("CLI sent %s %s, want GET .../status/ready", got.Method, got.Path)
	}
	text := out.String()
	for _, want := range []string{"Ready: true", "eno1 (tc): attached", "eno2 (tc): NOT ATTACHED"} {
		if !strings.Contains(text, want) {
			t.Fatalf("human output missing %q:\n%s", want, text)
		}
	}
}

// A 503 with the contract body is the NOT-READY verdict, not a request
// failure: the body still renders, and the exit status carries the
// verdict for automation probing with exit codes alone.
func TestReadyGetNotReadyRendersAndExitsNonZero(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusServiceUnavailable, notReadyBody))
	var out bytes.Buffer
	err := ReadyGet(gw.options(), &out, false)
	requireReason(t, err, api.ReasonResultNotOK)
	if api.HTTPStatusOf(err) != http.StatusServiceUnavailable {
		t.Fatalf("verdict error lost its HTTP status: %v", err)
	}
	if !strings.Contains(out.String(), "dependency keystore") {
		t.Fatalf("not-ready reasons were not rendered:\n%s", out.String())
	}
}

// JSON mode passes the gateway's body through verbatim - the contract
// body is the machine interface - while the exit status still carries
// the verdict.
func TestReadyGetJSONIsVerbatimBody(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusServiceUnavailable, notReadyBody))
	var out bytes.Buffer
	err := ReadyGet(gw.options(), &out, true)
	requireReason(t, err, api.ReasonResultNotOK)
	if strings.TrimSpace(out.String()) != notReadyBody {
		t.Fatalf("JSON mode rewrote the body:\n%s", out.String())
	}
}

// A 503 whose body is NOT the readiness contract (the boot-freeze
// middleware's error shape, for instance) is a status failure, never a
// fabricated verdict.
func TestReadyGetNonContract503IsStatusError(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusServiceUnavailable,
		`{"code":503,"message":"Maintenance mode","result":"booting"}`))
	var out bytes.Buffer
	err := ReadyGet(gw.options(), &out, false)
	requireReason(t, err, api.ReasonMaintenance)
}

func TestDiagnosticsGetRendersAssembly(t *testing.T) {
	body := `{"version":"v1.2.3","build_info":"rev abc","product":"loxilb-inference-gateway",` +
		`"api_version":"/netlox/v1 0.0.1","uptime_seconds":42,"ready":true,"ready_reasons":[],` +
		`"maintenance_state":"active",` +
		`"ebpf_attachments":[{"name":"eno1","mode":"tc","attached":true}],` +
		`"maps":[{"name":"conntrack","count":10,"capacity":1000}],` +
		`"external_dependencies":[{"type":"keystore","required":true,"status":"ready","latency_class":"fast"}]}`
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, body))
	var out bytes.Buffer
	if err := DiagnosticsGet(gw.options(), &out, false); err != nil {
		t.Fatalf("get diagnostics failed: %v", err)
	}
	text := out.String()
	for _, want := range []string{
		"Gateway: v1.2.3 (rev abc)",
		"API contract: /netlox/v1 0.0.1",
		"Uptime: 42s",
		"Maintenance: active",
		"eno1 (tc): attached",
		"Map conntrack: 10 of 1000 entries",
		"Dependency keystore: ready (fast)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("human output missing %q:\n%s", want, text)
		}
	}
}

func TestDiagnosticsGetUndecodableIsDecodeFailed(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, `not json`))
	var out bytes.Buffer
	err := DiagnosticsGet(gw.options(), &out, false)
	requireReason(t, err, api.ReasonDecodeFailed)
}

func TestDiagnosticsGetStatusErrorKeepsCode(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusUnauthorized,
		`{"code":401,"message":"Invalid authentication credentials","result":"x"}`))
	var out bytes.Buffer
	err := DiagnosticsGet(gw.options(), &out, false)
	requireReason(t, err, api.ReasonUnauthorized)
}
