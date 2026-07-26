# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial CLI, derived from [loxicmd](https://github.com/loxilb-io/loxicmd) and retargeted as the command-line
  interface for [loxilb-inference-gateway](https://github.com/loxilb-io/loxilb-inference-gateway).
- Full classic loxilb management surface (load balancer, endpoints, sessions, policy, VLAN/VXLAN,
  firewall, BGP, BFD, mirroring, routes, neighbors, IP addresses, conntrack, status).
- Project scaffolding: CI (build/vet/gofmt), leak-scan hygiene gate, issue/PR templates.

### Planned
- Inference-gateway command coverage: model-name routing, SSE controls, CHWBL prefix hashing,
  prefill/decode disaggregation, KV-cache-aware routing, per-tenant API keys and rate limits, KV
  inventory inspection.
- REST-client enhancements: bearer-token auth and configurable TLS (`--insecure`/`--cacert`/`--cert`/`--key`).
