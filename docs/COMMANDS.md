# Command Reference

`loxicmd` is verb-first: every operation is `loxicmd <verb> <noun> [flags]`, where
the verbs are `get`, `create`, `delete`, `set`, plus the top-level `save`, `apply`,
`version`, and `completion`.

Every command, flag, and default below maps to a real endpoint in the
inference gateway's `api/swagger.yml` / `api/swagger-extras.yml`. Example
addresses use the documentation ranges `192.0.2.0/24` (VIPs) and
`203.0.113.0/24` (backends); replace them with your own.

- [Global flags](#global-flags)
- [Load balancer & inference routing](#load-balancer--inference-routing)
- [AI-native resources](#ai-native-resources-api-keys-rate-limits-kv-inventory)
- [Authentication](#authentication)
- [TLS certificates & SNI](#tls-certificates--sni)
- [Guardrails & telemetry](#guardrails--telemetry)
- [Policy engine & DPU debug](#policy-engine--dpu-debug)
- [Configuration lifecycle](#configuration-lifecycle)
- [Metrics, HA & classic networking](#metrics-ha--classic-networking)
- [Shell completion](#shell-completion)

Where the gateway's *control plane* accepts configuration its *data plane* does
not yet enforce, the affected commands say so in their `--help` and it is noted
below.

---

## Global flags

These persistent flags apply to every command.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--apiserver` | `-s` | `127.0.0.1` | Gateway API server address |
| `--port` | `-p` | `11111` | Gateway API server port |
| `--protocol` | | `http` | `http` or `https` |
| `--output` | `-o` | table | Output format: `json`, `wide`, or table |
| `--token` | | | Bearer token for authenticated endpoints (falls back to `/tmp/loxilbtoken`) |
| `--bearer` | | `true` | Send the token as `Authorization: Bearer <token>`; disable for classic loxilb raw-token targets |
| `--insecure` | `-k` | `false` | Skip TLS certificate verification (https only) |
| `--cacert` | | | CA certificate (PEM) to verify the server (https only) |
| `--cert` | | | Client certificate (PEM) for mutual TLS (https only) |
| `--key` | | | Client private key (PEM) for mutual TLS (https only) |
| `--timeout` | `-t` | | Request timeout in seconds |

---

## Load balancer & inference routing

`create lb` / `get lb` / `delete lb` — swagger `POST/GET/DELETE /config/loadbalancer`.

All AI features require **`--mode fullproxy`** (`mode=4`). Setting any AI flag
without it is rejected. AI attributes are shown in full with `get lb -o json`.

```bash
# Model-name routing  (cicd: ai-model-routing)
loxicmd create lb 192.0.2.10 --tcp=2020:8000 --mode=fullproxy \
  --model-name=llama-70b --path-prefix=/ --path-match-mode=prefix \
  --endpoints=203.0.113.1:1,203.0.113.2:1

# SSE streaming + quota  (cicd: ai-sse-quota)
loxicmd create lb 192.0.2.11 --tcp=2020:8000 --mode=fullproxy \
  --sse-mode --max-stream-duration=120 --backend-keepalive-interval=30 \
  --endpoints=203.0.113.1:1

# CHWBL prefix-hash routing  (cicd: vllm-fullproxy)
loxicmd create lb 192.0.2.12 --tcp=2020:8000 --mode=fullproxy --select=chwbl \
  --chwbl-hash-level=2 --chwbl-load-factor=125 --chwbl-replication=100 \
  --endpoints=203.0.113.1:1,203.0.113.2:1

# Prefill/decode disaggregation  (cicd: vllm-pd-disagg)
loxicmd create lb 192.0.2.13 --tcp=2020:80 --mode=fullproxy \
  --pd-disagg --pd-cache-aware \
  --endpoints=203.0.113.1:1,203.0.113.2:1 --ep-role=prefill,decode --nixl-port=9001,9002

# KV-cache-aware routing, vLLM  (cicd: vllm-kvcache-routing-cpu)
loxicmd create lb 192.0.2.14 --tcp=2020:80 --mode=fullproxy \
  --pd-disagg --kv-exact-mode=1 --kv-zmq-port=5557 --kv-hash-algo=sha256_cbor \
  --kv-warmup=20 --kv-block-size=16 \
  --endpoints=203.0.113.1:1,203.0.113.2:1 --ep-role=prefill,decode

# KV-cache-aware routing, SGLang  (cicd: sglang-loxilb-kvcache)
loxicmd create lb 192.0.2.15 --tcp=2020:80 --mode=fullproxy \
  --kv-exact-mode=3 --kv-engine-type=sglang --kv-dp-ranks=2 \
  --endpoints=203.0.113.1:1

# get / delete
loxicmd get lb -o json
loxicmd delete lb --name=<rule-name>     # L7/fullproxy rules delete by --name
```

### AI flag groups (on `create lb`)

| Group | Flags |
|-------|-------|
| Mode / algo | `--mode fullproxy`, `--select chwbl\|chwbl-wrr\|gpuaware\|persist`, `--security plain\|https\|e2ehttps` |
| Model routing | `--model-name`, `--path-prefix`, `--path-match-mode disabled\|prefix\|exact`, `--backend-protocol http1\|http2\|both`, `--session-header-name`, `--trace-type` |
| SSE | `--sse-mode`, `--max-stream-duration`, `--backend-keepalive-interval` |
| CHWBL | `--chwbl-hash-level 1\|2\|3`, `--chwbl-load-factor`, `--chwbl-replication` |
| P/D disagg | `--pd-disagg`, `--pd-cache-aware`, `--pd-session-ttl`, `--pd-cache-threshold`, `--pd-balance-abs-threshold`; per-endpoint `--ep-role prefill\|decode\|normal`, `--nixl-port` |
| KV-cache | `--kv-exact-mode 0\|1\|3`, `--kv-zmq-port`, `--kv-hash-algo sha256_cbor\|xxhash_cbor\|sha256_sglang`, `--kv-engine-type vllm\|sglang`, `--kv-dp-ranks`, `--kv-warmup`, `--kv-block-size` |
| mTLS | `--mtls-frontend`, `--mtls-backend` (bundle: client-cert-mode, ca-path, cert/key, verify-server-cert, require-client-cn/cn-pattern) |
| HSTS | `--hsts-max-age`, `--hsts-include-subdomains` |

Validation: AI flags ⇒ `--mode fullproxy`; `--pd-cache-aware` ⇒ `--pd-disagg`;
`--kv-engine-type` is immutable per VIP on the server.

---

## AI-native resources (API keys, rate limits, KV inventory)

Require the gateway started with `--userservice` + a DB backend and an
authenticated session (see [Authentication](#authentication)).

> **Control-plane only today:** API keys and tenant rate limits store
> configuration; data-plane enforcement (401/403/429) is on the gateway
> roadmap. SSE stream lifecycle & token bookkeeping *are* wired.

```bash
# API keys — swagger /config/ai/apikey (+ PATCH via extras)   (cicd: ai-apikey)
loxicmd create apikey --tenant-id=tenant-a --name=key-1 \
  --allowed-models=llama-70b,mistral-7b --rps=5 --burst=10 --tokens-per-min=1000
loxicmd get apikey --tenant-id=tenant-a          # list; raw_key/hash never shown
loxicmd get apikey <key-id>                       # one
loxicmd set apikey <key-id> --allowed-models=mistral-7b   # PATCH
loxicmd set apikey <key-id> --enabled=false
loxicmd delete apikey <key-id>

# Tenant rate limit — swagger /config/ai/tenant/ratelimit   (cicd: ai-apikey)
loxicmd set ratelimit --tenant-id=tenant-a --rps=50 --tokens-per-min=2000
loxicmd get ratelimit tenant-a

# KV-cache block-hash inventory (read-only) — extras /config/ai/kv/inventory
loxicmd get kvinventory --service-id=3 --ep-idx=0
```

The `raw_key` is printed **once**, at creation — store it immediately.

---

## Authentication

```bash
loxicmd create user --username=admin --password='Admin123!' --role=admin  # /auth/users
loxicmd set login          # POST /auth/login; stores a Bearer token
loxicmd set refreshtoken
loxicmd set logout
```

---

## TLS certificates & SNI

```bash
# Certificate store — swagger /config/cert (no list endpoint; key never returned)
loxicmd create cert --cert-id=web --cert-file=server.crt --key-file=server.key --chain-file=chain.pem
loxicmd get cert web
loxicmd delete cert web

# SNI hostname → cert directory — swagger /sni/certificates   (cicd: e2ehttpsproxy-mtls)
loxicmd create sni --hostname=api.example.com --cert-path=/opt/loxilb/cert
loxicmd get sni
loxicmd delete sni --hostname=api.example.com     # DELETE carries the hostname in the body
```

The `--cert-file`/`--key-file` flags on `create cert` are distinct from the
global `--cert`/`--key` TLS client flags.

---

## Guardrails & telemetry

Enable/configure/inspect toggles. `get` renders the raw JSON status/stats
document. Only the `--configure`/`--scanners` flags you set are sent, so an
unset flag never overwrites a server-side default.

### GPU-aware load balancing — swagger `/config/gpu/*`, `/config/worker/metrics`

```bash
loxicmd set gpu --enable
loxicmd set gpu --disable
loxicmd set gpu --cleanup --max-age-hours=2      # prune stale conversation mappings
loxicmd get gpu                                   # status
loxicmd get gpu --workers                         # per-worker GPU metrics
```

### PII detection (Presidio) — swagger `/config/pii/*`

```bash
loxicmd set pii --enable
loxicmd set pii --configure --mode=mask --direction=both --score-threshold=0.7 \
  --analyzer-url=localhost:50051
loxicmd set pii --url-patterns --url-mode=replace --include=/v1/chat/* --exclude=/health
loxicmd get pii            # status
loxicmd get pii --stats    # scan/detection/block counters
```

`--mode`: detect·mask·redact·anonymize · `--direction`: both·request·response ·
`--fail-mode`: open·closed · `--scan-mode`: full·truncate. v2 flags:
`--enable-v2`, `--default-operator`, `--encryption-key`, `--batch-size`.

### LlamaFirewall AI-security — swagger `/config/llamafirewall/*`

```bash
loxicmd set llamafirewall --enable
loxicmd set llamafirewall --configure --server-url=localhost:50052 --block-threshold=0.9 \
  --fail-closed=false --cache-enabled=true
loxicmd set llamafirewall --scanners --prompt-guard --code-shield --regex
loxicmd set llamafirewall --health        # probe the gRPC server
loxicmd get llamafirewall                 # status + enabled scanners
loxicmd get llamafirewall --stats         # per-scanner + decision stats
```

Scanners: `--prompt-guard`, `--code-shield`, `--regex`, `--hidden-ascii`,
`--agent-alignment`, `--pii-detection`.

### HTTP/HTTPS request tracing — swagger `/config/trace/*`

```bash
loxicmd set trace --enable
loxicmd set trace --disable
loxicmd set trace --otlp --otlp-endpoint=jaeger.example.com:4317 --otlp-protocol=grpc \
  --otlp-use-tls --otlp-header=authorization=Bearer\ xyz
loxicmd get trace            # status (events, ring utilization, OTLP connectivity)
loxicmd get trace --otlp     # current OTLP exporter config
loxicmd get trace --parsers  # available trace parsers
```

### L4 connection tracing — swagger `/config/l4trace/*`

```bash
loxicmd set l4trace --enable --sampling-rate=100
loxicmd set l4trace --sampling-rate=10        # update rate while running (PUT)
loxicmd set l4trace --reset-stats
loxicmd set l4trace --disable
loxicmd get l4trace                            # status, sampling rate, event stats
```

---

## Policy engine & DPU debug

Raw-middleware endpoints (see `api/swagger-extras.yml`); their error bodies use
the `SimpleError` `{"error": "..."}` envelope.

### OPA L4 policy watcher — extras `/config/opa/watcher`

```bash
loxicmd set opa --opa-url=http://opa.example.com:8181 --policy-path=loxilb/l4 \
  --poll-interval-sec=30 [--fail-open]
loxicmd get opa       # status: not_configured | running | stopped
loxicmd delete opa    # stop and remove the watcher
```

URLs resolving to private/reserved ranges are rejected server-side (SSRF
protection).

### DPU offload debug — extras `/config/dpu/*`

```bash
loxicmd get dpu                                   # offload state + aggregate/per-pipe counters
loxicmd get dpu --flows                           # include per-flow/FDB/route/ACL arrays
loxicmd get dpu --pipe=ct_fwd_5tuple --limit=100  # filtered per-entry detail
loxicmd get dpu --hwcounters                       # per-flow hardware counters
loxicmd set dpu --action=unregister --plugin=doca # unload a DPU plugin
loxicmd set dpu --action=cb_force --mode=open      # pin the offload circuit breaker
```

---

## Configuration lifecycle

Modern snapshot/restore/persist — swagger `/config/snapshot`, `/config/restore`,
`/config/persist` (supersede the legacy `/config/export` and `/config/import`).

```bash
# Download a versioned, checksummed snapshot of all v1 config domains
loxicmd get snapshot -f snapshot.json
loxicmd get snapshot --components=loadbalancer,endpoint     # subset

# Restore a snapshot. Default is dry-run (validate + plan, no mutation).
loxicmd create restore -f snapshot.json            # dry-run: prints the plan
loxicmd create restore -f snapshot.json --commit   # apply, with rollback on failure

# Persist the running config to disk so it survives a daemon restart
loxicmd create persist
```

The classic client-side dump/apply also remains available:

```bash
loxicmd save        # write local config dumps
loxicmd apply -f <file>
```

---

## Metrics, HA & classic networking

```bash
loxicmd set metrics --enable        # Prometheus toggle — /config/metrics
loxicmd get metrics
loxicmd get hastate                 # cluster HA state — /config/cistate/all
loxicmd set log-level <level>       # operational params — /config/params
loxicmd get log-level
```

The inherited classic loxilb surface — `port`, `conntrack`, `session`,
`sessionulcl`, `policy`, `route`, `ipaddress`, `neighbor`, `fdb`, `vlan`,
`vxlan`, `firewall`, `mirror`, `bgp`, `bfd`, `endpoint`, `status` — is available
under the same verbs. Run `loxicmd <verb> --help` for the full list.

---

## Shell completion

```bash
loxicmd completion bash > /etc/bash_completion.d/loxicmd    # or zsh|fish|powershell
source /etc/bash_completion.d/loxicmd
```

`make install` installs the binary and bash completion in one step.
