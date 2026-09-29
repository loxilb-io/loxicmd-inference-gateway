/*
 * Copyright (c) 2022 NetLOX Inc
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
package api

import (
	"fmt"
	"sort"
)

type LoadBalancer struct {
	CommonAPI
}

type EpSelect uint
type LbMode int32
type LbOP int32
type LbSec int32

type LbRuleModGet struct {
	LbRules []LoadBalancerModel `json:"lbAttr"`
}

type LoadBalancerModel struct {
	Service      LoadBalancerService    `json:"serviceArguments" yaml:"serviceArguments"`
	SecondaryIPs []LoadBalancerSecIp    `json:"secondaryIPs" yaml:"secondaryIPs"`
	SrcIPs       []LbAllowedSrcIPArg    `json:"allowedSources" yaml:"allowedSources"`
	Endpoints    []LoadBalancerEndpoint `json:"endpoints" yaml:"endpoints"`
}

// FcEffective is the flow-control state of a service's model pool as the
// gateway reports it on GET: the resolved limits plus the live counters.
type FcEffective struct {
	Mode                string `json:"mode,omitempty"                   yaml:"mode,omitempty"`
	MaxOutstanding      uint32 `json:"max_outstanding,omitempty"        yaml:"max_outstanding,omitempty"`
	EpMaxInflight       uint32 `json:"ep_max_inflight,omitempty"        yaml:"ep_max_inflight,omitempty"`
	PrefillMaxInflight  uint32 `json:"prefill_max_inflight,omitempty"   yaml:"prefill_max_inflight,omitempty"`
	DecodeMaxInflight   uint32 `json:"decode_max_inflight,omitempty"    yaml:"decode_max_inflight,omitempty"`
	QueueDepth          uint32 `json:"queue_depth,omitempty"            yaml:"queue_depth,omitempty"`
	QueueWaitMs         uint32 `json:"queue_wait_ms,omitempty"          yaml:"queue_wait_ms,omitempty"`
	Inflight            uint32 `json:"inflight,omitempty"               yaml:"inflight,omitempty"`
	Queued              uint32 `json:"queued,omitempty"                 yaml:"queued,omitempty"`
	QueueMemoryBoundMib uint64 `json:"queue_memory_bound_mib,omitempty" yaml:"queue_memory_bound_mib,omitempty"`
	TelemetryStaleMs    uint32 `json:"telemetry_stale_ms,omitempty"     yaml:"telemetry_stale_ms,omitempty"`
	// The adaptive ceiling: the switch and its inputs in force, the ceiling
	// it holds now, its state (off, open, tightened, frozen) and why, and the
	// endpoints still ramping up after a return to service.
	Adaptive                string `json:"adaptive,omitempty"                  yaml:"adaptive,omitempty"`
	WarmupMs                uint32 `json:"warmup_ms,omitempty"                 yaml:"warmup_ms,omitempty"`
	TtftTargetMs            uint32 `json:"ttft_target_ms,omitempty"            yaml:"ttft_target_ms,omitempty"`
	EffectiveMaxOutstanding uint32 `json:"effective_max_outstanding,omitempty" yaml:"effective_max_outstanding,omitempty"`
	AdaptState              string `json:"adapt_state,omitempty"               yaml:"adapt_state,omitempty"`
	AdaptReason             string `json:"adapt_reason,omitempty"              yaml:"adapt_reason,omitempty"`
	WarmingEndpoints        uint32 `json:"warming_endpoints,omitempty"         yaml:"warming_endpoints,omitempty"`
	// Source names where each value came from: rule, env or default.
	Source *FcEffectiveSource `json:"source,omitempty" yaml:"source,omitempty"`
}

// FcEffectiveSource is where each value of FcEffective came from.
type FcEffectiveSource struct {
	Mode               string `json:"mode,omitempty"                 yaml:"mode,omitempty"`
	MaxOutstanding     string `json:"max_outstanding,omitempty"      yaml:"max_outstanding,omitempty"`
	EpMaxInflight      string `json:"ep_max_inflight,omitempty"      yaml:"ep_max_inflight,omitempty"`
	PrefillMaxInflight string `json:"prefill_max_inflight,omitempty" yaml:"prefill_max_inflight,omitempty"`
	DecodeMaxInflight  string `json:"decode_max_inflight,omitempty"  yaml:"decode_max_inflight,omitempty"`
	QueueDepth         string `json:"queue_depth,omitempty"          yaml:"queue_depth,omitempty"`
	QueueWaitMs        string `json:"queue_wait_ms,omitempty"        yaml:"queue_wait_ms,omitempty"`
	TelemetryStaleMs   string `json:"telemetry_stale_ms,omitempty"   yaml:"telemetry_stale_ms,omitempty"`
	Adaptive           string `json:"adaptive,omitempty"             yaml:"adaptive,omitempty"`
	WarmupMs           string `json:"warmup_ms,omitempty"            yaml:"warmup_ms,omitempty"`
	TtftTargetMs       string `json:"ttft_target_ms,omitempty"       yaml:"ttft_target_ms,omitempty"`
}

type LoadBalancerService struct {
	ExternalIP string   `json:"externalIP"         yaml:"externalIP"`
	Port       uint16   `json:"port"               yaml:"port"`
	PortMax    uint16   `json:"portMax,omitempty"  yaml:"portmax"`
	Protocol   string   `json:"protocol"           yaml:"protocol"`
	Sel        EpSelect `json:"sel"                yaml:"sel"`
	Mode       LbMode   `json:"mode"               yaml:"mode"`
	BGP        bool     `json:"BGP"                yaml:"BGP"`
	Monitor    bool     `json:"Monitor"            yaml:"Monitor"`
	Timeout    uint32   `json:"inactiveTimeOut"    yaml:"inactiveTimeOut"`
	Block      uint32   `json:"block"              yaml:"block"`
	Managed    bool     `json:"managed,omitempty"  yaml:"managed"`
	Name       string   `json:"name,omitempty"     yaml:"name"`
	Snat       bool     `json:"snat,omitempty"`
	Oper       LbOP     `json:"oper,omitempty"`
	Security   LbSec    `json:"security,omitempty" yaml:"security"`
	Host       string   `json:"host,omitempty"     yaml:"path"`
	PpV2       bool     `json:"proxyprotocolv2"    yaml:"proxyprotocolv2"`
	Egress     bool     `json:"egress"             yaml:"egress"`

	// Concurrent-connection ceiling across the rule's endpoints; 0 = unlimited.
	ConnectionLimit uint32 `json:"connectionLimit,omitempty" yaml:"connectionLimit,omitempty"`

	// Capacity admission queue of the service's model pool; 0 restores the
	// process default. Pointers because a replace keeps each field it
	// omits: nil stays off the wire, a pointer to 0 is sent. Depth requires
	// a non-zero wait on the resulting rule.
	FcMaxQueueDepth  *uint32 `json:"fc_max_queue_depth,omitempty"   yaml:"fc_max_queue_depth,omitempty"`
	FcMaxQueueWaitMs *uint32 `json:"fc_max_queue_wait_ms,omitempty" yaml:"fc_max_queue_wait_ms,omitempty"`
	// The rest of the capacity admission gate, the same way: FcMode is
	// off, observe, enforce or inherit (the process default), sent only when
	// given; nil numeric fields stay off the wire, a pointer to 0 restores
	// the process default on a replace.
	FcMode               string  `json:"fc_mode,omitempty"                 yaml:"fc_mode,omitempty"`
	FcMaxOutstanding     *uint32 `json:"fc_max_outstanding,omitempty"      yaml:"fc_max_outstanding,omitempty"`
	FcEpMaxInflight      *uint32 `json:"fc_ep_max_inflight,omitempty"      yaml:"fc_ep_max_inflight,omitempty"`
	FcPrefillMaxInflight *uint32 `json:"fc_prefill_max_inflight,omitempty" yaml:"fc_prefill_max_inflight,omitempty"`
	FcDecodeMaxInflight  *uint32 `json:"fc_decode_max_inflight,omitempty"  yaml:"fc_decode_max_inflight,omitempty"`
	FcTelemetryStaleMs   *uint32 `json:"fc_telemetry_stale_ms,omitempty"   yaml:"fc_telemetry_stale_ms,omitempty"`
	// The adaptive ceiling, under the same presence rules: FcAdaptive is
	// on, off or inherit, sent when given.
	FcAdaptive     string  `json:"fc_adaptive,omitempty"       yaml:"fc_adaptive,omitempty"`
	FcWarmupMs     *uint32 `json:"fc_warmup_ms,omitempty"      yaml:"fc_warmup_ms,omitempty"`
	FcTtftTargetMs *uint32 `json:"fc_ttft_target_ms,omitempty" yaml:"fc_ttft_target_ms,omitempty"`
	// Flow-control state the gateway reports on GET; never sent on create.
	FcEffective *FcEffective `json:"fc_effective,omitempty" yaml:"fc_effective,omitempty"`

	// Active health monitor probe (seen in AI cicd bodies alongside monitor=true).
	ProbeType    string `json:"probetype,omitempty"    yaml:"probetype,omitempty"`
	ProbePort    uint16 `json:"probeport,omitempty"    yaml:"probeport,omitempty"`
	ProbeReq     string `json:"probereq,omitempty"     yaml:"probereq,omitempty"`
	ProbeResp    string `json:"proberesp,omitempty"    yaml:"proberesp,omitempty"`
	ProbeTimeout uint32 `json:"probeTimeout,omitempty" yaml:"probeTimeout,omitempty"`
	ProbeRetries int    `json:"probeRetries,omitempty" yaml:"probeRetries,omitempty"`

	// --- Inference gateway (AI) fields. All live on serviceArguments and are
	// only sent when set (omitempty), so classic LB rules are unaffected.
	// mode=4 (fullproxy) is a prerequisite for every AI feature below.

	// Model routing / L7.
	ModelName       string `json:"model_name,omitempty"          yaml:"model_name,omitempty"`
	PathPrefix      string `json:"path_prefix,omitempty"         yaml:"path_prefix,omitempty"`
	PathMatchMode   string `json:"path_match_mode,omitempty"     yaml:"path_match_mode,omitempty"`
	SessionHdrName  string `json:"session_header_name,omitempty" yaml:"session_header_name,omitempty"`
	TraceType       string `json:"trace_type,omitempty"          yaml:"trace_type,omitempty"`
	BackendProtocol string `json:"backend_protocol,omitempty"    yaml:"backend_protocol,omitempty"`
	SockMapMode     string `json:"sockMapMode,omitempty"         yaml:"sockMapMode,omitempty"`

	// SSE streaming.
	SseMode              bool   `json:"sse_mode,omitempty"                       yaml:"sse_mode,omitempty"`
	APIKeyAuth           string `json:"api_key_auth,omitempty"                   yaml:"api_key_auth,omitempty"`
	MaxStreamDurationSec int32  `json:"max_stream_duration_sec,omitempty"        yaml:"max_stream_duration_sec,omitempty"`
	BackendKeepaliveSec  int32  `json:"backend_keepalive_interval_sec,omitempty" yaml:"backend_keepalive_interval_sec,omitempty"`
	CbEnable             bool   `json:"cb_enable,omitempty"                     yaml:"cb_enable,omitempty"`

	// CHWBL / WRR-hash prefix hashing (sel=8 or sel=10).
	ChwblPrefixHashLevel int  `json:"chwbl_prefix_hash_level,omitempty" yaml:"chwbl_prefix_hash_level,omitempty"`
	ChwblPrefixHashFlags int  `json:"chwbl_prefix_hash_flags,omitempty" yaml:"chwbl_prefix_hash_flags,omitempty"`
	ChwblMeanLoadFactor  int  `json:"chwbl_mean_load_factor,omitempty"  yaml:"chwbl_mean_load_factor,omitempty"`
	ChwblReplication     int  `json:"chwbl_replication,omitempty"       yaml:"chwbl_replication,omitempty"`
	ChwblEnableCacheSalt bool `json:"chwbl_enable_cache_salt,omitempty" yaml:"chwbl_enable_cache_salt,omitempty"`

	// Prefill/decode disaggregation.
	PdDisaggMode          bool  `json:"pd_disagg_mode,omitempty"           yaml:"pd_disagg_mode,omitempty"`
	PdCacheAwareMode      bool  `json:"pd_cache_aware_mode,omitempty"      yaml:"pd_cache_aware_mode,omitempty"`
	PdSessionTtlSec       int32 `json:"pd_session_ttl_sec,omitempty"       yaml:"pd_session_ttl_sec,omitempty"`
	PdCacheThreshold      int32 `json:"pd_cache_threshold,omitempty"       yaml:"pd_cache_threshold,omitempty"`
	PdBalanceAbsThreshold int32 `json:"pd_balance_abs_threshold,omitempty" yaml:"pd_balance_abs_threshold,omitempty"`
	PdBootstrapPort       int32 `json:"pdBootstrapPort,omitempty"          yaml:"pdBootstrapPort,omitempty"`

	// KV-cache-aware exact routing (camelCase keys, per swagger).
	KvExactMode   int64  `json:"kvExactMode,omitempty"   yaml:"kvExactMode,omitempty"`
	KvBlockSize   int64  `json:"kvBlockSize,omitempty"   yaml:"kvBlockSize,omitempty"`
	KvHashAlgo    string `json:"kvHashAlgo,omitempty"    yaml:"kvHashAlgo,omitempty"`
	KvZmqPort     int64  `json:"kvZmqPort,omitempty"     yaml:"kvZmqPort,omitempty"`
	KvWarmupSec   int64  `json:"kvWarmupSec,omitempty"   yaml:"kvWarmupSec,omitempty"`
	KvEngineType  string `json:"kvEngineType,omitempty"  yaml:"kvEngineType,omitempty"`
	KvDpRankCount int32  `json:"kvDpRankCount,omitempty" yaml:"kvDpRankCount,omitempty"`

	// HSTS (https listeners only).
	HstsMaxAge            uint32 `json:"hsts_max_age,omitempty"            yaml:"hsts_max_age,omitempty"`
	HstsIncludeSubdomains bool   `json:"hsts_include_subdomains,omitempty" yaml:"hsts_include_subdomains,omitempty"`
	HstsPreload           bool   `json:"hsts_preload,omitempty"            yaml:"hsts_preload,omitempty"`

	// Mutual TLS (nested; mode=4 only).
	MtlsFrontend *MtlsFrontend `json:"mtls_frontend,omitempty" yaml:"mtls_frontend,omitempty"`
	MtlsBackend  *MtlsBackend  `json:"mtls_backend,omitempty"  yaml:"mtls_backend,omitempty"`
}

// MtlsFrontend configures client-certificate authentication toward downstream
// clients. Valid with security=1 (https) or security=2 (e2ehttps).
type MtlsFrontend struct {
	ClientCertMode   string `json:"client_cert_mode,omitempty"    yaml:"client_cert_mode,omitempty"`
	ClientCAPath     string `json:"client_ca_path,omitempty"      yaml:"client_ca_path,omitempty"`
	ClientCACertData string `json:"client_ca_cert_data,omitempty" yaml:"client_ca_cert_data,omitempty"`
	RequireClientCN  bool   `json:"require_client_cn,omitempty"   yaml:"require_client_cn,omitempty"`
	ClientCNPattern  string `json:"client_cn_pattern,omitempty"   yaml:"client_cn_pattern,omitempty"`
	ClientCRLPath    string `json:"client_crl_path,omitempty"     yaml:"client_crl_path,omitempty"`
}

// MtlsBackend configures loxilb's client-certificate presentation toward the
// backend. Valid only with security=2 (e2ehttps).
type MtlsBackend struct {
	VerifyServerCert bool   `json:"verify_server_cert,omitempty" yaml:"verify_server_cert,omitempty"`
	BackendCAPath    string `json:"backend_ca_path,omitempty"    yaml:"backend_ca_path,omitempty"`
	ClientCertPath   string `json:"client_cert_path,omitempty"   yaml:"client_cert_path,omitempty"`
	ClientKeyPath    string `json:"client_key_path,omitempty"    yaml:"client_key_path,omitempty"`
	ClientCertData   string `json:"client_cert_data,omitempty"   yaml:"client_cert_data,omitempty"`
	ClientKeyData    string `json:"client_key_data,omitempty"    yaml:"client_key_data,omitempty"`
}

type LoadBalancerEndpoint struct {
	EndpointIP string `json:"endpointIP" yaml:"endpointIP"`
	TargetPort uint16 `json:"targetPort" yaml:"targetPort"`
	Weight     uint8  `json:"weight"     yaml:"weight"`
	State      string `json:"state"      yaml:"state"`
	Counter    string `json:"counter"    yaml:"counter"`
	// Inference gateway per-endpoint fields (P/D disaggregation).
	EpRole   int32 `json:"ep_role,omitempty"   yaml:"ep_role,omitempty"`
	NixlPort int32 `json:"nixl_port,omitempty" yaml:"nixl_port,omitempty"`
}

type LoadBalancerSecIp struct {
	SecondaryIP string `json:"secondaryIP" yaml:"secondaryIP"`
}

type LbAllowedSrcIPArg struct {
	// Prefix - Allowed Prefix
	Prefix string `json:"prefix" yaml:"prefix"`
}

type ConfigurationLBFile struct {
	TypeMeta   `yaml:",inline"`
	ObjectMeta `yaml:"metadata,omitempty"`
	Spec       LoadBalancerModel `yaml:"spec"`
}

func (service LoadBalancerService) Key() string {
	return fmt.Sprintf("%s|%05d|%s", service.ExternalIP, service.Port, service.Protocol)
}

func (lbresp LbRuleModGet) Sort() {
	sort.Slice(lbresp.LbRules, func(i, j int) bool {
		return lbresp.LbRules[i].Service.Key() < lbresp.LbRules[j].Service.Key()
	})
}
