# Sanitization & Fork-Hygiene Checklist

Run before every push. The automated portion is `hack/leak-scan.sh` (also enforced in CI via
`.github/workflows/leak-scan.yml`). This file is the human companion; it must **not** ship to the public
repo if it accumulates internal notes — keep it generic.

## Authorship & history
- [ ] Fresh, human-authored git history — no transferred base-repo history.
- [ ] No `Co-Authored-By`, AI trailers, or bot footers in any commit.
- [ ] Commit messages are conventional and contain no phase/plan/decision/task/bug IDs.

## Artifacts (must not be in the tree)
- [ ] No `CLAUDE.md` / `AGENTS.md` / `GEMINI.md`.
- [ ] No `.claude/` / `.serena/` / `.mcp.json` / `.codegraph/` / `.planning/` / `claudedocs/`.
- [ ] No internal plan (`LOXICMD-INFERENCE-GATEWAY-PLAN.md`).

## Content
- [ ] No absolute `/Users/...` paths.
- [ ] No customer names (e.g. "NAVER"); no private/testbed IPs, SSH key paths, or reserved host IDs.
- [ ] Example addresses use documentation ranges (`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`)
      or `127.0.0.1` — never real testbed IPs.
- [ ] No `ghcr.io/netlox-dev` images/tokens; no `docs.netlox.io` or other internal hosts.
- [ ] Legitimate product values retained where they are real API content (e.g. `anthropic` trace-type,
      `claude-3` as a model-name example).

## Build & module
- [ ] Module path is `github.com/loxilb-io/loxicmd-inference-gateway`; no bare `loxicmd/` import remains.
- [ ] `make` builds cleanly on Linux; `go vet ./...` and `gofmt -l .` are clean.
- [ ] Invoked binary name is `loxicmd` (per decision D2).

## Gate
- [ ] `hack/leak-scan.sh` exits 0.
