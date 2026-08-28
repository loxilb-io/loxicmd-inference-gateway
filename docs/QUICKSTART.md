# Quickstart — model-name routing

This walkthrough stands up **model-name routing**: the gateway inspects the
`X-Model` HTTP header (or the `"model"` field of an OpenAI-compatible JSON body)
and routes each request to the matching backend pool, falling back to a wildcard
pool when a rule sets an empty model name. It mirrors the gateway's
`ai-model-routing` test scenario, driven entirely from `loxicmd`.

Addresses use the documentation ranges `192.0.2.0/24` (the VIP) and
`203.0.113.0/24` (backends) — substitute your own.

## Prerequisites

- A running `loxilb-inference-gateway` reachable at `-s <host> -p <port>`
  (default `127.0.0.1:11111`).
- Three HTTP backends, one per model pool.
- `loxicmd` built (`make`) — see the [README](../README.md).

Point every command at your gateway with the global `-s/-p` flags, e.g.
`loxicmd -s 192.0.2.254 get lb`.

## 1. Create the model pools

Model routing is an L7 feature, so each rule uses `--mode=fullproxy`. Give each
model its own VIP port and backend pool; leave `--model-name` empty for the
wildcard catch-all.

```bash
# llama-70b  → backend .1
loxicmd create lb 192.0.2.10 --tcp=2020:8080 --mode=fullproxy \
  --model-name=llama-70b --path-prefix=/ --path-match-mode=prefix \
  --endpoints=203.0.113.1:1

# mistral-7b → backend .2
loxicmd create lb 192.0.2.10 --tcp=2021:8080 --mode=fullproxy \
  --model-name=mistral-7b --path-prefix=/ --path-match-mode=prefix \
  --endpoints=203.0.113.2:1

# wildcard (empty model name) → backend .3
loxicmd create lb 192.0.2.10 --tcp=2022:8080 --mode=fullproxy \
  --path-prefix=/ --path-match-mode=prefix \
  --endpoints=203.0.113.3:1
```

## 2. Verify the rules

```bash
loxicmd get lb -o json      # AI attributes (model_name, mode=4, path_*) appear here
```

You should see three services, with `model_name` set to `llama-70b`,
`mistral-7b`, and empty (wildcard) respectively, each with `mode: 4`.

To require a tenant API key on a model pool, create that service with
`--api-key-auth=required`. The policy is disabled by default and is independent
of management-plane bearer login:

```bash
loxicmd create lb 192.0.2.11 --tcp=2020:8080 --mode=fullproxy \
  --model-name=protected-model --path-prefix=/ --path-match-mode=prefix \
  --api-key-auth=required --endpoints=203.0.113.1:1
```

Create/import tenant keys and quotas as described in the
[AI-native resource reference](COMMANDS.md#ai-native-resources-api-keys-rate-limits-kv-inventory).

## 3. Exercise the routing

```bash
# Header selects the model pool
curl -s http://192.0.2.10:2020/ -H 'X-Model: llama-70b'      # → llama backend

# JSON body model field selects the pool
curl -s http://192.0.2.10:2021/ \
  -H 'Content-Type: application/json' \
  -d '{"model":"mistral-7b","messages":[]}'                  # → mistral backend

# No model → wildcard rule
curl -s http://192.0.2.10:2022/                              # → wildcard backend

# Unknown model → 503 model_unavailable
curl -s http://192.0.2.10:2020/ -H 'X-Model: does-not-exist'
```

The `X-Model` header takes precedence over the JSON body, and matching is
case-sensitive.

## 4. Snapshot the configuration

Capture the working setup so it can be restored or moved to another instance:

```bash
loxicmd get snapshot -f ai-routing-snapshot.json
loxicmd create persist        # also write it to the gateway's on-disk config
```

Restore it later (dry-run first, then commit):

```bash
loxicmd create restore -f ai-routing-snapshot.json            # prints the plan
loxicmd create restore -f ai-routing-snapshot.json --commit   # applies it
```

## 5. Clean up

L7/fullproxy rules delete by name; find each rule's name in `get lb -o json`.

```bash
loxicmd delete lb --name=<rule-name>
```

## Next steps

- SSE streaming, CHWBL, prefill/decode, and KV-cache routing:
  [Command Reference → load balancer](COMMANDS.md#load-balancer--inference-routing).
- Per-tenant API keys and rate limits:
  [Command Reference → AI-native resources](COMMANDS.md#ai-native-resources-api-keys-rate-limits-kv-inventory).
- Guardrails (PII, LlamaFirewall) and tracing:
  [Command Reference → guardrails & telemetry](COMMANDS.md#guardrails--telemetry).
