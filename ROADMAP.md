# Roadmap

This roadmap captures the direction of loxicmd-inference-gateway. It is
intentionally short and honest: items here are intent, not commitments, and
priorities shift with community feedback. The CLI's scope is defined in the
[README](README.md#where-it-fits-scope--non-goals); it tracks the feature
surface of
[loxilb-inference-gateway](https://github.com/loxilb-io/loxilb-inference-gateway),
so gateway-side roadmap items generally imply CLI follow-up here.

## Near term

- **Binary releases.** Publish versioned release artifacts (tarball +
  checksums) via a tag-triggered workflow, versioned in lockstep with the
  gateway (see [CHANGELOG.md](CHANGELOG.md)).
- **Automated contract drift checks.** Keep the consumed Swagger surface and
  engine-specific invariants synchronized as the gateway evolves, with a CI
  job that compares this repository's contract manifest against a selected
  gateway checkout.
- **Safer local token storage.** Store bearer tokens under the user's home
  directory instead of `/tmp`, keeping a fallback read for compatibility with
  classic loxicmd.

## Exploring

- **Shell-native UX improvements.** Richer `-o wide` views for AI resources,
  and command aliases matching upstream loxicmd where they diverge.
- **Scenario-linked docs tests.** CI that cross-checks `docs/COMMANDS.md`
  examples against the gateway's `cicd/` scenario payloads so documentation
  cannot drift from the real API.

## How to influence this roadmap

Open an issue describing your serving stack and the management workflow you
need. Real deployment feedback moves items from *Exploring* to *Near term*
faster than anything else.
