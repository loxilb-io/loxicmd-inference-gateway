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
package create

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type CreateLoadBalancerOptions struct {
	ExternalIP     string
	TCP            []string
	UDP            []string
	ICMP           bool
	Mode           string
	BGP            bool
	Security       string
	Monitor        bool
	Attach         bool
	Detach         bool
	Timeout        uint32
	Mark           uint32
	SCTP           []string
	Endpoints      []string
	SecIPs         []string
	Select         string
	Name           string
	Host           string
	AllowedSources []string
	PPv2En         bool
	Egress         bool

	// Active health monitor probe.
	ProbeType    string
	ProbePort    uint16
	ProbeReq     string
	ProbeTimeout uint32
	ProbeRetries int

	// --- Inference gateway (AI) options (require --mode fullproxy) ---
	// Model routing / L7.
	ModelName         string
	PathPrefix        string
	PathMatchMode     string
	SessionHeaderName string
	TraceType         string
	BackendProtocol   string
	// SSE.
	SseMode              bool
	APIKeyAuth           string
	MaxStreamDurationSec int32
	BackendKeepaliveSec  int32
	CbEnable             bool
	// CHWBL / WRR-hash.
	ChwblPrefixHashLevel int
	ChwblPrefixHashFlags int
	ChwblMeanLoadFactor  int
	ChwblReplication     int
	ChwblEnableCacheSalt bool
	// Prefill/decode disaggregation.
	PdDisaggMode          bool
	PdCacheAwareMode      bool
	PdSessionTtlSec       int32
	PdCacheThreshold      int32
	PdBalanceAbsThreshold int32
	PdBootstrapPort       int32
	// KV-cache-aware routing.
	KvExactMode   int64
	KvBlockSize   int64
	KvHashAlgo    string
	KvZmqPort     int64
	KvWarmupSec   int64
	KvEngineType  string
	KvDpRankCount int32
	// Per-endpoint (aligned to --endpoints order).
	EpRoles   []string
	NixlPorts []int
	// HSTS.
	HstsMaxAge            uint32
	HstsIncludeSubdomains bool
	HstsPreload           bool
	// mTLS frontend.
	MtlsClientCertMode  string
	MtlsClientCAPath    string
	MtlsRequireClientCN bool
	MtlsClientCNPattern string
	MtlsClientCRLPath   string
	// mTLS backend.
	MtlsBackendVerifyServer   bool
	MtlsBackendCAPath         string
	MtlsBackendClientCertPath string
	MtlsBackendClientKeyPath  string
}

type CreateLoadBalancerResult struct {
	Result string `json:"result"`
}

const CreateLoadBalancerSuccess = "success"

func ReadCreateLoadBalancerOptions(o *CreateLoadBalancerOptions, args []string) error {
	if len(args) > 1 {
		fmt.Println("create lb command get so many args")
		fmt.Println(args)
	} else if len(args) <= 0 {
		return errors.New("create lb need EXTERNAL-IP args")
	}

	if val := net.ParseIP(args[0]); val != nil {
		o.ExternalIP = args[0]
	} else {
		return fmt.Errorf("externel IP '%s' is invalid format", args[0])
	}
	return nil
}

func SelectToNum(sel string) int {
	var ret int
	switch sel {
	case "rr":
		ret = 0
	case "hash":
		ret = 1
	case "priority":
		ret = 2
	case "persist":
		ret = 3
	case "lc":
		ret = 4
	case "n2":
		ret = 5
	case "n3":
		ret = 6
	case "chwbl":
		ret = 8
	case "gpuaware":
		ret = 9
	case "wrr-hash", "chwbl-wrr":
		ret = 10
	default:
		ret = 0
	}
	return ret
}

// EpRoleToNum maps an endpoint role name to its numeric value.
// 0 = normal, 1 = prefill, 2 = decode. Numeric input is accepted as-is.
func EpRoleToNum(role string) (int32, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "normal", "0":
		return 0, nil
	case "prefill", "1":
		return 1, nil
	case "decode", "2":
		return 2, nil
	default:
		return 0, fmt.Errorf("endpoint role '%s' is invalid (want normal|prefill|decode)", role)
	}
}

func ModeToNum(sel string) int {
	var ret int
	switch sel {
	case "onearm":
		ret = 1
	case "fullnat":
		ret = 2
	case "dsr":
		ret = 3
	case "fullproxy":
		ret = 4
	case "hostonearm":
		ret = 5
	default:
		ret = 0
	}
	return ret
}

func SecStringToNum(sec string) int {
	var ret int
	switch sec {
	case "https":
		ret = 1
	case "tls":
		ret = 1
	case "e2ehttps":
		ret = 2
	case "e2etls":
		ret = 2
	default:
		ret = 0
	}
	return ret
}

func NewCreateLoadBalancerCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateLoadBalancerOptions{}

	var createLbCmd = &cobra.Command{
		Use:   "lb IP [--select=<rr|hash|priority|persist>] [--tcp=<ports>:<targetPorts>] [--udp=<ports>:<targetPorts>] [--sctp=<ports>:<targetPorts>] [--icmp] [--mark=<val>] [--secips=<ip>,] [--sources=<ip>,] [--endpoints=<ip>:<weight>,] [--mode=<onearm|fullnat>] [--bgp] [--monitor] [--inatimeout=<to>] [--name=<service-name>] [--attachEP] [--detachEP] [--security=<https|e2ehttps|none>] [--host=<url>] [--ppv2en] [--egress]",
		Short: "Create a LoadBalancer",
		Long: `Create a LoadBalancer

--select value options
  	rr - select the lb end-points based on round-robin
	hash - select the lb end-points based on hashing
	priority - select the lb based on weighted round-robin
	persist - select the lb end-point based on sender
	n2 - select the lb end-point base on N2 interface params (only available with fullproxy mode)
	n3 - select the lb end-point base on N3 interface params
	lc - select the lb end-point base on least connection

--mode value options
	onearm - LB put LB-IP as srcIP
	fullnat - LB put Service IP as scrIP
	dsr - LB in DSR mode allows return traffic to bypass the load balancer (only available with hash select)
	fullproxy - LB operating as a L7 proxy
	hostonearm - LB operating in host one-arm

ex)
	loxicmd create lb 192.168.0.200 --tcp=80:32015 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1
	loxicmd create lb 192.168.0.200 --tcp=8080-8081:32015 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1
	loxicmd create lb 192.168.0.200 --tcp=5000:5201-5300 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1
	loxicmd create lb 192.168.0.200 --tcp=80:32015 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1 --security=https
	loxicmd create lb 192.168.0.200 --tcp=80:32015 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1 --host=loxilb.io
	loxicmd create lb 192.168.0.200 --tcp=80:32015 --name="http-service" --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1
	loxicmd create lb 192.168.0.200 --udp=80:32015 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1 --mark=10
	loxicmd create lb 192.168.0.200 --tcp=80:32015 --udp=80:32015 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1
	loxicmd create lb 192.168.0.200 --select=hash --tcp=80:32015 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1
	loxicmd create lb 192.168.0.200 --tcp=80:80 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1 --mode=dsr --select=hash
	loxicmd create lb 192.168.0.200 --sctp=37412:38412 --secips=192.168.0.201,192.168.0.202 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1
	loxicmd create lb 192.168.0.200 --tcp=80:32015 --endpoints=10.212.0.1:1,10.212.0.2:1,10.212.0.3:1 --sources=10.10.10.1/32

	loxicmd create lb  2001::1 --tcp=2020:8080 --endpoints=4ffe::1:1,5ffe::1:1,6ffe::1:1
	loxicmd create lb  2001::1 --tcp=2020:8080 --endpoints=31.31.31.1:1,32.32.32.1:1,33.33.33.1:1
	loxicmd create lb 10.10.10.254 --sctp=2020:8080 --endpoints=33.33.33.1:1 --attachEP
	loxicmd create lb 100.100.100.1 --tcp=8080:80 --endpoints=10.10.10.1:1 --ppv2en
	`,
		PreRun: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				cmd.Help()
				os.Exit(0)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			var sctp bool
			if err := ReadCreateLoadBalancerOptions(&o, args); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			if err := validateLBAIOptions(&o); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}

			ProtoPortpair := make(map[string][]string)
			// TCP LoadBalancer
			if len(o.TCP) > 0 {
				ProtoPortpair["tcp"] = o.TCP
			}
			if len(o.UDP) > 0 {
				ProtoPortpair["udp"] = o.UDP
			}
			if len(o.SCTP) > 0 {
				ProtoPortpair["sctp"] = o.SCTP
				sctp = true
			}
			if o.ICMP {
				//icmpProtoPortpair := make(map[string][]string)
				ProtoPortpair["icmp"] = []string{"0:0"}
			}
			if !sctp && len(o.SecIPs) > 0 {
				fmt.Printf("Secondary IPs allowed in SCTP only\n")
				return
			}

			// Common part of the load balancer. Endpoints are parsed in order so
			// per-endpoint AI attributes (ep-role, nixl-port) align by index.
			endpointList, err := GetEndpointList(o.Endpoints)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			epRoles, err := parseEndpointRoles(o.EpRoles, len(endpointList))
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			nixlPorts, err := alignNixlPorts(o.NixlPorts, len(endpointList))
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			for proto, portPairList := range ProtoPortpair {
				portTargetPorts, err := GetPortPairList(portPairList)
				if err != nil {
					fmt.Printf("Error: %s\n", err.Error())
					return
				}
				if len(portTargetPorts) <= 0 || len(portTargetPorts) > 2 {
					fmt.Printf("portPair: None specified\n")
					return
				}

				startSPort := uint16(0)
				endSPort := uint16(0)
				first := false

				for sPort := range portTargetPorts {
					if !first {
						startSPort = sPort
						first = true
					} else {
						if sPort > startSPort {
							endSPort = sPort
						} else {
							endSPort = startSPort
							startSPort = sPort
						}
					}
				}

				lbModel := api.LoadBalancerModel{}
				oper := 0
				if o.Attach {
					oper = 1
				} else if o.Detach {
					oper = 2
				}
				lbService := api.LoadBalancerService{
					ExternalIP: o.ExternalIP,
					Protocol:   proto,
					Port:       startSPort,
					PortMax:    endSPort,
					Sel:        api.EpSelect(SelectToNum(o.Select)),
					BGP:        o.BGP,
					Monitor:    o.Monitor,
					Mode:       api.LbMode(ModeToNum(o.Mode)),
					Timeout:    o.Timeout,
					Block:      o.Mark,
					Name:       o.Name,
					Oper:       api.LbOP(oper),
					Security:   api.LbSec(SecStringToNum(o.Security)),
					Host:       o.Host,
					PpV2:       o.PPv2En,
					Egress:     o.Egress,
				}
				applyAIServiceOptions(&lbService, &o)

				lbModel.Service = lbService
				targetPorts := portTargetPorts[startSPort]
				for i, e := range endpointList {
					for _, targetPort := range targetPorts {
						if o.Mode == "dsr" && targetPort != startSPort {
							fmt.Printf("Error: No port-translation in dsr mode\n")
							return
						}
						ep := api.LoadBalancerEndpoint{
							EndpointIP: e.IP,
							TargetPort: targetPort,
							Weight:     e.Weight,
							EpRole:     epRoles[i],
							NixlPort:   nixlPorts[i],
						}
						lbModel.Endpoints = append(lbModel.Endpoints, ep)
					}
				}

				for _, sip := range o.SecIPs {
					sp := api.LoadBalancerSecIp{
						SecondaryIP: sip,
					}
					lbModel.SecondaryIPs = append(lbModel.SecondaryIPs, sp)
				}

				for _, sip := range o.AllowedSources {
					sp := api.LbAllowedSrcIPArg{
						Prefix: sip,
					}
					lbModel.SrcIPs = append(lbModel.SrcIPs, sp)
				}

				resp, err := LoadbalancerAPICall(restOptions, lbModel)
				if err != nil {
					fmt.Printf("Error: %s\n", err.Error())
					return
				}

				defer resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					PrintCreateResult(resp, *restOptions)
					return
				}
				// Surface a non-2xx server rejection instead of silently
				// no-op'ing (e.g. an invalid --kv-hash-algo enum). Without this the
				// command exits 0 with no rule created and no diagnostic.
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}
		},
	}

	createLbCmd.Flags().StringSliceVar(&o.TCP, "tcp", o.TCP, "Port pairs can be specified as '<port>:<targetPort>'")
	createLbCmd.Flags().StringSliceVar(&o.UDP, "udp", o.UDP, "Port pairs can be specified as '<port>:<targetPort>'")
	createLbCmd.Flags().StringSliceVar(&o.SCTP, "sctp", o.SCTP, "Port pairs can be specified as '<port>:<targetPort>'")
	createLbCmd.Flags().BoolVarP(&o.ICMP, "icmp", "", false, "ICMP Ping packet Load balancer")
	createLbCmd.Flags().StringVarP(&o.Mode, "mode", "", o.Mode, "NAT mode for load balancer rule")
	createLbCmd.Flags().BoolVarP(&o.BGP, "bgp", "", false, "Enable BGP in the load balancer")
	createLbCmd.Flags().BoolVarP(&o.Monitor, "monitor", "", false, "Enable monitoring end-points of this rule")
	createLbCmd.Flags().StringSliceVar(&o.SecIPs, "secips", o.SecIPs, "Secondary IPs for SCTP multihoming rule specified as '<secondaryIP>'")
	createLbCmd.Flags().StringVarP(&o.Select, "select", "", "rr", "Select the hash algorithm for the load balance.(ex) rr, hash, priority, persist, lc")
	createLbCmd.Flags().Uint32VarP(&o.Timeout, "inatimeout", "", 0, "Specify the timeout (in seconds) after which a LB session will be reset for inactivity")
	createLbCmd.Flags().Uint32VarP(&o.Mark, "mark", "", 0, "Specify the mark num to segregate a load-balancer VIP service")
	createLbCmd.Flags().StringSliceVar(&o.Endpoints, "endpoints", o.Endpoints, "Endpoints is pairs that can be specified as '<endpointIP>:<Weight>'")
	createLbCmd.Flags().StringVarP(&o.Name, "name", "", o.Name, "Name for load balancer rule")
	createLbCmd.Flags().BoolVarP(&o.Attach, "attachEP", "", false, "Attach endpoints to the load balancer rule")
	createLbCmd.Flags().BoolVarP(&o.Detach, "detachEP", "", false, "Detach endpoints from the load balancer rule")
	createLbCmd.Flags().StringVarP(&o.Security, "security", "", o.Security, "Security mode for load balancer rule")
	createLbCmd.Flags().StringVarP(&o.Host, "host", "", o.Host, "Ingress Host URL Path")
	createLbCmd.Flags().StringSliceVar(&o.AllowedSources, "sources", o.AllowedSources, "Allowed sources for this rule as '<allowedSources>'")
	createLbCmd.Flags().BoolVarP(&o.PPv2En, "ppv2en", "", false, "Enable proxy protocol v2")
	createLbCmd.Flags().BoolVarP(&o.Egress, "egress", "", false, "Specify egress rule")

	// Active health monitor probe.
	createLbCmd.Flags().StringVar(&o.ProbeType, "probetype", "", "Health probe type (ex) http, https, tcp, ping")
	createLbCmd.Flags().Uint16Var(&o.ProbePort, "probeport", 0, "Health probe port")
	createLbCmd.Flags().StringVar(&o.ProbeReq, "probereq", "", "Health probe request path/string (ex) /health")
	createLbCmd.Flags().Uint32Var(&o.ProbeTimeout, "probetimeout", 0, "Health probe timeout in seconds")
	createLbCmd.Flags().IntVar(&o.ProbeRetries, "proberetries", 0, "Health probe retry count")

	// --- Inference gateway (AI) flags. All require --mode fullproxy. ---
	// Model routing / L7.
	createLbCmd.Flags().StringVar(&o.ModelName, "model-name", "", "LLM model name for pool selection (empty = wildcard/fallback pool)")
	createLbCmd.Flags().StringVar(&o.PathPrefix, "path-prefix", "", "L7 path prefix to match (ex) /v1")
	createLbCmd.Flags().StringVar(&o.PathMatchMode, "path-match-mode", "", "Path match mode: disabled|prefix|exact")
	createLbCmd.Flags().StringVar(&o.SessionHeaderName, "session-header-name", "", "Header for session stickiness with --select=persist (ex) mcp-session-id, X-Conversation-Id, cookie:<name>")
	createLbCmd.Flags().StringVar(&o.TraceType, "trace-type", "", "Deep-inspection trace catalog (ex) anthropic, mcp, v1")
	createLbCmd.Flags().StringVar(&o.BackendProtocol, "backend-protocol", "", "Backend transport: http1|http2|both")
	// SSE.
	createLbCmd.Flags().BoolVar(&o.SseMode, "sse-mode", false, "Enable SSE streaming mode (suppresses idle timeout during text/event-stream)")
	createLbCmd.Flags().StringVar(&o.APIKeyAuth, "api-key-auth", "", "Data-plane X-Api-Key policy: disabled|required (default: disabled)")
	createLbCmd.Flags().Int32Var(&o.MaxStreamDurationSec, "max-stream-duration", 0, "Max SSE stream duration in seconds (0 = system cap)")
	createLbCmd.Flags().Int32Var(&o.BackendKeepaliveSec, "backend-keepalive-interval", 0, "Backend TCP keepalive interval in seconds during stream (0 = off)")
	createLbCmd.Flags().BoolVar(&o.CbEnable, "cb-enable", false, "Enable the per-endpoint circuit breaker")
	// CHWBL / WRR-hash (with --select=chwbl or --select=chwbl-wrr).
	createLbCmd.Flags().IntVar(&o.ChwblPrefixHashLevel, "chwbl-hash-level", 0, "CHWBL prefix hash level 1|2|3 (1=system+model, 2=+session, 3=+RAG)")
	createLbCmd.Flags().IntVar(&o.ChwblPrefixHashFlags, "chwbl-hash-flags", 0, "CHWBL prefix hash flags bitmask 0-255 (0 = auto-detect)")
	createLbCmd.Flags().IntVar(&o.ChwblMeanLoadFactor, "chwbl-load-factor", 0, "CHWBL bounded-load factor 100-300 (default 125)")
	createLbCmd.Flags().IntVar(&o.ChwblReplication, "chwbl-replication", 0, "CHWBL virtual nodes per endpoint 1-1024 (default 100)")
	createLbCmd.Flags().BoolVar(&o.ChwblEnableCacheSalt, "chwbl-cache-salt", false, "Require cache_salt in requests for tenant isolation")
	// Prefill/decode disaggregation.
	createLbCmd.Flags().BoolVar(&o.PdDisaggMode, "pd-disagg", false, "Enable prefill/decode disaggregation")
	createLbCmd.Flags().BoolVar(&o.PdCacheAwareMode, "pd-cache-aware", false, "Enable cache-aware P/D endpoint selection (requires --pd-disagg)")
	createLbCmd.Flags().Int32Var(&o.PdSessionTtlSec, "pd-session-ttl", 0, "P/D session stickiness TTL in seconds (0 = no expiry)")
	createLbCmd.Flags().Int32Var(&o.PdCacheThreshold, "pd-cache-threshold", 0, "P/D cache-match threshold 0-100 (default 20)")
	createLbCmd.Flags().Int32Var(&o.PdBalanceAbsThreshold, "pd-balance-abs-threshold", 0, "P/D absolute connection imbalance threshold (default 3)")
	createLbCmd.Flags().Int32Var(&o.PdBootstrapPort, "pd-bootstrap-port", 0, "SGLang P/D bootstrap port (0 = engine default 8998)")
	// KV-cache-aware routing.
	createLbCmd.Flags().Int64Var(&o.KvExactMode, "kv-exact-mode", 0, "KV-cache exact routing: 0=off, 1=zmq P/D, 3=zmq single-role")
	createLbCmd.Flags().Int64Var(&o.KvBlockSize, "kv-block-size", 0, "KV token block size (default 16; must match engine)")
	createLbCmd.Flags().StringVar(&o.KvHashAlgo, "kv-hash-algo", "", "KV block hash algo: sha256_cbor|xxhash_cbor|sha256_sglang|blockhash_trtllm (prefer omit for engine default)")
	createLbCmd.Flags().Int64Var(&o.KvZmqPort, "kv-zmq-port", 0, "KV ZMQ publisher port on prefill endpoints (default 5557)")
	createLbCmd.Flags().Int64Var(&o.KvWarmupSec, "kv-warmup", 0, "KV subscriber warmup seconds before activation (default 30)")
	createLbCmd.Flags().StringVar(&o.KvEngineType, "kv-engine-type", "", "Inference engine: vllm|sglang|trtllm|llamacpp (immutable per rule after create)")
	createLbCmd.Flags().Int32Var(&o.KvDpRankCount, "kv-dp-ranks", 0, "SGLang data-parallel rank count 1-8 (default 1)")
	// Per-endpoint (aligned to --endpoints order).
	createLbCmd.Flags().StringSliceVar(&o.EpRoles, "ep-role", o.EpRoles, "Per-endpoint role aligned to --endpoints: normal|prefill|decode")
	createLbCmd.Flags().IntSliceVar(&o.NixlPorts, "nixl-port", o.NixlPorts, "Per-endpoint NIXL KV-transfer port aligned to --endpoints")
	// HSTS.
	createLbCmd.Flags().Uint32Var(&o.HstsMaxAge, "hsts-max-age", 0, "HSTS max-age seconds (0 = no HSTS)")
	createLbCmd.Flags().BoolVar(&o.HstsIncludeSubdomains, "hsts-include-subdomains", false, "Add includeSubDomains to HSTS header")
	createLbCmd.Flags().BoolVar(&o.HstsPreload, "hsts-preload", false, "Add preload to HSTS header")
	// mTLS frontend.
	createLbCmd.Flags().StringVar(&o.MtlsClientCertMode, "mtls-client-cert-mode", "", "mTLS frontend client cert mode: disabled|optional|required")
	createLbCmd.Flags().StringVar(&o.MtlsClientCAPath, "mtls-client-ca-path", "", "mTLS frontend client CA bundle path")
	createLbCmd.Flags().BoolVar(&o.MtlsRequireClientCN, "mtls-require-client-cn", false, "mTLS frontend: enforce client CN pattern")
	createLbCmd.Flags().StringVar(&o.MtlsClientCNPattern, "mtls-client-cn-pattern", "", "mTLS frontend client CN glob pattern")
	createLbCmd.Flags().StringVar(&o.MtlsClientCRLPath, "mtls-client-crl-path", "", "mTLS frontend CRL path")
	// mTLS backend (security=e2ehttps only).
	createLbCmd.Flags().BoolVar(&o.MtlsBackendVerifyServer, "mtls-backend-verify-server", false, "mTLS backend: verify backend server certificate")
	createLbCmd.Flags().StringVar(&o.MtlsBackendCAPath, "mtls-backend-ca-path", "", "mTLS backend CA path (empty = system store)")
	createLbCmd.Flags().StringVar(&o.MtlsBackendClientCertPath, "mtls-backend-cert-path", "", "mTLS backend client certificate path")
	createLbCmd.Flags().StringVar(&o.MtlsBackendClientKeyPath, "mtls-backend-key-path", "", "mTLS backend client key path")

	return createLbCmd
}

func PrintCreateResult(resp *http.Response, o api.RESTOptions) {
	result := CreateLoadBalancerResult{}
	resultByte, err := io.ReadAll(resp.Body)
	//fmt.Printf("Debug: response.Body: %s\n", string(resultByte))

	if err != nil {
		fmt.Printf("Error: Failed to read HTTP response: (%s)\n", err.Error())
		return
	}
	if err := json.Unmarshal(resultByte, &result); err != nil {
		fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
		return
	}

	if o.PrintOption == "json" {
		// TODO: need to test MarshalIndent
		resultIndent, _ := json.MarshalIndent(resp.Body, "", "\t")
		fmt.Println(string(resultIndent))
		return
	}

	fmt.Printf("%s\n", result.Result)
}

func GetPortPairList(portPairStrList []string) (map[uint16][]uint16, error) {
	result := make(map[uint16][]uint16)
	for _, portPairStr := range portPairStrList {
		portPair := strings.Split(portPairStr, ":")
		if len(portPair) != 2 {
			continue
		}

		servicePorts := strings.Split(portPair[0], "-")

		// 0 is port, 1 is targetPort
		var portList []int
		for _, servicePort := range servicePorts {
			port, err := strconv.Atoi(servicePort)
			if err != nil {
				return nil, fmt.Errorf("port '%s' is not integer", servicePort)
			}
			portList = append(portList, port)
		}

		var err error
		startTP := 0
		endTP := 0

		targetPortRange := strings.Split(portPair[1], "-")
		if len(targetPortRange) > 2 {
			continue
		} else if len(targetPortRange) == 2 {
			startTP, err = strconv.Atoi(targetPortRange[0])
			if err != nil {
				return nil, fmt.Errorf("targetPort0 '%s' is not integer", targetPortRange[0])
			}
			endTP, err = strconv.Atoi(targetPortRange[1])
			if err != nil {
				return nil, fmt.Errorf("targetPort1 '%s' is not integer", targetPortRange[1])
			}
			if endTP < startTP {
				return nil, fmt.Errorf("targetPort2 '%s' <  targetPort2 '%s'", targetPortRange[1], targetPortRange[0])
			}
		} else {
			startTP, err = strconv.Atoi(targetPortRange[0])
			if err != nil {
				return nil, fmt.Errorf("targetPort0 '%s' is not integer", targetPortRange[0])
			}
			endTP = startTP
		}

		for _, port := range portList {
			result[uint16(port)] = make([]uint16, 0)

			for targetPort := startTP; targetPort <= endTP; targetPort++ {
				result[uint16(port)] = append(result[uint16(port)], uint16(targetPort))
			}
		}
	}
	return result, nil
}

// EndpointSpec is an ordered (ip, weight) endpoint parsed from --endpoints.
type EndpointSpec struct {
	IP     string
	Weight uint8
}

// GetEndpointList parses --endpoints entries in order, preserving position so
// per-endpoint AI attributes (--ep-role, --nixl-port) align by index. IPv6
// endpoints are supported because the weight is split off the last ':'.
func GetEndpointList(endpointsList []string) ([]EndpointSpec, error) {
	var result []EndpointSpec
	for _, endpointStr := range endpointsList {
		weightIdx := strings.LastIndex(endpointStr, ":")
		if weightIdx < 0 {
			return nil, fmt.Errorf("endpoint '%s' is invalid format", endpointStr)
		}
		// 0 is endpoint IP, 1 is weight
		weight, err := strconv.Atoi(endpointStr[weightIdx+1:])
		if err != nil {
			return nil, fmt.Errorf("endpoint's weight '%s' is invalid format", endpointStr[weightIdx+1:])
		}
		result = append(result, EndpointSpec{IP: endpointStr[:weightIdx], Weight: uint8(weight)})
	}
	return result, nil
}

// parseEndpointRoles converts --ep-role values to numeric roles, aligned to the
// endpoint count. Returns a zero-filled slice when no roles are supplied.
func parseEndpointRoles(roles []string, epCount int) ([]int32, error) {
	out := make([]int32, epCount)
	if len(roles) == 0 {
		return out, nil
	}
	if len(roles) != epCount {
		return nil, fmt.Errorf("--ep-role count (%d) must match --endpoints count (%d)", len(roles), epCount)
	}
	for i, r := range roles {
		n, err := EpRoleToNum(r)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// alignNixlPorts validates --nixl-port against the endpoint count and returns a
// zero-filled slice when none are supplied.
func alignNixlPorts(ports []int, epCount int) ([]int32, error) {
	out := make([]int32, epCount)
	if len(ports) == 0 {
		return out, nil
	}
	if len(ports) != epCount {
		return nil, fmt.Errorf("--nixl-port count (%d) must match --endpoints count (%d)", len(ports), epCount)
	}
	for i, p := range ports {
		out[i] = int32(p)
	}
	return out, nil
}

// lbMtlsFrontendRequested reports whether any mTLS frontend option is set.
func lbMtlsFrontendRequested(o *CreateLoadBalancerOptions) bool {
	return o.MtlsClientCertMode != "" || o.MtlsClientCAPath != "" || o.MtlsRequireClientCN ||
		o.MtlsClientCNPattern != "" || o.MtlsClientCRLPath != ""
}

// lbMtlsBackendRequested reports whether any mTLS backend option is set.
func lbMtlsBackendRequested(o *CreateLoadBalancerOptions) bool {
	return o.MtlsBackendVerifyServer || o.MtlsBackendCAPath != "" ||
		o.MtlsBackendClientCertPath != "" || o.MtlsBackendClientKeyPath != ""
}

// lbAIRequested reports whether any inference-gateway option that requires L7
// fullproxy termination is set.
//
// NOTE: --backend-protocol is deliberately excluded. The backend transport
// (http1|http2|both) is a per-endpoint capability the datapath honors on plain
// L4 rules too (e.g. the http2ep scenario: an h2c backend behind a default-mode
// TCP VIP). The REST API applies backend_protocol in any mode, so forcing
// --mode fullproxy here would make the CLI stricter than the API it drives.
func lbAIRequested(o *CreateLoadBalancerOptions) bool {
	sel := SelectToNum(o.Select)
	return o.ModelName != "" || o.PathPrefix != "" || o.PathMatchMode != "" ||
		o.SessionHeaderName != "" || o.TraceType != "" ||
		o.SseMode || o.APIKeyAuth != "" || o.MaxStreamDurationSec != 0 || o.BackendKeepaliveSec != 0 || o.CbEnable ||
		o.ChwblPrefixHashLevel != 0 || o.ChwblPrefixHashFlags != 0 || o.ChwblMeanLoadFactor != 0 ||
		o.ChwblReplication != 0 || o.ChwblEnableCacheSalt ||
		o.PdDisaggMode || o.PdCacheAwareMode || o.PdSessionTtlSec != 0 ||
		o.PdCacheThreshold != 0 || o.PdBalanceAbsThreshold != 0 || o.PdBootstrapPort != 0 ||
		o.KvExactMode != 0 || o.KvBlockSize != 0 || o.KvHashAlgo != "" || o.KvZmqPort != 0 ||
		o.KvWarmupSec != 0 || o.KvEngineType != "" || o.KvDpRankCount != 0 ||
		len(o.EpRoles) > 0 || len(o.NixlPorts) > 0 ||
		o.HstsMaxAge != 0 || o.HstsIncludeSubdomains || o.HstsPreload ||
		lbMtlsFrontendRequested(o) || lbMtlsBackendRequested(o) ||
		sel == 8 || sel == 9 || sel == 10
}

// validateLBAIOptions enforces the documented cross-field constraints before
// building the request.
func validateLBAIOptions(o *CreateLoadBalancerOptions) error {
	switch o.APIKeyAuth {
	case "", "disabled", "required":
	default:
		return fmt.Errorf("--api-key-auth must be one of disabled|required")
	}
	if !lbAIRequested(o) {
		return nil
	}
	if ModeToNum(o.Mode) != 4 {
		return fmt.Errorf("AI features require '--mode fullproxy'")
	}
	if o.PdCacheAwareMode && !o.PdDisaggMode {
		return fmt.Errorf("--pd-cache-aware requires --pd-disagg")
	}
	return validateKVEngineOptions(o)
}

func validateKVEngineOptions(o *CreateLoadBalancerOptions) error {
	engine := o.KvEngineType
	if engine == "" {
		engine = "vllm"
	}
	switch engine {
	case "vllm", "sglang", "trtllm", "llamacpp":
	default:
		return fmt.Errorf("--kv-engine-type must be one of vllm|sglang|trtllm|llamacpp")
	}
	if o.KvDpRankCount < 0 || o.KvDpRankCount > 8 {
		return fmt.Errorf("--kv-dp-ranks must be within 1..8 (0 = default 1)")
	}
	if o.KvZmqPort < 0 || o.KvZmqPort > 65535 {
		return fmt.Errorf("--kv-zmq-port must be within 1..65535 (0 = server default)")
	}
	if o.KvBlockSize < 0 || o.KvWarmupSec < 0 {
		return fmt.Errorf("--kv-block-size and --kv-warmup must be non-negative")
	}
	if o.KvExactMode != 0 && o.KvExactMode != 1 && o.KvExactMode != 3 {
		return fmt.Errorf("--kv-exact-mode must be one of 0|1|3")
	}
	if o.KvExactMode == 1 && !o.PdDisaggMode {
		return fmt.Errorf("--kv-exact-mode=1 requires --pd-disagg")
	}
	if o.KvExactMode == 3 && o.PdDisaggMode {
		return fmt.Errorf("--kv-exact-mode=3 is incompatible with --pd-disagg")
	}
	if (o.KvExactMode == 1 || o.KvExactMode == 3) && o.KvDpRankCount > 0 {
		base := o.KvZmqPort
		if base == 0 {
			base = 5557
		}
		if base+int64(o.KvDpRankCount)-1 > 65535 {
			return fmt.Errorf("--kv-zmq-port + --kv-dp-ranks - 1 must be <= 65535")
		}
	}

	if err := validateKVHashForEngine(o.KvHashAlgo, engine); err != nil {
		return err
	}
	if o.PdBootstrapPort < 0 || o.PdBootstrapPort > 65535 {
		return fmt.Errorf("--pd-bootstrap-port must be within 0..65535")
	}
	if o.PdBootstrapPort != 0 && !(o.PdDisaggMode && engine == "sglang") {
		return fmt.Errorf("--pd-bootstrap-port requires --pd-disagg and --kv-engine-type=sglang")
	}
	if o.PdDisaggMode {
		hasPrefill, hasDecode := false, false
		for _, role := range o.EpRoles {
			value, err := EpRoleToNum(role)
			if err != nil {
				return err
			}
			hasPrefill = hasPrefill || value == 1
			hasDecode = hasDecode || value == 2
		}
		if !hasPrefill || !hasDecode {
			return fmt.Errorf("--pd-disagg requires at least one prefill and one decode --ep-role")
		}
	}

	switch engine {
	case "trtllm":
		if o.KvZmqPort != 0 && o.KvZmqPort != 5557 {
			return fmt.Errorf("--kv-zmq-port is not used by trtllm; omit it")
		}
		if o.KvDpRankCount > 1 {
			return fmt.Errorf("--kv-dp-ranks is not used by trtllm; omit it")
		}
	case "llamacpp":
		if o.KvExactMode != 0 {
			return fmt.Errorf("--kv-exact-mode is unsupported for llamacpp")
		}
		if o.PdDisaggMode {
			return fmt.Errorf("--pd-disagg is unsupported for llamacpp")
		}
		if o.KvZmqPort != 0 && o.KvZmqPort != 5557 {
			return fmt.Errorf("--kv-zmq-port is not used by llamacpp; omit it")
		}
		if o.KvDpRankCount > 1 {
			return fmt.Errorf("--kv-dp-ranks is not used by llamacpp; omit it")
		}
		if o.KvBlockSize != 0 && o.KvBlockSize != 16 {
			return fmt.Errorf("--kv-block-size is not used by llamacpp; omit it")
		}
	}
	return nil
}

func validateKVHashForEngine(hash, engine string) error {
	if hash == "" {
		return nil
	}
	allowed := map[string]map[string]bool{
		"vllm":     {"sha256_cbor": true, "xxhash_cbor": true},
		"sglang":   {"sha256_sglang": true},
		"trtllm":   {"blockhash_trtllm": true},
		"llamacpp": {},
	}
	if allowed[engine][hash] {
		return nil
	}
	if engine == "llamacpp" {
		return fmt.Errorf("--kv-hash-algo is unsupported for --kv-engine-type=llamacpp; omit it")
	}
	return fmt.Errorf("--kv-hash-algo %q is incompatible with --kv-engine-type=%s; omit it for the engine default", hash, engine)
}

// applyAIServiceOptions copies inference-gateway options onto the service model.
func applyAIServiceOptions(s *api.LoadBalancerService, o *CreateLoadBalancerOptions) {
	// Active health monitor probe.
	s.ProbeType = o.ProbeType
	s.ProbePort = o.ProbePort
	s.ProbeReq = o.ProbeReq
	s.ProbeTimeout = o.ProbeTimeout
	s.ProbeRetries = o.ProbeRetries
	// Model routing / L7.
	s.ModelName = o.ModelName
	s.PathPrefix = o.PathPrefix
	s.PathMatchMode = o.PathMatchMode
	s.SessionHdrName = o.SessionHeaderName
	s.TraceType = o.TraceType
	s.BackendProtocol = o.BackendProtocol
	// SSE.
	s.SseMode = o.SseMode
	s.APIKeyAuth = o.APIKeyAuth
	s.MaxStreamDurationSec = o.MaxStreamDurationSec
	s.BackendKeepaliveSec = o.BackendKeepaliveSec
	s.CbEnable = o.CbEnable
	// CHWBL.
	s.ChwblPrefixHashLevel = o.ChwblPrefixHashLevel
	s.ChwblPrefixHashFlags = o.ChwblPrefixHashFlags
	s.ChwblMeanLoadFactor = o.ChwblMeanLoadFactor
	s.ChwblReplication = o.ChwblReplication
	s.ChwblEnableCacheSalt = o.ChwblEnableCacheSalt
	// P/D.
	s.PdDisaggMode = o.PdDisaggMode
	s.PdCacheAwareMode = o.PdCacheAwareMode
	s.PdSessionTtlSec = o.PdSessionTtlSec
	s.PdCacheThreshold = o.PdCacheThreshold
	s.PdBalanceAbsThreshold = o.PdBalanceAbsThreshold
	s.PdBootstrapPort = o.PdBootstrapPort
	// KV.
	s.KvExactMode = o.KvExactMode
	s.KvBlockSize = o.KvBlockSize
	s.KvHashAlgo = o.KvHashAlgo
	s.KvZmqPort = o.KvZmqPort
	s.KvWarmupSec = o.KvWarmupSec
	s.KvEngineType = o.KvEngineType
	s.KvDpRankCount = o.KvDpRankCount
	// HSTS.
	s.HstsMaxAge = o.HstsMaxAge
	s.HstsIncludeSubdomains = o.HstsIncludeSubdomains
	s.HstsPreload = o.HstsPreload
	// mTLS.
	if lbMtlsFrontendRequested(o) {
		s.MtlsFrontend = &api.MtlsFrontend{
			ClientCertMode:  o.MtlsClientCertMode,
			ClientCAPath:    o.MtlsClientCAPath,
			RequireClientCN: o.MtlsRequireClientCN,
			ClientCNPattern: o.MtlsClientCNPattern,
			ClientCRLPath:   o.MtlsClientCRLPath,
		}
	}
	if lbMtlsBackendRequested(o) {
		s.MtlsBackend = &api.MtlsBackend{
			VerifyServerCert: o.MtlsBackendVerifyServer,
			BackendCAPath:    o.MtlsBackendCAPath,
			ClientCertPath:   o.MtlsBackendClientCertPath,
			ClientKeyPath:    o.MtlsBackendClientKeyPath,
		}
	}
}

func LoadbalancerAPICall(restOptions *api.RESTOptions, lbModel api.LoadBalancerModel) (*http.Response, error) {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}

	return client.LoadBalancer().Create(ctx, lbModel)
}
