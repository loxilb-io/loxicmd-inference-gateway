# Host backend invocation contract (`loxicmd appliance`)

The `appliance` command family is a thin dispatcher: it validates arguments,
performs the version handshake below, invokes the host lifecycle backend, and
maps the result into the public envelope and exit-code contracts. It
implements no lifecycle logic of its own and never bypasses the backend
through the gateway API.

## Backend location and execution

- The backend executable path is fixed at compile time:
  `/usr/libexec/loxilb-appliance/loxilb-appliance-backend`. It is not
  discoverable or overridable through `PATH`, the environment, the working
  directory, or any flag. (Test builds may relocate it via build-time
  variables only.)
- Invocation is direct argv execution (`execve` semantics, Go
  `exec.CommandContext`). No shell, no `sh -c`, no string concatenation into
  a command line.
- Each subcommand carries an explicit argv allowlist; arguments outside it
  are rejected by the CLI before any process is started.
- The child environment is fixed by the CLI (empty but for a pinned minimal
  set); the caller's environment is not forwarded. An ordinary user cannot
  redirect the backend path, product root, or config/state directories
  through environment variables.

## Version and capability handshake

Before any mutating subcommand, the CLI runs:

```
loxilb-appliance-backend contract-version --json
```

and validates the response against `backend-contract.schema.json`. The rules:

- If the backend is absent or the handshake fails to execute: exit `5`
  (`UNAVAILABLE`, `data.origin` = `backend`, `data.componentCode` =
  `BACKEND_UNAVAILABLE`). No host state is changed and the CLI does not fall
  back to implementing the function itself.
- If the backend's `apiVersion` major differs from the CLI's, or the
  required command/capability is not advertised: exit `6`
  (`CONTRACT_MISMATCH`). No host state is changed.
- Read-only subcommands may proceed on a failed handshake only for
  best-effort diagnosis and must mark the result degraded.

## Streams, secrets, and correlation

- stdin is used only for an explicit secret file descriptor or declared
  streaming input. Secrets never travel through argv, the environment, or
  any JSON document (request or response).
- Backend stdout is either human output or exactly one JSON document; stderr
  is reserved for warnings and errors. The CLI preserves backend stdout,
  stderr, and exit status without loss, applying secret-redaction rules only.
- The CLI generates a correlation ID per invocation, passes it to the
  backend as `--correlation-id`, and reports it in the envelope. The same ID
  must be searchable in the journal/audit trail on the host.

## Result mapping

- Backend exit codes and error origins map onto the public exit-code
  taxonomy (`exit-codes.md`) without loss: the backend's own code is
  preserved verbatim in `data.componentCode` with `data.origin` = `backend`.
- If the backend emits JSON that violates its schema, or dies mid-operation,
  the CLI reports a failure (`6` or `8` per the taxonomy's ambiguity rule) —
  it never converts a contract violation or an unknown outcome into success.
- A backend outcome that indicates partial application yields exit `8` with
  the backend's operation ID in `data.operationId`.

## Availability rules

- `appliance` subcommands must work while the gateway container is stopped
  or unhealthy; the dispatcher never requires a gateway API connection for a
  backend-only command.
- Functions the installed backend does not advertise are reported as
  unavailable in help/capabilities and fail with `6` if invoked; they are
  never implemented as successful stubs.
