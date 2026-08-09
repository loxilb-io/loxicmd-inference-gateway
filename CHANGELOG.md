# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Versioning

loxicmd-inference-gateway versions in lockstep with
[loxilb-inference-gateway](https://github.com/loxilb-io/loxilb-inference-gateway)
and uses the same `vMAJOR.MINOR.PATCH[.BUILD]` scheme (e.g. `v0.9.8.6`):
CLI `vX` ships against gateway `vX`. A release build takes the version from the
git tag; a local `make build` stamps the Makefile's `VERSION` (see
[Makefile](Makefile)) plus date/branch/commit build info, both reported by
`loxicmd version`.

## [Unreleased]

### Added
- Initial CLI, derived from [loxicmd](https://github.com/loxilb-io/loxicmd) and retargeted as the command-line
  interface for [loxilb-inference-gateway](https://github.com/loxilb-io/loxilb-inference-gateway).
- Full classic loxilb management surface (load balancer, endpoints, sessions, policy, VLAN/VXLAN,
  firewall, BGP, BFD, mirroring, routes, neighbors, IP addresses, conntrack, status).
- REST-client foundation: bearer-token auth (`--bearer`) and configurable TLS
  (`--insecure`/`--cacert`/`--cert`/`--key`), plus a dual `Error`/`SimpleError` decoder for raw-middleware
  endpoints.
- Inference-gateway load-balancer flags: model-name routing, SSE controls, CHWBL prefix hashing,
  prefill/decode disaggregation, KV-cache-aware routing (vLLM & SGLang), and mTLS.
- AI-native resources: per-tenant API keys (`apikey`, incl. PATCH), tenant rate limits (`ratelimit`),
  KV inventory (`kvinventory`), user/auth flow (`create user`, `set login/logout/refreshtoken`).
- TLS/ops: certificate store (`cert`), SNI mappings (`sni`), Prometheus metrics toggle (`metrics`), HA state.
- Guardrails & telemetry: GPU-aware LB (`gpu`), PII detection (`pii`), LlamaFirewall (`llamafirewall`),
  HTTP/L4 request tracing (`trace`/`l4trace`), OPA policy watcher (`opa`), DPU offload debug (`dpu`).
- Configuration lifecycle: snapshot download (`get snapshot`), restore (`create restore`, dry-run by
  default), and persist-to-disk (`create persist`).
- Docs: [command reference](docs/COMMANDS.md) and [quickstart](docs/QUICKSTART.md).
- Project scaffolding: CI (build/vet/gofmt/tests, golangci-lint, CodeQL, govulncheck, gitleaks,
  release-hygiene gate), tag-triggered release workflow, and issue/PR templates.
