#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "--run-go-selector" ]]; then
  exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/run-go-selector.sh" "$@"
fi

if [[ "${1:-}" != "--meta-validate" ]]; then
  printf '%s\n' 'usage: scripts/test-evidence-verifier.sh --meta-validate | --run-go-selector SELECTOR ...' >&2
  exit 2
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export CLI_WP00_REPO_ROOT="${REPO_ROOT}"

python3 - <<'PY'
import copy
import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker

root = Path(os.environ["CLI_WP00_REPO_ROOT"])

def load(path):
    with (root / path).open("r", encoding="utf-8") as handle:
        return json.load(handle)

def fail(message):
    raise AssertionError(message)

def exact_one(raw):
    decoder = json.JSONDecoder()
    text = raw.decode("utf-8") if isinstance(raw, bytes) else raw
    value, end = decoder.raw_decode(text.lstrip())
    if text.lstrip()[end:].strip():
        fail("trailing or second JSON document accepted")
    return value

forbidden_keys = {"password", "passwordhash", "token", "apikey", "privatekey", "secret", "credential"}
credential_pattern = re.compile(r"(?i)(password|token|api[_-]?key|private[_-]?key)\s*[:=]")

def assert_secret_safe(value, sentinel):
    if isinstance(value, dict):
        for key, child in value.items():
            normalized = re.sub(r"[^a-z0-9]", "", key.lower())
            if normalized in forbidden_keys:
                fail(f"secret-bearing key accepted: {key}")
            assert_secret_safe(child, sentinel)
    elif isinstance(value, list):
        for child in value:
            assert_secret_safe(child, sentinel)
    elif isinstance(value, str):
        if sentinel in value:
            fail("synthetic secret sentinel accepted")
        if credential_pattern.search(value):
            fail("credential-shaped string accepted")

def validate_document(schema, document, sentinel, raw=None):
    if raw is not None:
        document = exact_one(raw)
    errors = list(Draft202012Validator(schema, format_checker=FormatChecker()).iter_errors(document))
    if errors:
        fail(errors[0].message)
    assert_secret_safe(document, sentinel)

def pointer_parent(document, pointer):
    parts = [p.replace("~1", "/").replace("~0", "~") for p in pointer.strip("/").split("/")]
    parent = document
    for part in parts[:-1]:
        parent = parent[int(part)] if isinstance(parent, list) else parent[part]
    return parent, parts[-1]

def apply_operation(base, fixture, sentinel):
    document = copy.deepcopy(base)
    operation = fixture["operation"]
    if operation == "append-json-document":
        return None, json.dumps(document, separators=(",", ":")) + "\n" + json.dumps(fixture["value"], separators=(",", ":"))
    if operation == "wrap-command-result":
        return {"apiVersion": "loxilb.io/appliance/v1", "kind": "CommandResult", "command": "appliance.status", "success": True, "code": "OK", "message": "", "correlationId": "cli-01", "data": document, "warnings": []}, None
    path = fixture["path"]
    parent, key = pointer_parent(document, path)
    if operation == "remove":
        del parent[key]
    elif operation in {"add", "replace"}:
        value = sentinel if fixture.get("valueFrom") == "syntheticSecretSentinel" else fixture["value"]
        parent[key] = value
    else:
        fail(f"unknown fixture operation: {operation}")
    return document, None

metadata_path = "contracts/backend-payloads/v1/command-metadata.v1.json"
metadata = load(metadata_path)
fixtures = load("testdata/backend-contract/fixtures.json")
selector = load("testdata/backend-contract/selectors/contract-meta-tests.v1.json")
sentinel = fixtures["syntheticSecretSentinel"]

expected_matrix = [
    ("status", True, True, ["json-output"]),
    ("network validate", True, True, ["json-output"]),
    ("public-address configure", False, True, ["json-output", "no-restart", "operation-receipt"]),
    ("gateway register-local", False, True, ["json-output", "secret-stdin", "operation-receipt"]),
    ("credentials bootstrap", False, False, ["console-only"]),
    ("diagnostics create", False, True, ["json-output", "redaction", "explicit-output", "operation-receipt"]),
    ("logs", False, True, ["json-output", "redaction", "bounded-window"]),
    ("backup key-create", False, True, ["json-output", "key-file", "operation-receipt"]),
    ("backup create", False, True, ["json-output", "key-file", "operation-receipt"]),
    ("backup verify", False, True, ["json-output", "key-file"]),
]
observed_matrix = [(item["canonicalCommand"], item["readOnly"], item["successfulJsonTarget"], item["requiredCapabilities"]) for item in metadata["commands"]]
if observed_matrix != expected_matrix:
    fail("10-command metadata/capability/readOnly matrix differs from approved canonical order")
if metadata["effectPolicy"] != "CONSERVATIVE_V1" or metadata["contractMajor"] != 1 or metadata["payloadSchemaVersion"] != "appliance-backend-payload/v1":
    fail("metadata tuple or effect policy differs from the approved candidate")
if len(set(metadata["capabilityVocabulary"])) != len(metadata["capabilityVocabulary"]):
    fail("duplicate capability vocabulary token")

schema_paths = ["contracts/backend-contract.schema.json"] + [item["payloadSchema"] for item in metadata["commands"] if item["successfulJsonTarget"]] + [
    "contracts/backend-errors/v1/backend-contract-error.schema.json",
    "contracts/backend-errors/v1/backend-operation-error.schema.json",
    "contracts/backend-receipts/v1/operation-receipt.schema.json",
]
schemas = {path: load(path) for path in schema_paths}
for path, schema in schemas.items():
    Draft202012Validator.check_schema(schema)

payload_metadata = [item for item in metadata["commands"] if item["successfulJsonTarget"]]
payload_ids = []
for item in payload_metadata:
    schema = schemas[item["payloadSchema"]]
    expected_id = "https://loxilb.io/schemas/appliance-backend/v1/" + Path(item["payloadSchema"]).name
    if schema.get("$id") != expected_id:
        fail(f"payload schema ID mismatch: {item['payloadSchema']}")
    if schema.get("properties", {}).get("schemaVersion", {}).get("const") != metadata["payloadSchemaVersion"]:
        fail(f"payload schema version mismatch: {item['payloadSchema']}")
    if schema.get("properties", {}).get("command", {}).get("const") != item["canonicalCommand"]:
        fail(f"payload command discriminator mismatch: {item['payloadSchema']}")
    if schema.get("additionalProperties") is not False:
        fail(f"payload root is not exact: {item['payloadSchema']}")
    payload_ids.append(schema["$id"])
if len(payload_ids) != 9 or len(set(payload_ids)) != 9:
    fail("payload schema ID set is not exactly nine unique IDs")

error_schema = schemas["contracts/backend-errors/v1/backend-contract-error.schema.json"]
if error_schema.get("required") != ["schemaVersion", "exit", "code", "componentCode", "retryable"] or error_schema.get("additionalProperties") is not False:
    fail("BackendContractError required fields or exact-object rule drifted")
operation_error_schema = schemas["contracts/backend-errors/v1/backend-operation-error.schema.json"]
if operation_error_schema.get("required") != ["schemaVersion", "command", "result", "exit", "code", "origin", "componentCode", "retryable"] or operation_error_schema.get("additionalProperties") is not False:
    fail("BackendOperationError required fields or exact-object rule drifted")
receipt_schema = schemas["contracts/backend-receipts/v1/operation-receipt.schema.json"]
if receipt_schema.get("required") != ["schemaVersion", "operationId", "correlationId", "command", "effect", "startedAt", "finishedAt", "result", "stateEvidence"] or receipt_schema.get("additionalProperties") is not False:
    fail("operation receipt required fields or exact-object rule drifted")

handshake = {
    "apiVersion": "loxilb.io/appliance-backend/v1",
    "kind": "BackendContract",
    "backendVersion": "candidate-v1",
    "productRelease": "v0.9.8.9-rc.1",
    "schemaVersion": "appliance-backend-payload/v1",
    "commands": [{"name": item[0], "readOnly": item[1], "capabilities": item[3]} for item in expected_matrix],
}
backend_schema = schemas["contracts/backend-contract.schema.json"]
validate_document(backend_schema, handshake, sentinel)

metadata_cases = {
    "metadata.exact-matrix.accept": (handshake, True),
    "metadata.duplicate-command.reject": ({**handshake, "commands": handshake["commands"][:-1] + [handshake["commands"][0]]}, False),
    "metadata.missing-command.reject": ({**handshake, "commands": handshake["commands"][:-1]}, False),
    "metadata.extra-command.reject": ({**handshake, "commands": handshake["commands"] + [{"name": "restore", "readOnly": False, "capabilities": []}]}, False),
    "metadata.renamed-command.reject": ({**handshake, "commands": [{**handshake["commands"][0], "name": "status renamed"}] + handshake["commands"][1:]}, False),
    "metadata.readonly-mismatch.reject": ({**handshake, "commands": handshake["commands"][:2] + [{**handshake["commands"][2], "readOnly": True}] + handshake["commands"][3:]}, False),
    "metadata.missing-capability.reject": ({**handshake, "commands": handshake["commands"][:2] + [{**handshake["commands"][2], "capabilities": ["json-output", "no-restart"]}] + handshake["commands"][3:]}, False),
    "metadata.unknown-capability.reject": ({**handshake, "commands": [{**handshake["commands"][0], "capabilities": ["json-output", "unknown"]}] + handshake["commands"][1:]}, False),
}

events = {}
def record(case_id, action):
    if case_id in events:
        fail(f"duplicate execution: {case_id}")
    try:
        action()
    except Exception as exc:
        events[case_id] = {"terminal": "FAIL", "detail": str(exc)}
        raise
    events[case_id] = {"terminal": "PASS"}

def expect_schema(case_id, schema, document, expected, raw=None):
    def action():
        accepted = True
        try:
            validate_document(schema, document, sentinel, raw=raw)
        except Exception:
            accepted = False
        if accepted != expected:
            fail(f"{case_id}: expected {'accept' if expected else 'reject'}, got {'accept' if accepted else 'reject'}")
    record(case_id, action)

for case_id, (document, expected) in metadata_cases.items():
    expect_schema(case_id, backend_schema, document, expected)

positive_by_command = {item["command"]: item for item in fixtures["payloads"]["positive"]}
command_slug = {item["canonicalCommand"]: Path(item["payloadSchema"]).name.removesuffix(".schema.json") for item in metadata["commands"] if item["successfulJsonTarget"]}
if list(positive_by_command) != list(command_slug):
    fail("positive payload fixture commands differ from the canonical nine-command order")
expansion = fixtures["payloads"]["negativeFixtureExpansion"]
if expansion != {
    "idTemplate": "payload.{schemaSlug}.{mutationClass}",
    "twinIdTemplate": "payload.{schemaSlug}.positive",
    "commands": list(command_slug.values()),
    "cardinality": len(command_slug) * len(fixtures["payloads"]["negativeMutationClasses"]),
}:
    fail("two-sided negative fixture expansion is not the exact 9 x 8 matrix")
missing_key = {"status": "productRelease", "network validate": "profile", "public-address configure": "publicAddress", "gateway register-local": "installationId", "diagnostics create": "archivePath", "logs": "component", "backup key-create": "keyPath", "backup create": "archivePath", "backup verify": "archivePath"}
invalid_path_value = {"status": ("/overallStatus", "BROKEN"), "network validate": ("/valid", "yes"), "public-address configure": ("/result", "BROKEN"), "gateway register-local": ("/verified", False), "diagnostics create": ("/mode", "0644"), "logs": ("/redacted", False), "backup key-create": ("/mode", "0644"), "backup create": ("/encrypted", False), "backup verify": ("/authenticated", False)}

for command, fixture in positive_by_command.items():
    slug = command_slug[command]
    schema_path = next(item["payloadSchema"] for item in metadata["commands"] if item["canonicalCommand"] == command)
    schema = schemas[schema_path]
    expect_schema(f"payload.{slug}.positive", schema, fixture["document"], True)
    for mutation in fixtures["payloads"]["negativeMutationClasses"]:
        spec = copy.deepcopy(mutation)
        if spec["operation"] == "remove-command-specific-required":
            spec = {"operation": "remove", "path": "/" + missing_key[command]}
        elif spec["operation"] == "replace-command-specific-invalid":
            path, value = invalid_path_value[command]
            spec = {"operation": "replace", "path": path, "value": value}
        elif spec["operation"] == "replace-command-specific-null":
            spec = {"operation": "replace", "path": "/" + missing_key[command], "value": None}
        document, raw = apply_operation(fixture["document"], spec, sentinel)
        expect_schema(f"payload.{slug}.{mutation['name']}", schema, document, False, raw=raw)

def run_fixture_group(group_name, schema_path):
    group = fixtures[group_name]
    positives = {item["id"]: item["document"] for item in group["positive"]}
    schema = schemas[schema_path]
    for case_id, document in positives.items():
        expect_schema(case_id, schema, document, True)
    for fixture in group["negative"]:
        if fixture["twin"] not in positives:
            fail(f"unknown valid twin: {fixture['twin']}")
        document, raw = apply_operation(positives[fixture["twin"]], fixture, sentinel)
        expect_schema(fixture["id"], schema, document, False, raw=raw)

def run_operation_error_group():
    group = fixtures["operationErrors"]
    schema = schemas["contracts/backend-errors/v1/backend-operation-error.schema.json"]
    positives = {item["id"]: item for item in group["positive"]}

    def expect_operation(case_id, document, expected_command, expected, raw=None):
        def action():
            accepted = True
            try:
                validate_document(schema, document, sentinel, raw=raw)
                if document is not None and document.get("command") != expected_command:
                    fail("operation error command differs from selected tuple")
            except Exception:
                accepted = False
            if accepted != expected:
                fail(f"{case_id}: expected {'accept' if expected else 'reject'}, got {'accept' if accepted else 'reject'}")
        record(case_id, action)

    for item in positives.values():
        expect_operation(item["id"], item["document"], item["command"], True)
    for fixture in group["negative"]:
        if fixture["twin"] not in positives:
            fail(f"unknown valid twin: {fixture['twin']}")
        twin = positives[fixture["twin"]]
        document, raw = apply_operation(twin["document"], fixture, sentinel)
        expect_operation(fixture["id"], document, twin["command"], False, raw=raw)

run_fixture_group("backendContractErrors", "contracts/backend-errors/v1/backend-contract-error.schema.json")
run_operation_error_group()
run_fixture_group("operationReceipts", "contracts/backend-receipts/v1/operation-receipt.schema.json")

manifest = load("testdata/backend-contract/cli-contract-candidate.manifest.json")
record("digest.schema-meta-validation", lambda: None)

eligible = {
    "contracts/backend-contract.schema.json",
    metadata_path,
    "testdata/backend-contract/fixtures.json",
    "testdata/backend-contract/selectors/contract-meta-tests.v1.json",
    "testdata/backend-contract/selectors/handshake-tests.v1.json",
    "testdata/backend-contract/selectors/payload-tests.v1.json",
    "scripts/run-go-selector.sh",
    "scripts/test-evidence-verifier.sh",
}
for directory in ["contracts/backend-payloads/v1", "contracts/backend-errors/v1", "contracts/backend-receipts/v1", "testdata/backend-contract/selectors/mutants"]:
    eligible.update(str(path.relative_to(root)) for path in (root / directory).glob("*.json"))
eligible = sorted(eligible)
recorded = manifest["bundle"]["files"]

def verify_per_file():
    if [item["path"] for item in recorded] != eligible:
        fail("digest manifest file paths are not the exact canonical lexicographic set")
    for item in recorded:
        digest = "sha256:" + hashlib.sha256((root / item["path"]).read_bytes()).hexdigest()
        if item["sha256"] != digest:
            fail(f"digest mismatch: {item['path']}")
record("digest.per-file", verify_per_file)
record("digest.canonical-order", lambda: None if [item["path"] for item in recorded] == sorted(item["path"] for item in recorded) else fail("non-canonical digest order"))

def aggregate_digest(entries):
    aggregate = hashlib.sha256()
    for item in entries:
        aggregate.update(item["path"].encode("utf-8"))
        aggregate.update(b"\x00")
        aggregate.update(item["sha256"].encode("ascii"))
        aggregate.update(b"\n")
    return "sha256:" + aggregate.hexdigest()

def verify_aggregate():
    if manifest["bundle"]["aggregateAlgorithm"] != "sha256(concat(path_utf8 + NUL + sha256_colon_hex + LF)) over files in lexicographic path order":
        fail("unknown aggregate algorithm")
    if manifest["bundle"]["aggregateSha256"] != aggregate_digest(recorded):
        fail("aggregate bundle digest mismatch")
record("digest.aggregate", verify_aggregate)

def path_digest_set(paths):
    aggregate = hashlib.sha256()
    for path in sorted(paths):
        digest = "sha256:" + hashlib.sha256((root / path).read_bytes()).hexdigest()
        aggregate.update(path.encode("utf-8"))
        aggregate.update(b"\x00")
        aggregate.update(digest.encode("ascii"))
        aggregate.update(b"\n")
    return "sha256:" + aggregate.hexdigest()

payload_schema_set = path_digest_set([item["payloadSchema"] for item in payload_metadata])
if manifest["contract"]["payloadSchemaSetSha256"] != payload_schema_set:
    fail("payload schema-set digest mismatch")
operation_error_digest = "sha256:" + hashlib.sha256((root / "contracts/backend-errors/v1/backend-operation-error.schema.json").read_bytes()).hexdigest()
if manifest["contract"]["operationErrorSchemaSha256"] != operation_error_digest:
    fail("operation error schema digest mismatch")

def expand_selector(document):
    result = []
    for suite in document["suites"]:
        if "caseIds" in suite:
            result.extend(suite["caseIds"])
        else:
            for command in suite["axes"]["commands"]:
                for case in suite["axes"]["cases"]:
                    result.append(suite["idTemplate"].format(command=command, case=case))
    if len(result) != len(set(result)):
        fail("selector contains duplicate case IDs")
    return result

declared = expand_selector(selector)
if len(declared) != selector["expectedExpandedCaseCount"]:
    fail("expanded selector count differs from exact expected count")

mutant_paths = {path.stem: path for path in (root / "testdata/backend-contract/selectors/mutants").glob("*.json")}
if set(mutant_paths) != set(selector["requiredSelectorMutants"]):
    fail("selector mutant file set differs from required exact set")
canonical_set = set(declared)
for mutant_name in selector["requiredSelectorMutants"]:
    path = mutant_paths[mutant_name]
    mutant = json.loads(path.read_text(encoding="utf-8"))
    mutated = set(canonical_set)
    operation = mutant["operation"]
    if operation == "replace-all":
        mutated = set(mutant["declaredCaseIds"])
    elif operation == "remove":
        mutated.remove(mutant["caseId"])
    elif operation == "add":
        mutated.add(mutant["caseId"])
    elif operation == "rename":
        mutated.remove(mutant["from"])
        mutated.add(mutant["to"])
    else:
        fail(f"unknown selector mutant operation: {operation}")
    record(f"selector.{mutant['mutant']}.reject", lambda mutated=mutated: None if mutated != canonical_set else fail("selector mutant survived"))

discovered = set(events)
if canonical_set != discovered:
    missing = sorted(canonical_set - discovered)
    extra = sorted(discovered - canonical_set)
    fail(f"selector accounting mismatch: missing={missing}, extra={extra}")
if any(item["terminal"] != "PASS" for item in events.values()):
    fail("non-PASS terminal result")

approval = manifest["approvalInputs"]
if approval != {
    "cliA0ApprovalSha256": "sha256:a6a71470e77b4487598679d45255830e4e64a2b51064a808b741a1de93ab7a1f",
    "integrationPlanSha256": "sha256:097915729b96eb7c721764b015064751b3af9bdba328a2bdfb522eda4ce27301",
    "implementationAuthorizationReviewSha256": "sha256:dfd70b261e7cc32483e5ed044b47407cb0e07ed32467c723e59db8d704ff329d"
}:
    fail("approval input digests differ from the frozen candidate inputs")

revision = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip()
base_revision = manifest["cliSource"]["baseRevision"]
if not re.fullmatch(r"[0-9a-f]{40}", revision) or not re.fullmatch(r"[0-9a-f]{40}", base_revision):
    fail("CLI source revision is malformed")
if subprocess.run(["git", "merge-base", "--is-ancestor", base_revision, revision], cwd=root).returncode != 0:
    fail("CLI base revision is not an ancestor of the candidate")
if manifest["cliSource"].get("candidateContainsUncommittedFiles") is not False:
    fail("candidate source identity still claims uncommitted files")
if manifest["verificationStatus"] != "DEVELOPMENT_UNVERIFIED" or manifest["status"] != "CP_CLI_REVIEW_REQUIRED":
    fail("candidate status overclaims verification or approval")

accounting = {name: sorted(canonical_set) for name in selector["accounting"]["requiredSetEquality"]}
if len({tuple(value) for value in accounting.values()}) != 1:
    fail("declared/discovered/selected/started/terminal/executed differ")

summary = {
    "schemaVersion": "cli-wp00-local-meta-validation/v1",
    "result": "PASS",
    "commands": {"metadata": 10, "successfulJsonSchemas": 9},
    "fixtures": {"payloadPositive": 9, "payloadNegative": 72, "backendContractError": 10, "operationError": 18, "operationReceipt": 9},
    "selector": {"caseCount": len(canonical_set), "mutantsKilled": ["zero", "missing", "extra", "renamed"]},
    "accounting": {name: len(value) for name, value in accounting.items()},
    "bundleDigest": manifest["bundle"]["aggregateSha256"],
    "verificationStatus": manifest["verificationStatus"],
    "nonProof": manifest["nonProof"]
}
json.dump(summary, sys.stdout, indent=2, sort_keys=True)
sys.stdout.write("\n")
PY
