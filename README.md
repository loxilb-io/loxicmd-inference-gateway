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

## Status

AI command coverage is being built out in phases (see the project roadmap). Today the CLI provides the full
classic loxilb surface; AI-specific commands and load-balancer flags are landing incrementally. Where the
gateway's control plane accepts configuration that its data plane does not yet enforce (e.g. API-key /
rate-limit enforcement), the affected commands say so in their help text.

## Features

**Load Balancer Management** — create, delete, and inspect service-type load balancers across multiple
algorithms, protocols, and NAT modes (including `fullproxy`, required for AI features).

**Inference Gateway (in progress)** — model-name routing, SSE streaming controls, CHWBL prefix hashing,
prefill/decode disaggregation, KV-cache-aware routing, per-tenant API keys and rate limits, KV inventory
inspection.

**Networking & Monitoring** — port/interface dumps, connection tracking, neighbors and routes, QoS policy,
VLAN/VXLAN, firewall, BGP, BFD, mirroring, sessions, endpoints, and IP address management.

## Installation

### Prerequisites
- Go 1.23 or later
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

## Command Reference

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

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## Related Projects

- [loxilb-inference-gateway](https://github.com/loxilb-io/loxilb-inference-gateway) — the inference gateway this CLI drives
- [loxilb](https://github.com/loxilb-io/loxilb) — the core eBPF load balancer
