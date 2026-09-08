# loxicmd public machine contracts

This directory freezes the machine-readable contracts that automation may
depend on across releases:

| File | Contract |
|---|---|
| `command-result.schema.json` | The single JSON envelope every command emits under `-o json` / `--output json` |
| `exit-codes.md` | The process exit-code taxonomy and the error-origin preservation rules |
| `host-backend-contract.md` | How the `appliance` command family invokes the host lifecycle backend |
| `backend-contract.schema.json` | The response document of the backend's `contract-version --json` handshake |

## Stability rules

- Key names, value types, and enum members in a published schema are frozen.
  New optional keys and new enum members may be added; nothing is renamed,
  retyped, or removed within a major `apiVersion`.
- Exit-code numbers are frozen and are never renumbered or reused.
- Envelope keys are always present. A key that has nothing to say carries its
  empty value (`""`, `{}`, `[]`); `null` is never emitted.
- Timestamps are RFC 3339 in UTC with a trailing `Z`, second precision or
  finer (e.g. `2026-09-07T12:00:00Z`).
- Secret material (passwords, tokens, API keys, private keys) is prohibited
  from every contract document. Secret-bearing keys are omitted entirely —
  never emitted with a masked or placeholder value, because a masked value
  still reveals the secret's existence and invites parsers to depend on the
  key.

## Consuming the contracts

Automation must parse the JSON envelope and the exit code, never the human
text. Human-readable output (the default, without `-o json`) is not a
contract and may change between releases.
