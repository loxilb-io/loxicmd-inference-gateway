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
  prefill/decode disaggregation, circuit breaking, per-service API-key enforcement, engine-aware routing
  for vLLM, SGLang, TensorRT-LLM, and llama.cpp, and mTLS.
- AI-native resources: generated or securely imported per-tenant API keys (`apikey`, incl. PATCH), tenant
  rate limits with burst and model quotas (`ratelimit`), KV inventory (`kvinventory`), and the user/auth
  flow (`create user`, `set login/logout/refreshtoken`).
- QoS policy targets with symbolic rule/port/egress attachments and bracketed IPv6 rule keys.
- TLS/ops: certificate store (`cert`), SNI mappings (`sni`), Prometheus metrics toggle (`metrics`), HA state.
- Guardrails & telemetry: GPU-aware LB (`gpu`), PII detection (`pii`), LlamaFirewall (`llamafirewall`),
  HTTP/L4 request tracing (`trace`/`l4trace`), OPA policy watcher (`opa`), DPU offload debug (`dpu`).
- Configuration lifecycle: snapshot download (`get snapshot`), restore (`create restore`, dry-run by
  default), and persist-to-disk (`create persist`).
- Docs: [command reference](docs/COMMANDS.md) and [quickstart](docs/QUICKSTART.md).
- Project scaffolding: CI (build/vet/gofmt/tests, golangci-lint, CodeQL, govulncheck, gitleaks,
  release-hygiene gate), tag-triggered release workflow, and issue/PR templates.
- Public machine contracts under [contracts/](contracts/): the `CommandResult`
  JSON envelope schema and the frozen exit-code taxonomy, with golden tests
  pinning every command family's success output.
- `--token-file`: read the API token from an owner-only (`0600`) regular file
  instead of passing it on the command line. The file follows the same
  secret-file rules as every other secret-bearing path (absolute path, no
  symlinks, owner-only permissions) and the value never appears in argv or
  the process environment.

### Deprecated
- `--token`: the literal token is visible in shell history and process
  listings. It keeps working through the deprecation window but now prints a
  one-line warning on stderr; use `--token-file` instead. A future release
  removes `--token` (announced here and in that release's notes before it
  happens). `--token` and `--token-file` are mutually exclusive.

### Changed
- **Exit codes**: failures now terminate with the frozen taxonomy
  (`2`–`8`, see [contracts/exit-codes.md](contracts/exit-codes.md)) instead of
  the legacy `0`/`1`. Success stays `0`; `1` is reserved so automation can
  detect a pre-taxonomy binary. Scripts that tested `$? -eq 1` must test
  `$? -ne 0` (or branch on the specific code); commands that previously
  printed an error but exited `0` now exit non-zero.
- **JSON output**: `-o json` emits the `CommandResult` envelope
  ([contracts/command-result.schema.json](contracts/command-result.schema.json))
  for every command, including the configuration-lifecycle family, which
  previously used an interim document of its own — the field-by-field
  migration table is in [contracts/README.md](contracts/README.md).
- Failure text prints exactly once, on stderr, from the single exit point;
  commands no longer print their own `Error:` lines.
