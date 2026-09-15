# loxicmd public machine contracts

This directory freezes the machine-readable contracts that automation may
depend on across releases:

| File | Contract |
|---|---|
| `command-result.schema.json` | The single JSON envelope every command emits under `-o json` / `--output json` |
| `exit-codes.md` | The process exit-code taxonomy and the error-origin preservation rules |
| `host-backend-contract.md` | How the `appliance` command family invokes the host lifecycle backend |
| `backend-contract.schema.json` | The response document of the backend's `contract-version --json` handshake |
| `backend-payloads/v1/*.schema.json` | The nine command-selected bare backend payloads consumed by the Appliance CLI |
| `backend-errors/v1/backend-operation-error.schema.json` | The command-selected structured error payload for backend exits 2–8 |

## Appliance backend payload validation

The Product team owns the payload meanings. During CLI-first development, the
CLI repository holds the CP-CLI-approved byte-for-byte consumer schemas,
fixtures, and digest manifest. Product packaging must later intake that exact
bundle; local schema acceptance is not evidence that an installed Product
backend emits it.

The Go consumer and Appliance dispatcher select a payload only by the exact tuple
`{contractMajor, schemaVersion, canonicalCommand}` and returns data only after
exact-one structural and command-specific semantic validation. Unsupported
tuples, nested `CommandResult` envelopes, second JSON documents, unknown or
null fields, and secret-bearing keys are rejected as a typed
`PayloadValidationError`. Only the copied bytes in `ValidatedPayload` may enter
the public envelope's `data.backend`; syntactically valid but unvalidated JSON
is never exposed.

When the backend process exits non-zero, it emits the common
`BackendOperationError` document instead of a success payload. The document
uses the same payload `schemaVersion`, must repeat the selected canonical
command exactly, and preserves the backend's exit, stable code, origin,
component code, retryability, and optional correlation/operation evidence.
Exit 8 additionally requires an operation ID and recovery guidance. The
dispatcher proves that the document's `exit` matches the observed process exit
and that any supplied correlation ID matches the invocation before composing a
public result. Valid structured exits `2` through `8` retain their exact public
exit, code, origin, component code, and operation evidence.

That error's local classification is always `CONTRACT_MISMATCH` (exit 6).
This package does not decide whether an already-started mutating operation has
an uncertain or partial outcome. The dispatcher attaches spawn, exit,
correlation, and operation evidence to the typed validation error. Its final
context-sensitive choice between exit `6` and the mutating ambiguity rule's
exit `8` remains a separate composition step.

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

### Status contract compatibility

CLI-WP05 extends the v1 `status` payload with optional consumer fields so an
already-published v1 producer remains valid. The canonical fixture includes
all of them: top-level `reasonCode`, `status` and `observedAt` for the `state`,
`dataplane`, and `management` planes, and
`publicAddressTls: {"configured": boolean}`. When present, verdicts use only
`READY`, `DEGRADED`, `NOT_READY`, or `UNKNOWN`, observations are RFC 3339 UTC
timestamps ending in `Z`, and the TLS object permits no certificate, key,
token, password, customer rule, or other material. The pre-existing required
fields, including plane `live`, `ready`, and `reasonCode`, remain unchanged.

## Consuming the contracts

Automation must parse the JSON envelope and the exit code, never the human
text. Human-readable output (the default, without `-o json`) is not a
contract and may change between releases.

## Migration: the interim lifecycle report

Before the envelope ships as the product contract, the configuration-lifecycle
commands (`get snapshot`, `create restore`, `create persist`, `save --api`,
`get/set maintenance`) emitted an interim document of their own under
`-o json`. That shape was never pinned in a ProductLock; in-house automation
written against it migrates as follows:

| Interim field | Envelope location |
|---|---|
| `command` | `command` (dotted form, e.g. `create.persist`) |
| `result` (`ok`/`error`) | `success` (boolean) |
| `reason` | `data.componentCode` (verbatim; the coarse verdict is `code`) |
| `message` | `message` |
| `http_status` | `data.httpStatus` |
| `contract` | `data.contract` |
| `notes` | `warnings` (each with a stable `code`) |
| `persist` / `restore` / `snapshot` / `maintenance` | same keys under `data` |

The exit codes of these commands moved from the interim `0/1` to the frozen
taxonomy at the same release, so a consumer branching on the exit status and
one branching on the envelope migrate together.
