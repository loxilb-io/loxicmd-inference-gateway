#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export CLI_SELECTOR_REPO_ROOT="${REPO_ROOT}"

python3 - "$@" <<'PY'
import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("--run-go-selector", required=True)
parser.add_argument("--suite", required=True)
parser.add_argument("--package", required=True)
parser.add_argument("--count", type=int, default=1)
parser.add_argument("--json-events", required=True)
parser.add_argument("--require-selector-mutants", required=True)
args = parser.parse_args(sys.argv[1:])

root = Path(os.environ["CLI_SELECTOR_REPO_ROOT"])
selector_path = root / args.run_go_selector
selector = json.loads(selector_path.read_text(encoding="utf-8"))
if selector.get("schemaVersion") != "cli-go-test-selector/v1":
    raise SystemExit("unsupported selector schemaVersion")
suites = {suite["name"]: suite for suite in selector["suites"]}
if args.suite not in suites:
    raise SystemExit(f"unknown selector suite: {args.suite}")
expected = suites[args.suite]["tests"]
if not expected or len(expected) != len(set(expected)) or any(not re.fullmatch(r"Test[A-Za-z0-9_]+", name) for name in expected):
    raise SystemExit("selector expected set is zero, duplicate, or contains an invalid Go test name")
packages = [item for item in args.package.split(",") if item]
if packages != [selector["package"]]:
    raise SystemExit("requested package does not exactly match the selector")

pattern = "^(" + "|".join(re.escape(name) for name in expected) + ")$"
environment = os.environ.copy()
listed = subprocess.run(["go", "test", "-list", pattern, *packages], cwd=root, env=environment, text=True, capture_output=True)
if listed.returncode != 0:
    sys.stderr.write(listed.stdout + listed.stderr)
    raise SystemExit(listed.returncode)
discovered = [line.strip() for line in listed.stdout.splitlines() if re.fullmatch(r"Test[A-Za-z0-9_]+", line.strip())]
if set(discovered) != set(expected) or len(discovered) != len(expected):
    raise SystemExit(f"declared/discovered mismatch: declared={sorted(expected)}, discovered={sorted(discovered)}")

required_mutants = args.require_selector_mutants.split(",")
if required_mutants != selector["requiredSelectorMutants"]:
    raise SystemExit("required selector mutant list differs from the manifest")
mutants = {
    "zero": [],
    "missing": expected[:-1],
    "extra": expected + ["TestCLIWP01SelectorUnexpectedExtra"],
    "renamed": [expected[0] + "Renamed", *expected[1:]],
}
killed = []
for name in required_mutants:
    candidate = mutants[name]
    if set(candidate) == set(discovered) and len(candidate) == len(discovered):
        raise SystemExit(f"selector mutant survived: {name}")
    killed.append(name)

events_path = Path(args.json_events)
events_path.parent.mkdir(parents=True, exist_ok=True)
executed = subprocess.run(["go", "test", "-run", pattern, f"-count={args.count}", "-json", *packages], cwd=root, env=environment, text=True, capture_output=True)
events_path.write_text(executed.stdout, encoding="utf-8")
if executed.returncode != 0:
    sys.stderr.write(executed.stdout + executed.stderr)
    raise SystemExit(executed.returncode)

started = set()
terminal = {}
for line in executed.stdout.splitlines():
    try:
        event = json.loads(line)
    except json.JSONDecodeError:
        continue
    test = event.get("Test", "")
    if not test or "/" in test or test not in expected:
        continue
    if event.get("Action") == "run":
        started.add(test)
    if event.get("Action") in {"pass", "fail", "skip"}:
        if test in terminal:
            raise SystemExit(f"duplicate terminal event: {test}")
        terminal[test] = event["Action"]

sets = {
    "declared": set(expected),
    "discovered": set(discovered),
    "selected": set(expected),
    "started": started,
    "terminal": set(terminal),
    "executed": set(terminal),
}
if len({frozenset(value) for value in sets.values()}) != 1:
    raise SystemExit("selector accounting set equality failed: " + repr({key: sorted(value) for key, value in sets.items()}))
bad = {name: action for name, action in terminal.items() if action != "pass"}
if bad:
    raise SystemExit("non-PASS terminal events: " + repr(bad))

summary = {
    "schemaVersion": "cli-go-test-selector-result/v1",
    "result": "PASS",
    "selectorId": selector["selectorId"],
    "suite": args.suite,
    "accounting": {key: len(value) for key, value in sets.items()},
    "failed": 0,
    "skipped": 0,
    "omitted": 0,
    "selectorMutantsKilled": killed,
    "jsonEvents": str(events_path),
}
json.dump(summary, sys.stdout, indent=2, sort_keys=True)
sys.stdout.write("\n")
PY
