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
		"max_stream_duration_sec", "cb_enable", "pdBootstrapPort", "kvEngineType", "sockMapMode",
		"connectionLimit", "fc_max_queue_depth", "fc_max_queue_wait_ms", "fc_effective")
}

// --connection-limit is an L4 attribute, sent only when set: a classic rule
// keeps its unlimited default absent, a declared ceiling reaches the wire as
// connectionLimit.
func TestConnectionLimitServiceArgument(t *testing.T) {
	t.Run("declared", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.30", Select: "rr", ConnectionLimit: 100})
		assertKey(t, m, "connectionLimit", 100)
	})
	t.Run("unset omitted", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.31", Select: "rr"})
		assertAbsent(t, m, "connectionLimit")
	})
	t.Run("flag is registered as uint32", func(t *testing.T) {
		f := NewCreateLoadBalancerCmd(&api.RESTOptions{}).Flags().Lookup("connection-limit")
		if f == nil {
			t.Fatal("--connection-limit is not registered")
		}
		if f.Value.Type() != "uint32" {
			t.Fatalf("--connection-limit type %s, want uint32", f.Value.Type())
		}
	})
}

// --fc-max-queue-depth / --fc-max-queue-wait-ms are sent only when set: an
// unset pair stays absent, a declared pair reaches the wire as
// fc_max_queue_depth / fc_max_queue_wait_ms, and the gateway's bounds are
// enforced before the request is built.
func TestFcQueueServiceArguments(t *testing.T) {
	t.Run("declared", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.32", Select: "rr", FcMaxQueueDepth: 64, FcMaxQueueWaitMs: 30000})
		assertKey(t, m, "fc_max_queue_depth", 64)
		assertKey(t, m, "fc_max_queue_wait_ms", 30000)
	})
	t.Run("unset omitted", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.33", Select: "rr"})
		assertAbsent(t, m, "fc_max_queue_depth", "fc_max_queue_wait_ms", "fc_effective")
	})
	t.Run("flags are registered as uint32", func(t *testing.T) {
		flags := NewCreateLoadBalancerCmd(&api.RESTOptions{}).Flags()
		for _, name := range []string{"fc-max-queue-depth", "fc-max-queue-wait-ms"} {
			f := flags.Lookup(name)
			if f == nil {
				t.Fatalf("--%s is not registered", name)
			}
			if f.Value.Type() != "uint32" {
				t.Fatalf("--%s type %s, want uint32", name, f.Value.Type())
			}
		}
	})
	t.Run("pair accepted", func(t *testing.T) {
		if err := validateLBAIOptions(&CreateLoadBalancerOptions{FcMaxQueueDepth: 65536, FcMaxQueueWaitMs: 3600000}); err != nil {
			t.Fatalf("ceiling pair rejected: %v", err)
		}
	})
	// create lb is create-or-replace, and a replace keeps each field it
	// omits: a flag given as 0 must reach the gateway (it restores the
	// process default), an omitted one must not (it keeps the rule's value).
	t.Run("explicit zero goes on the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.36", Select: "rr",
			FcMaxQueueDepthSet: true, FcMaxQueueWaitMsSet: true})
		assertKey(t, m, "fc_max_queue_depth", 0)
		assertKey(t, m, "fc_max_queue_wait_ms", 0)
	})
	t.Run("depth alone is the gateway's to judge", func(t *testing.T) {
		// On a replace the rule's current wait completes the pair.
		o := &CreateLoadBalancerOptions{ExternalIP: "192.0.2.37", Select: "rr", FcMaxQueueDepth: 64, FcMaxQueueDepthSet: true}
		if err := validateLBAIOptions(o); err != nil {
			t.Fatalf("depth alone rejected client-side: %v", err)
		}
		m := serviceMap(t, o)
		assertKey(t, m, "fc_max_queue_depth", 64)
		assertAbsent(t, m, "fc_max_queue_wait_ms")
	})
	t.Run("depth with an explicit zero wait", func(t *testing.T) {
		err := validateLBAIOptions(&CreateLoadBalancerOptions{FcMaxQueueDepth: 64, FcMaxQueueDepthSet: true, FcMaxQueueWaitMsSet: true})
		if err == nil {
			t.Fatal("expected error for --fc-max-queue-depth with --fc-max-queue-wait-ms 0")
		}
		if want := "--fc-max-queue-wait-ms must be non-zero when --fc-max-queue-depth is set"; err.Error() != want {
			t.Fatalf("error %q, want %q", err.Error(), want)
		}
	})
	t.Run("depth above ceiling", func(t *testing.T) {
		if err := validateLBAIOptions(&CreateLoadBalancerOptions{FcMaxQueueDepth: 65537, FcMaxQueueWaitMs: 1000}); err == nil {
			t.Fatal("expected error for --fc-max-queue-depth above 65536")
		}
	})
	t.Run("wait above ceiling", func(t *testing.T) {
		if err := validateLBAIOptions(&CreateLoadBalancerOptions{FcMaxQueueDepth: 64, FcMaxQueueWaitMs: 3600001}); err == nil {
			t.Fatal("expected error for --fc-max-queue-wait-ms above 3600000")
		}
	})
}

// A GET payload carrying fc_effective decodes into the read-only struct.
func TestFcEffectiveReadback(t *testing.T) {
	payload := `{"externalIP":"192.0.2.34","port":2020,"protocol":"tcp","sel":0,"mode":4,"BGP":false,"Monitor":false,` +
		`"inactiveTimeOut":240,"block":0,"proxyprotocolv2":false,"egress":false,` +
		`"fc_max_queue_depth":64,"fc_max_queue_wait_ms":30000,` +
		`"fc_effective":{"mode":"enforce","max_outstanding":128,"ep_max_inflight":32,"prefill_max_inflight":8,` +
		`"decode_max_inflight":24,"queue_depth":64,"queue_wait_ms":30000,"inflight":5,"queued":2,"queue_memory_bound_mib":64}}`
	var s api.LoadBalancerService
	if err := json.Unmarshal([]byte(payload), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.FcMaxQueueDepth == nil || *s.FcMaxQueueDepth != 64 || s.FcMaxQueueWaitMs == nil || *s.FcMaxQueueWaitMs != 30000 {
		t.Fatalf("queue fields = %v/%v, want 64/30000", s.FcMaxQueueDepth, s.FcMaxQueueWaitMs)
	}
	fe := s.FcEffective
	if fe == nil {
		t.Fatal("fc_effective did not decode")
	}
	want := api.FcEffective{Mode: "enforce", MaxOutstanding: 128, EpMaxInflight: 32, PrefillMaxInflight: 8,
		DecodeMaxInflight: 24, QueueDepth: 64, QueueWaitMs: 30000, Inflight: 5, Queued: 2, QueueMemoryBoundMib: 64}
	if *fe != want {
		t.Fatalf("fc_effective = %+v, want %+v", *fe, want)
	}
	// Absent on the wire stays nil, so a create body never echoes it back.
	var bare api.LoadBalancerService
	if err := json.Unmarshal([]byte(`{"externalIP":"192.0.2.35","port":2020,"protocol":"tcp","sel":0,"mode":0,"BGP":false,"Monitor":false,"inactiveTimeOut":0,"block":0,"proxyprotocolv2":false,"egress":false}`), &bare); err != nil {
		t.Fatalf("unmarshal bare: %v", err)
	}
	if bare.FcEffective != nil {
		t.Fatalf("fc_effective = %+v, want nil", *bare.FcEffective)
	}
}

func TestSockMapModeServiceArguments(t *testing.T) {
	t.Run("both", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{
			ExternalIP: "192.0.2.21", Mode: "fullproxy", SockMapMode: "both",
		})
		assertKey(t, m, "sockMapMode", "both")
	})
	t.Run("off", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{
			ExternalIP: "192.0.2.22", Mode: "fullproxy", SockMapMode: "off",
		})
		assertKey(t, m, "sockMapMode", "off")
	})
	t.Run("unset omitted", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{
			ExternalIP: "192.0.2.23", Mode: "fullproxy",
		})
		assertAbsent(t, m, "sockMapMode")
	})
}

func TestResilienceAndBootstrapServiceArguments(t *testing.T) {
	m := serviceMap(t, &CreateLoadBalancerOptions{
		ExternalIP: "192.0.2.17", Mode: "fullproxy", CbEnable: true,
		PdDisaggMode: true, KvEngineType: "sglang", PdBootstrapPort: 8998,
	})
	assertKey(t, m, "cb_enable", true)
	assertKey(t, m, "pdBootstrapPort", 8998)
}

func TestAdditionalEngineServiceArguments(t *testing.T) {
	t.Run("trtllm", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{
			ExternalIP: "192.0.2.18", Mode: "fullproxy", KvEngineType: "trtllm",
			KvExactMode: 3, KvBlockSize: 32,
		})
		assertKey(t, m, "kvEngineType", "trtllm")
		assertKey(t, m, "kvExactMode", 3)
		assertKey(t, m, "kvBlockSize", 32)
		assertAbsent(t, m, "kvHashAlgo", "kvZmqPort", "kvDpRankCount")
	})
	t.Run("llamacpp", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{
			ExternalIP: "192.0.2.19", Mode: "fullproxy", Select: "chwbl", KvEngineType: "llamacpp",
		})
		assertKey(t, m, "kvEngineType", "llamacpp")
		assertKey(t, m, "sel", 8)
		assertAbsent(t, m, "kvExactMode", "kvHashAlgo", "pd_disagg_mode")
	})
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

func TestValidateSockMapMode(t *testing.T) {
	for _, mode := range []string{"off", "request", "response", "both"} {
		if err := validateLBAIOptions(&CreateLoadBalancerOptions{SockMapMode: mode, Mode: "fullproxy"}); err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
	}
	if err := validateLBAIOptions(&CreateLoadBalancerOptions{SockMapMode: "invalid", Mode: "fullproxy"}); err == nil {
		t.Fatal("expected enum rejection")
	}
	if err := validateLBAIOptions(&CreateLoadBalancerOptions{SockMapMode: "both", Mode: "onearm"}); err == nil {
		t.Fatal("expected fullproxy requirement")
	}
}

func TestValidateKVEngineOptions(t *testing.T) {
	positive := []CreateLoadBalancerOptions{
		{Mode: "fullproxy", KvEngineType: "vllm", PdDisaggMode: true, KvExactMode: 1, EpRoles: []string{"prefill", "decode"}},
		{Mode: "fullproxy", KvEngineType: "sglang", PdDisaggMode: true, KvExactMode: 1, KvDpRankCount: 8, KvZmqPort: 65528, PdBootstrapPort: 8998, EpRoles: []string{"prefill", "decode"}},
		{Mode: "fullproxy", KvEngineType: "trtllm", KvExactMode: 3, KvBlockSize: 32},
		{Mode: "fullproxy", Select: "chwbl", KvEngineType: "llamacpp"},
	}
	for i := range positive {
		if err := validateLBAIOptions(&positive[i]); err != nil {
			t.Errorf("positive case %d rejected: %v", i, err)
		}
	}

	negative := []CreateLoadBalancerOptions{
		{Mode: "fullproxy", KvEngineType: "tensorrt"},
		{Mode: "fullproxy", KvEngineType: "vllm", KvHashAlgo: "sha256_sglang"},
		{Mode: "fullproxy", KvEngineType: "sglang", KvHashAlgo: "sha256_cbor"},
		{Mode: "fullproxy", KvEngineType: "trtllm", KvHashAlgo: "sha256_cbor"},
		{Mode: "fullproxy", KvEngineType: "llamacpp", KvHashAlgo: "sha256_cbor"},
		{Mode: "fullproxy", KvEngineType: "llamacpp", KvExactMode: 3},
		{Mode: "fullproxy", KvEngineType: "trtllm", KvZmqPort: 5561},
		{Mode: "fullproxy", KvEngineType: "sglang", KvDpRankCount: 9},
		{Mode: "fullproxy", KvEngineType: "sglang", KvExactMode: 3, KvDpRankCount: 8, KvZmqPort: 65529},
		{Mode: "fullproxy", KvEngineType: "sglang", PdBootstrapPort: 8998},
		{Mode: "fullproxy", KvEngineType: "vllm", PdDisaggMode: true, EpRoles: []string{"prefill"}},
		{Mode: "fullproxy", KvEngineType: "vllm", KvExactMode: 2},
	}
	for i := range negative {
		if err := validateLBAIOptions(&negative[i]); err == nil {
			t.Errorf("negative case %d accepted: %+v", i, negative[i])
		}
	}
}

// The rest of the admission gate follows the queue's presence rules: a flag
// given goes on the wire even at 0, an omitted one stays off; --fc-mode is
// sent when given, "inherit" included.
func TestFcGateServiceArguments(t *testing.T) {
	t.Run("declared", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.40", Select: "rr",
			FcMode: "enforce", FcMaxOutstanding: 8, FcMaxOutstandingSet: true, FcEpMaxInflight: 4, FcEpMaxInflightSet: true,
			FcPrefillMaxInflight: 2, FcPrefillMaxInflightSet: true, FcDecodeMaxInflight: 6, FcDecodeMaxInflightSet: true,
			FcTelemetryStaleMs: 60000, FcTelemetryStaleMsSet: true})
		assertKey(t, m, "fc_mode", "enforce")
		assertKey(t, m, "fc_max_outstanding", 8)
		assertKey(t, m, "fc_ep_max_inflight", 4)
		assertKey(t, m, "fc_prefill_max_inflight", 2)
		assertKey(t, m, "fc_decode_max_inflight", 6)
		assertKey(t, m, "fc_telemetry_stale_ms", 60000)
	})
	t.Run("omitted stays off the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.41", Select: "rr"})
		assertAbsent(t, m, "fc_mode", "fc_max_outstanding", "fc_ep_max_inflight",
			"fc_prefill_max_inflight", "fc_decode_max_inflight", "fc_telemetry_stale_ms")
	})
	t.Run("explicit zero and inherit go on the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.42", Select: "rr",
			FcMode: "inherit", FcMaxOutstandingSet: true, FcTelemetryStaleMsSet: true})
		assertKey(t, m, "fc_mode", "inherit")
		assertKey(t, m, "fc_max_outstanding", 0)
		assertKey(t, m, "fc_telemetry_stale_ms", 0)
		assertAbsent(t, m, "fc_ep_max_inflight")
	})
	for _, c := range []struct {
		name string
		o    CreateLoadBalancerOptions
		want string
	}{
		{"an unknown mode", CreateLoadBalancerOptions{FcMode: "yes"}, "--fc-mode must be one of off|observe|enforce|inherit"},
		{"a ceiling above 100000", CreateLoadBalancerOptions{FcMaxOutstanding: 100001}, "--fc-max-outstanding must be within 0..100000"},
		{"a decode ceiling above 100000", CreateLoadBalancerOptions{FcDecodeMaxInflight: 100001}, "--fc-decode-max-inflight must be within 0..100000"},
		{"a window above an hour", CreateLoadBalancerOptions{FcTelemetryStaleMs: 3600001}, "--fc-telemetry-stale-ms must be within 0..3600000"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := validateLBAIOptions(&c.o)
			if err == nil || err.Error() != c.want {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
	t.Run("the flags are registered", func(t *testing.T) {
		flags := NewCreateLoadBalancerCmd(&api.RESTOptions{}).Flags()
		for name, typ := range map[string]string{"fc-mode": "string", "fc-max-outstanding": "uint32",
			"fc-ep-max-inflight": "uint32", "fc-prefill-max-inflight": "uint32",
			"fc-decode-max-inflight": "uint32", "fc-telemetry-stale-ms": "uint32"} {
			f := flags.Lookup(name)
			if f == nil || f.Value.Type() != typ {
				t.Fatalf("--%s not registered as %s", name, typ)
			}
		}
	})
}

// The adaptive ceiling's three fields follow the same presence rules.
func TestFcAdaptiveServiceArguments(t *testing.T) {
	t.Run("declared", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.43", Select: "rr",
			FcAdaptive: "on", FcWarmupMs: 20000, FcWarmupMsSet: true, FcTtftTargetMs: 300, FcTtftTargetMsSet: true})
		assertKey(t, m, "fc_adaptive", "on")
		assertKey(t, m, "fc_warmup_ms", 20000)
		assertKey(t, m, "fc_ttft_target_ms", 300)
	})
	t.Run("omitted stays off the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.44", Select: "rr"})
		assertAbsent(t, m, "fc_adaptive", "fc_warmup_ms", "fc_ttft_target_ms")
	})
	t.Run("explicit zero and inherit go on the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.45", Select: "rr",
			FcAdaptive: "inherit", FcWarmupMsSet: true, FcTtftTargetMsSet: true})
		assertKey(t, m, "fc_adaptive", "inherit")
		assertKey(t, m, "fc_warmup_ms", 0)
		assertKey(t, m, "fc_ttft_target_ms", 0)
	})
	for _, c := range []struct {
		name string
		o    CreateLoadBalancerOptions
		want string
	}{
		{"an unknown switch", CreateLoadBalancerOptions{FcAdaptive: "yes"}, "--fc-adaptive must be one of on|off|inherit"},
		{"a warm-up above an hour", CreateLoadBalancerOptions{FcWarmupMs: 3600001}, "--fc-warmup-ms must be within 0..3600000"},
		{"a target above an hour", CreateLoadBalancerOptions{FcTtftTargetMs: 3600001}, "--fc-ttft-target-ms must be within 0..3600000"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := validateLBAIOptions(&c.o)
			if err == nil || err.Error() != c.want {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
	t.Run("the flags are registered", func(t *testing.T) {
		flags := NewCreateLoadBalancerCmd(&api.RESTOptions{}).Flags()
		for name, typ := range map[string]string{"fc-adaptive": "string", "fc-warmup-ms": "uint32", "fc-ttft-target-ms": "uint32"} {
			f := flags.Lookup(name)
			if f == nil || f.Value.Type() != typ {
				t.Fatalf("--%s not registered as %s", name, typ)
			}
		}
	})
}

func TestFcAdaptiveReadback(t *testing.T) {
	payload := `{"externalIP":"192.0.2.46","port":2030,"protocol":"tcp","sel":0,"mode":4,"BGP":false,"Monitor":false,` +
		`"inactiveTimeOut":240,"block":0,"proxyprotocolv2":false,"egress":false,` +
		`"fc_adaptive":"on","fc_warmup_ms":20000,"fc_ttft_target_ms":300,` +
		`"fc_effective":{"mode":"enforce","max_outstanding":10,"adaptive":"on","warmup_ms":20000,"ttft_target_ms":300,` +
		`"effective_max_outstanding":2,"adapt_state":"tightened","adapt_reason":"queued","warming_endpoints":1,` +
		`"source":{"adaptive":"rule","warmup_ms":"rule","ttft_target_ms":"env"}}}`
	var s api.LoadBalancerService
	if err := json.Unmarshal([]byte(payload), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.FcAdaptive != "on" || s.FcWarmupMs == nil || *s.FcWarmupMs != 20000 || s.FcTtftTargetMs == nil || *s.FcTtftTargetMs != 300 {
		t.Fatalf("rule fields = %q/%v/%v, want on/20000/300", s.FcAdaptive, s.FcWarmupMs, s.FcTtftTargetMs)
	}
	fe := s.FcEffective
	if fe == nil || fe.Source == nil {
		t.Fatal("fc_effective or its source did not decode")
	}
	if fe.Adaptive != "on" || fe.WarmupMs != 20000 || fe.TtftTargetMs != 300 || fe.EffectiveMaxOutstanding != 2 ||
		fe.AdaptState != "tightened" || fe.AdaptReason != "queued" || fe.WarmingEndpoints != 1 {
		t.Fatalf("fc_effective = %+v", *fe)
	}
	if fe.Source.Adaptive != "rule" || fe.Source.WarmupMs != "rule" || fe.Source.TtftTargetMs != "env" {
		t.Fatalf("source = %+v", *fe.Source)
	}
}

// The tenant share follows the same presence rules, bounded 0..100.
func TestFcTenantShareServiceArguments(t *testing.T) {
	t.Run("declared", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.47", Select: "rr",
			FcTenantMaxSharePct: 25, FcTenantMaxSharePctSet: true})
		assertKey(t, m, "fc_tenant_max_share_pct", 25)
	})
	t.Run("omitted stays off the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.48", Select: "rr"})
		assertAbsent(t, m, "fc_tenant_max_share_pct")
	})
	t.Run("explicit zero goes on the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.49", Select: "rr", FcTenantMaxSharePctSet: true})
		assertKey(t, m, "fc_tenant_max_share_pct", 0)
	})
	t.Run("above a hundred", func(t *testing.T) {
		err := validateLBAIOptions(&CreateLoadBalancerOptions{FcTenantMaxSharePct: 101})
		if want := "--fc-tenant-max-share-pct must be within 0..100"; err == nil || err.Error() != want {
			t.Fatalf("error %v, want %q", err, want)
		}
	})
	t.Run("a hundred is accepted", func(t *testing.T) {
		if err := validateLBAIOptions(&CreateLoadBalancerOptions{FcTenantMaxSharePct: 100}); err != nil {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("the flag is registered", func(t *testing.T) {
		f := NewCreateLoadBalancerCmd(&api.RESTOptions{}).Flags().Lookup("fc-tenant-max-share-pct")
		if f == nil || f.Value.Type() != "uint32" {
			t.Fatal("--fc-tenant-max-share-pct not registered as uint32")
		}
	})
}

func TestFcTenantShareReadback(t *testing.T) {
	payload := `{"externalIP":"192.0.2.50","port":2033,"protocol":"tcp","sel":0,"mode":4,"BGP":false,"Monitor":false,` +
		`"inactiveTimeOut":240,"block":0,"proxyprotocolv2":false,"egress":false,"fc_tenant_max_share_pct":50,` +
		`"fc_effective":{"mode":"enforce","max_outstanding":4,"tenant_max_share_pct":50,"tenants_active":2,` +
		`"source":{"tenant_max_share_pct":"rule"}}}`
	var s api.LoadBalancerService
	if err := json.Unmarshal([]byte(payload), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.FcTenantMaxSharePct == nil || *s.FcTenantMaxSharePct != 50 {
		t.Fatalf("rule field = %v, want 50", s.FcTenantMaxSharePct)
	}
	fe := s.FcEffective
	if fe == nil || fe.Source == nil || fe.TenantMaxSharePct != 50 || fe.TenantsActive != 2 || fe.Source.TenantMaxSharePct != "rule" {
		t.Fatalf("fc_effective = %+v", fe)
	}
}
