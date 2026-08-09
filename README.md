# loxicmd-inference-gateway

![build workflow](https://github.com/loxilb-io/loxicmd-inference-gateway/actions/workflows/build.yml/badge.svg)
![Go Version](https://img.shields.io/github/go-mod/go-version/loxilb-io/loxicmd-inference-gateway)
![License](https://img.shields.io/github/license/loxilb-io/loxicmd-inference-gateway)

## Overview

**loxicmd-inference-gateway** is the command-line interface (CLI) for
[loxilb-inference-gateway](https://github.com/loxilb-io/loxilb-inference-gateway) — an inference-aware
L4/L7 load balancer for LLM serving fleets. It manages both the classic loxilb load-balancing surface and
the inference-gateway's AI-native features (model-name routing, SSE streaming, prefill/decode
disaggregation, KV-cache-aware routing, per-tenant API keys and rate limits, and more) from the terminal.

It is derived from [loxicmd](https://github.com/loxilb-io/loxicmd) and speaks the same
`/netlox/v1` REST API, extended for the inference gateway.

## Where it fits (scope & non-goals)

This project is only the **command-line client**. The data path, control plane, and
AI-routing logic live in
[loxilb-inference-gateway](https://github.com/loxilb-io/loxilb-inference-gateway); this CLI
configures and inspects them over the gateway's REST API. It does not proxy traffic, does
not implement routing itself, and does not replace `kubectl`/CRD-based management for
Kubernetes deployments — it is the operator's terminal companion for standalone and
debugging workflows. With no AI feature flags used, it behaves as a drop-in `loxicmd` for
classic loxilb.

## Status

The CLI covers the full classic loxilb surface plus the inference gateway's AI-native features. Every
command and flag traces to the gateway's `api/swagger.yml` / `api/swagger-extras.yml` and the runnable
scenarios in the gateway's
[`cicd/` directory](https://github.com/loxilb-io/loxilb-inference-gateway/tree/main/cicd). Where the gateway's control plane accepts configuration that its data plane does not yet enforce
(e.g. API-key / rate-limit enforcement), the affected commands say so in their help text.

See the [Command Reference](docs/COMMANDS.md) for every command, and the
[Quickstart](docs/QUICKSTART.md) for a model-routing walkthrough.

## Features

**Load Balancer Management** — create, delete, and inspect service-type load balancers across multiple
algorithms, protocols, and NAT modes (including `fullproxy`, required for AI features).

**Inference Gateway** — model-name routing, SSE streaming controls, CHWBL prefix hashing, prefill/decode
disaggregation, KV-cache-aware routing (vLLM & SGLang), per-tenant API keys and rate limits, KV inventory
inspection, and TLS/mTLS.

**Guardrails & Telemetry** — GPU-aware load balancing, PII detection (Presidio), LlamaFirewall AI-security
scanning, HTTP/L4 request tracing with OTLP export, OPA policy watcher, and DPU offload debug.

**Operations** — Prometheus metrics, HA state, and configuration lifecycle (snapshot / restore / persist).

**Networking & Monitoring** — port/interface dumps, connection tracking, neighbors and routes, QoS policy,
VLAN/VXLAN, firewall, BGP, BFD, mirroring, sessions, endpoints, and IP address management.

## Installation

### Prerequisites
- Go 1.25 or later
- Make utility
- Linux (the CLI uses Linux netlink APIs for local network inspection)

### Building from Source

```bash
git clone https://github.com/loxilb-io/loxicmd-inference-gateway.git
cd loxicmd-inference-gateway
go get .
make
./loxicmd version
```

The build produces a binary named `loxicmd`.

## Quick Start

```bash
# List all load balancers
./loxicmd get lb

# Create a load balancer
./loxicmd create lb 192.0.2.200 --tcp=80:32015 --endpoints=203.0.113.1:1,203.0.113.2:1

# Delete a load balancer
./loxicmd delete lb 192.0.2.100 --tcp=80

# Output formats for automation or readability
./loxicmd get lb -o json
./loxicmd get lb -o wide
```

### Inference Gateway Examples

All AI features require `--mode fullproxy`.

```bash
# Model-name routing
./loxicmd create lb 192.0.2.10 --tcp=2020:8000 --mode=fullproxy \
  --model-name=llama-70b --path-prefix=/ --path-match-mode=prefix \
  --endpoints=203.0.113.1:1,203.0.113.2:1

# SSE streaming with a wall-clock cap
./loxicmd create lb 192.0.2.11 --tcp=2020:8000 --mode=fullproxy \
  --sse-mode --max-stream-duration=120 --backend-keepalive-interval=30 \
  --endpoints=203.0.113.1:1

# KV-cache-aware routing (vLLM) with prefill/decode endpoints
./loxicmd create lb 192.0.2.12 --tcp=2020:80 --mode=fullproxy \
  --pd-disagg --kv-exact-mode=1 --kv-zmq-port=5557 --kv-hash-algo=sha256_cbor \
  --kv-warmup=20 --kv-block-size=16 \
  --endpoints=203.0.113.1:1,203.0.113.2:1 --ep-role=prefill,decode --nixl-port=9001,9002

# CHWBL prefix-hash routing
./loxicmd create lb 192.0.2.13 --tcp=2020:8000 --mode=fullproxy --select=chwbl \
  --chwbl-hash-level=2 --chwbl-load-factor=125 --endpoints=203.0.113.1:1,203.0.113.2:1
```

### API Keys, Rate Limits & KV Inventory

These endpoints require an authenticated session (gateway started with
`--userservice`). Obtain a token first, then manage AI resources.

> API keys and tenant rate limits are **control-plane CRUD** today; data-plane
> enforcement (401/403/429) is on the roadmap.

```bash
# Authenticate (stores the bearer token; set login prompts for the password)
./loxicmd create user --username=admin --password='<your-password>' --role=admin
./loxicmd set login

# Per-tenant API keys (raw key is shown only once, at creation)
./loxicmd create apikey --tenant-id=tenant-a --name=key-1 \
  --allowed-models=llama-70b,mistral-7b --rps=5 --burst=10 --tokens-per-min=1000
./loxicmd get apikey --tenant-id=tenant-a
./loxicmd set apikey <key-id> --allowed-models=mistral-7b     # PATCH
./loxicmd set apikey <key-id> --enabled=false
./loxicmd delete apikey <key-id>

# Per-tenant rate limit
./loxicmd set ratelimit --tenant-id=tenant-a --rps=50 --tokens-per-min=2000
./loxicmd get ratelimit tenant-a

# KV-cache block-hash inventory (read-only)
./loxicmd get kvinventory --service-id=3 --ep-idx=0
```

### TLS Certificates & Operations

```bash
# TLS certificate store
./loxicmd create cert --cert-id=web --cert-file=server.crt --key-file=server.key --chain-file=chain.pem
./loxicmd get cert web            # private key is never returned
./loxicmd delete cert web

# SNI certificate mappings (hostname -> cert directory)
./loxicmd create sni --hostname=api.example.com --cert-path=/opt/loxilb/cert
./loxicmd get sni
./loxicmd delete sni --hostname=api.example.com

# Prometheus metrics toggle + HA state
./loxicmd set metrics --enable
./loxicmd get metrics
./loxicmd get hastate
```

### Guardrails, Tracing & Config Lifecycle

```bash
# GPU-aware LB, PII detection, LlamaFirewall (enable → inspect → configure)
./loxicmd set gpu --enable                 && ./loxicmd get gpu
./loxicmd set pii --enable                 && ./loxicmd get pii --stats
./loxicmd set llamafirewall --scanners --prompt-guard --code-shield
./loxicmd get llamafirewall

# Request tracing (HTTP + L4) with OTLP export
./loxicmd set trace --otlp --otlp-endpoint=jaeger.example.com:4317 --otlp-protocol=grpc
./loxicmd set l4trace --enable --sampling-rate=100 && ./loxicmd get l4trace

# OPA policy watcher (raw middleware) and DPU offload debug
./loxicmd set opa --opa-url=http://opa.example.com:8181 && ./loxicmd get opa
./loxicmd get dpu --hwcounters

# Configuration lifecycle: snapshot → dry-run restore → persist
./loxicmd get snapshot -f snapshot.json
./loxicmd create restore -f snapshot.json           # dry-run plan (add --commit to apply)
./loxicmd create persist
```

## Command Reference

The full, swagger-traceable reference lives in **[docs/COMMANDS.md](docs/COMMANDS.md)**.

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--apiserver` | `-s` | gateway API server address (default `127.0.0.1`) |
| `--port` | `-p` | gateway API server port (default `11111`) |
| `--protocol` | | `http` or `https` |
| `--output` | `-o` | output format (`json`, `wide`, or table) |
| `--token` | | token for authenticated endpoints |
| `--bearer` | | send the token as `Authorization: Bearer <token>` (default `true`; disable for classic loxilb raw-token targets) |
| `--insecure` | `-k` | skip TLS certificate verification (https only) |
| `--cacert` | | CA certificate (PEM) to verify the server (https only) |
| `--cert` | | client certificate (PEM) for mutual TLS (https only) |
| `--key` | | client private key (PEM) for mutual TLS (https only) |
| `--timeout` | `-t` | request timeout in seconds |

Run `./loxicmd help` or `./loxicmd <command> --help` for full details.

## Community & Governance

Contributions are welcome via pull request. Every PR needs at least one approving review from a
maintainer and passing CI before it can merge.

- [CONTRIBUTING.md](CONTRIBUTING.md) — how to build, test, and submit changes (incl. DCO sign-off)
- [GOVERNANCE.md](GOVERNANCE.md) — project governance and decision-making
- [MAINTAINERS.md](MAINTAINERS.md) — current maintainers
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — community standards
- [SECURITY.md](SECURITY.md) — how to report security vulnerabilities (never as public issues)
- [ROADMAP.md](ROADMAP.md) — where the project is heading

## License

Apache License 2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).

## Related Projects

- [loxilb-inference-gateway](https://github.com/loxilb-io/loxilb-inference-gateway) — the inference gateway this CLI drives
- [loxilb](https://github.com/loxilb-io/loxilb) — the core eBPF load balancer
