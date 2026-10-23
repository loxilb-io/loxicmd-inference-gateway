# loxicmd exit-code taxonomy

Every `loxicmd` command, in every command family, terminates with exactly one
of the codes below. The numbers are frozen: they are never renumbered or
reused, and new meanings get new numbers.

| Exit | Envelope `code` | Meaning | Retry guidance |
|---:|---|---|---|
| `0` | `OK` | Success | Not required |
| `2` | `INVALID_ARGUMENT` | Invalid command, argument, or flag combination | Correct the input; do not retry unchanged |
| `3` | `AUTH` | Authentication failed or OS privilege insufficient | Verify credential or role; do not retry unchanged |
| `4` | `PRECONDITION` | A required precondition is not met (state, readiness, missing file) | Check state/readiness, then retry |
| `5` | `UNAVAILABLE` | Gateway, OAM, or host backend unreachable or refusing service | May retry with bounded backoff |
| `6` | `CONTRACT_MISMATCH` | Schema, version, or validation mismatch between this CLI and its peer | Use a compatible release or input; do not retry unchanged |
| `7` | `FAILED` | Operation failed and no state change is confirmed | Correct the cause, then retry |
| `8` | `PARTIAL` | Partial apply or recovery required | Do NOT retry automatically; recover using the operation ID |
| `9` | `FEATURE_UNAVAILABLE` | The command needs a gateway feature this peer does not offer; nothing was sent or changed | Upgrade the gateway, or use a build that offers the feature; do not retry unchanged |

Reserved values:

- `1` is not emitted by a conforming release. It is reserved as the
  unclassified legacy failure code so that automation encountering `1` can
  detect it is talking to a pre-taxonomy binary.
- `126`/`127` and signal-death codes (`128+n`) keep their shell semantics and
  are not part of this contract.

## `9` `FEATURE_UNAVAILABLE`

Commands that use a gateway feature (the priority bands and the default
reservation on a load balancer, the objectives on keys and tenants, the
`objective` and `netidentity` resources) read `GET /version` once per
invocation before they send anything. The feature keys are `bands`,
`objectives`, `netidentity` and `reservation`. The code is emitted on three
triggers:

1. **Not a gateway.** `VersionGetEntry.product` is not
   `loxilb-inference-gateway` (plain loxilb, or a gateway build that
   predates the field). The message is
   `this gateway does not advertise <feature>; upgrade the gateway or use a build that does`
   and the request is not sent.
2. **Path not served.** The gateway answers `404` for the feature's own path
   (`/config/ai/objective`, `/config/ai/netidentity`) rather than for a
   named item on it: the gateway is one that predates the resource.
3. **Reserved: token absent.** A gateway that advertises feature tokens in
   `VersionGetEntry.features` but not the one the command needs. Gateways up
   to loxilb-inference-gateway 7a58e59 advertise only `api_key_auth` and
   `jwt_auth_profile`, none of the four keys above, so this trigger is not
   enforced yet: when `product` is the gateway and the token is absent, the
   request is sent and one line goes to stderr —
   `warning: gateway does not advertise <feature>; sending anyway, the gateway's answer decides`
   — and the exit code follows the gateway's answer. The refusal is turned on
   once gateways advertise the keys.

## Rules

1. **The exit code and the JSON envelope always agree.** `success` is true
   exactly when the exit code is `0`, and the envelope `code` maps
   one-to-one to the exit code (table above).
2. **`--help` exits `0`.** Help shown because required arguments were missing
   is an invalid invocation and exits `2` (help text goes to stderr in that
   case).
3. **Origin is preserved, never flattened into prose.** A failure caused by a
   downstream component carries `data.origin`, `data.httpStatus`, and
   `data.componentCode` in the envelope, verbatim. Automation must branch on
   these fields and on the exit code — never by parsing `message` or human
   output.
4. **Retry semantics are part of the contract.** Orchestration may retry `5`
   (bounded backoff) and `4`/`7` (after addressing the cause). It must never
   auto-retry `8`: a partial apply retried blindly can compound damage;
   recovery goes through the returned operation ID.
5. **Ambiguity resolves downward to safety.** If an operation's outcome is
   unknown (timeout mid-mutation, peer died), the CLI reports `8` when a
   state change may have occurred and `5`/`7` only when it is confirmed that
   no state changed. Unknown is never reported as success.
6. **One classification per failure.** When several codes could apply, the
   most specific wins; `7` is the fallback for a classified failure with no
   better category, not a catch-all for laziness — the origin fields still
   say what actually happened.
