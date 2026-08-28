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
package create

import (
	"encoding/json"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// serviceMap builds a service the way the command does (base + AI mapping),
// marshals it, and returns the decoded JSON object for key-by-key assertions.
func serviceMap(t *testing.T, o *CreateLoadBalancerOptions) map[string]any {
	t.Helper()
	s := api.LoadBalancerService{
		ExternalIP: o.ExternalIP,
		Protocol:   "tcp",
		Sel:        api.EpSelect(SelectToNum(o.Select)),
		Mode:       api.LbMode(ModeToNum(o.Mode)),
		Security:   api.LbSec(SecStringToNum(o.Security)),
	}
	applyAIServiceOptions(&s, o)
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func assertKey(t *testing.T, m map[string]any, key string, want any) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Fatalf("key %q absent, want %v", key, want)
	}
	// JSON numbers decode to float64; normalize.
	if wf, ok := want.(int); ok {
		if gf, ok := got.(float64); !ok || int(gf) != wf {
			t.Fatalf("key %q = %v, want %d", key, got, wf)
		}
		return
	}
	if got != want {
		t.Fatalf("key %q = %v (%T), want %v (%T)", key, got, got, want, want)
	}
}

func assertAbsent(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; ok {
			t.Fatalf("key %q present, want absent (omitempty)", k)
		}
	}
}

// A plain classic LB must not emit any AI keys (omitempty contract): the server
// then applies its own defaults and behavior is unchanged from base loxilb.
func TestClassicLB_NoAIKeys(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.1", Select: "rr"})
	assertAbsent(t, m,
		"model_name", "sse_mode", "api_key_auth", "kvExactMode", "kvBlockSize", "kvHashAlgo",
		"pd_disagg_mode", "chwbl_prefix_hash_level", "path_match_mode",
		"mtls_frontend", "mtls_backend", "hsts_max_age", "trace_type",
		"max_stream_duration_sec", "kvEngineType")
}

func TestAPIKeyAuthServiceArguments(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.16", Mode: "fullproxy", APIKeyAuth: "required",
	})
	assertKey(t, m, "api_key_auth", "required")
}

// vllm-kvcache-routing-cpu: kvExactMode=1 P/D with camelCase kv* keys.
func TestKVCacheServiceArguments(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.10", Mode: "fullproxy", Select: "rr",
		PdDisaggMode: true, KvExactMode: 1, KvZmqPort: 5557,
		KvHashAlgo: "sha256_cbor", KvWarmupSec: 20, KvBlockSize: 16,
	})
	assertKey(t, m, "mode", 4)
	assertKey(t, m, "pd_disagg_mode", true)
	assertKey(t, m, "kvExactMode", 1)
	assertKey(t, m, "kvZmqPort", 5557)
	assertKey(t, m, "kvHashAlgo", "sha256_cbor")
	assertKey(t, m, "kvWarmupSec", 20)
	assertKey(t, m, "kvBlockSize", 16)
	// engine type not set -> omitted so the server default (vllm) applies.
	assertAbsent(t, m, "kvEngineType", "kvDpRankCount")
}

// sglang-loxilb-kvcache: kvExactMode=3 with engine=sglang, kvHashAlgo omitted.
func TestSGLangServiceArguments(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.11", Mode: "fullproxy", Select: "rr",
		KvExactMode: 3, KvEngineType: "sglang", KvDpRankCount: 3,
		KvZmqPort: 5561, KvWarmupSec: 20, KvBlockSize: 16,
	})
	assertKey(t, m, "kvExactMode", 3)
	assertKey(t, m, "kvEngineType", "sglang")
	assertKey(t, m, "kvDpRankCount", 3)
	assertKey(t, m, "kvZmqPort", 5561)
	assertAbsent(t, m, "kvHashAlgo") // omitted -> engine default (sha256_sglang)
}

// ai-sse-quota: sse fields alongside model routing.
func TestSSEServiceArguments(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.12", Mode: "fullproxy", Select: "rr",
		ModelName: "llama-70b", SseMode: true,
		MaxStreamDurationSec: 120, BackendKeepaliveSec: 30,
	})
	assertKey(t, m, "model_name", "llama-70b")
	assertKey(t, m, "sse_mode", true)
	assertKey(t, m, "max_stream_duration_sec", 120)
	assertKey(t, m, "backend_keepalive_interval_sec", 30)
}

// ai-model-routing: path-based routing keys.
func TestModelRoutingServiceArguments(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.13", Mode: "fullproxy", Select: "rr",
		ModelName: "mistral-7b", PathPrefix: "/", PathMatchMode: "prefix",
	})
	assertKey(t, m, "model_name", "mistral-7b")
	assertKey(t, m, "path_prefix", "/")
	assertKey(t, m, "path_match_mode", "prefix")
}

// vllm-fullproxy: CHWBL (sel=8) with bounded-load knobs.
func TestCHWBLServiceArguments(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.14", Mode: "fullproxy", Select: "chwbl",
		ChwblPrefixHashLevel: 2, ChwblMeanLoadFactor: 250, ChwblReplication: 200,
	})
	assertKey(t, m, "sel", 8)
	assertKey(t, m, "chwbl_prefix_hash_level", 2)
	assertKey(t, m, "chwbl_mean_load_factor", 250)
	assertKey(t, m, "chwbl_replication", 200)
}

// e2ehttpsproxy-mtls: nested mtls_frontend / mtls_backend objects.
func TestMTLSServiceArguments(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.15", Mode: "fullproxy", Security: "e2ehttps",
		MtlsClientCertMode: "required", MtlsClientCAPath: "/opt/ca.pem",
		MtlsRequireClientCN: true, MtlsClientCNPattern: "*.corp.example.com",
		MtlsBackendVerifyServer: true, MtlsBackendCAPath: "/opt/backend-ca.pem",
	})
	fe, ok := m["mtls_frontend"].(map[string]any)
	if !ok {
		t.Fatalf("mtls_frontend missing or wrong type: %v", m["mtls_frontend"])
	}
	assertKey(t, fe, "client_cert_mode", "required")
	assertKey(t, fe, "client_ca_path", "/opt/ca.pem")
	assertKey(t, fe, "require_client_cn", true)
	assertKey(t, fe, "client_cn_pattern", "*.corp.example.com")
	be, ok := m["mtls_backend"].(map[string]any)
	if !ok {
		t.Fatalf("mtls_backend missing or wrong type: %v", m["mtls_backend"])
	}
	assertKey(t, be, "verify_server_cert", true)
	assertKey(t, be, "backend_ca_path", "/opt/backend-ca.pem")
}

func TestSelectToNum_AI(t *testing.T) {
	cases := map[string]int{"rr": 0, "persist": 3, "chwbl": 8, "gpuaware": 9, "wrr-hash": 10, "chwbl-wrr": 10}
	for in, want := range cases {
		if got := SelectToNum(in); got != want {
			t.Fatalf("SelectToNum(%q) = %d, want %d", in, got, want)
		}
	}
}

// vllm-pd-disagg: ordered endpoints with per-endpoint roles and NIXL ports.
func TestEndpointRolesAligned(t *testing.T) {
	eps := []string{"31.31.31.1:1", "32.32.32.1:1", "33.33.33.1:1"}
	list, err := GetEndpointList(eps)
	if err != nil {
		t.Fatalf("GetEndpointList: %v", err)
	}
	if len(list) != 3 || list[0].IP != "31.31.31.1" || list[2].IP != "33.33.33.1" {
		t.Fatalf("endpoint order not preserved: %+v", list)
	}
	roles, err := parseEndpointRoles([]string{"prefill", "decode", "prefill"}, len(list))
	if err != nil {
		t.Fatalf("parseEndpointRoles: %v", err)
	}
	if roles[0] != 1 || roles[1] != 2 || roles[2] != 1 {
		t.Fatalf("roles = %v, want [1 2 1]", roles)
	}
	nixl, err := alignNixlPorts([]int{9001, 9002, 9003}, len(list))
	if err != nil {
		t.Fatalf("alignNixlPorts: %v", err)
	}
	if nixl[0] != 9001 || nixl[2] != 9003 {
		t.Fatalf("nixl = %v", nixl)
	}
}

func TestEndpointRolesLengthMismatch(t *testing.T) {
	if _, err := parseEndpointRoles([]string{"prefill"}, 3); err == nil {
		t.Fatal("expected length-mismatch error for --ep-role")
	}
	if _, err := alignNixlPorts([]int{9001}, 3); err == nil {
		t.Fatal("expected length-mismatch error for --nixl-port")
	}
}

// IPv6 endpoints must still parse (weight split off the last colon).
func TestEndpointIPv6(t *testing.T) {
	list, err := GetEndpointList([]string{"4ffe::1:2"})
	if err != nil {
		t.Fatalf("GetEndpointList ipv6: %v", err)
	}
	if list[0].IP != "4ffe::1" || list[0].Weight != 2 {
		t.Fatalf("ipv6 parse = %+v, want ip=4ffe::1 weight=2", list[0])
	}
}

// TestFullPDDisaggBody assembles a complete LoadBalancerModel matching the
// verbatim vllm-pd-disagg cache-aware cicd body (2 prefill + 2 decode with NIXL
// ports) and checks the serialized endpoints carry ep_role/nixl_port in order.
func TestFullPDDisaggBody(t *testing.T) {
	o := &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.20", Mode: "fullproxy", Select: "rr", Security: "https",
		PdDisaggMode: true, PdCacheAwareMode: true, SseMode: true, Host: "192.0.2.20",
		Endpoints: []string{"203.0.113.1:1", "203.0.113.3:1", "203.0.113.2:1", "203.0.113.4:1"},
		EpRoles:   []string{"prefill", "prefill", "decode", "decode"},
		NixlPorts: []int{9001, 9003, 9002, 9004},
	}
	if err := validateLBAIOptions(o); err != nil {
		t.Fatalf("validate: %v", err)
	}

	svc := api.LoadBalancerService{
		ExternalIP: o.ExternalIP, Protocol: "tcp", Port: 2023,
		Sel: api.EpSelect(SelectToNum(o.Select)), Mode: api.LbMode(ModeToNum(o.Mode)),
		Security: api.LbSec(SecStringToNum(o.Security)), Host: o.Host,
	}
	applyAIServiceOptions(&svc, o)

	list, _ := GetEndpointList(o.Endpoints)
	roles, _ := parseEndpointRoles(o.EpRoles, len(list))
	nixl, _ := alignNixlPorts(o.NixlPorts, len(list))
	model := api.LoadBalancerModel{Service: svc}
	for i, e := range list {
		model.Endpoints = append(model.Endpoints, api.LoadBalancerEndpoint{
			EndpointIP: e.IP, TargetPort: 8000, Weight: e.Weight, EpRole: roles[i], NixlPort: nixl[i],
		})
	}

	b, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Service   map[string]any   `json:"serviceArguments"`
		Endpoints []map[string]any `json:"endpoints"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	assertKey(t, decoded.Service, "pd_disagg_mode", true)
	assertKey(t, decoded.Service, "pd_cache_aware_mode", true)
	assertKey(t, decoded.Service, "sse_mode", true)
	assertKey(t, decoded.Service, "host", "192.0.2.20")

	if len(decoded.Endpoints) != 4 {
		t.Fatalf("endpoints len = %d, want 4", len(decoded.Endpoints))
	}
	// Order preserved; roles/ports aligned exactly as in the cicd body.
	assertKey(t, decoded.Endpoints[0], "ep_role", 1)
	assertKey(t, decoded.Endpoints[0], "nixl_port", 9001)
	assertKey(t, decoded.Endpoints[2], "ep_role", 2)
	assertKey(t, decoded.Endpoints[2], "nixl_port", 9002)
	assertKey(t, decoded.Endpoints[3], "ep_role", 2)
	assertKey(t, decoded.Endpoints[3], "nixl_port", 9004)
}

func TestValidateAIRequiresFullproxy(t *testing.T) {
	// AI field without fullproxy -> error.
	if err := validateLBAIOptions(&CreateLoadBalancerOptions{ModelName: "x", Mode: "onearm"}); err == nil {
		t.Fatal("expected error: AI without fullproxy")
	}
	// AI field with fullproxy -> ok.
	if err := validateLBAIOptions(&CreateLoadBalancerOptions{ModelName: "x", Mode: "fullproxy"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// pd-cache-aware requires pd-disagg.
	if err := validateLBAIOptions(&CreateLoadBalancerOptions{PdCacheAwareMode: true, Mode: "fullproxy"}); err == nil {
		t.Fatal("expected error: pd-cache-aware without pd-disagg")
	}
	// classic LB (no AI) with non-fullproxy mode -> ok.
	if err := validateLBAIOptions(&CreateLoadBalancerOptions{Select: "rr", Mode: "onearm"}); err != nil {
		t.Fatalf("classic LB should not be gated: %v", err)
	}
}

func TestValidateAPIKeyAuth(t *testing.T) {
	for _, policy := range []string{"disabled", "required"} {
		if err := validateLBAIOptions(&CreateLoadBalancerOptions{APIKeyAuth: policy, Mode: "fullproxy"}); err != nil {
			t.Fatalf("policy %q rejected: %v", policy, err)
		}
	}
	if err := validateLBAIOptions(&CreateLoadBalancerOptions{APIKeyAuth: "optional", Mode: "fullproxy"}); err == nil {
		t.Fatal("expected closed-enum rejection")
	}
	if err := validateLBAIOptions(&CreateLoadBalancerOptions{APIKeyAuth: "required", Mode: "onearm"}); err == nil {
		t.Fatal("expected fullproxy requirement")
	}
}
