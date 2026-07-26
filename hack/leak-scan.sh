#!/usr/bin/env bash
#
# leak-scan.sh — fail if internal / AI-authorship artifacts leak into the tree.
# Run locally before every push; also runs in CI (.github/workflows/leak-scan.yml).
#
# Only files that would actually ship are scanned: when run inside a git repo we
# use `git ls-files` (respects .gitignore), otherwise we fall back to `find`.
# This script and SANITIZATION-CHECKLIST.md are excluded because they necessarily
# name the very patterns being searched for.
#
# Exit non-zero on any hit. Keep patterns tight to avoid false positives.

set -u
cd "$(dirname "$0")/.."

status=0
SELF="hack/leak-scan.sh"
CHECKLIST="SANITIZATION-CHECKLIST.md"

# Build the list of shippable files.
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  mapfile -t files < <(git ls-files)
else
  mapfile -t files < <(find . -path ./.git -prune -o -type f -print | sed 's|^\./||')
fi

# Drop the two self-referential files from content scanning.
scan_files=()
for f in "${files[@]}"; do
  [ "$f" = "$SELF" ] && continue
  [ "$f" = "$CHECKLIST" ] && continue
  scan_files+=("$f")
done

# 1) AI-authorship & internal tooling artifacts (paths that must not ship).
banned_paths=(
  "CLAUDE.md" "AGENTS.md" "GEMINI.md"
  ".claude" ".serena" ".mcp.json" ".codegraph" ".planning" "claudedocs"
  "LOXICMD-INFERENCE-GATEWAY-PLAN.md"
)
for f in "${files[@]}"; do
  for p in "${banned_paths[@]}"; do
    case "$f" in
      "$p"|*/"$p"|"$p"/*|*/"$p"/*)
        echo "LEAK: banned path shippable: $f"; status=1 ;;
    esac
  done
done

# 2) Content patterns that must never appear in shippable files.
banned_content=(
  "Co-Authored-By"
  "ghcr.io/netlox-dev"
  "docs.netlox.io"
  "NAVER"
  "/Users/"
)
# 3) Known internal/testbed IPs. Documentation ranges
#    (192.0.2.x / 198.51.100.x / 203.0.113.x) and 127.0.0.1 are allowed.
banned_ips=(
  "110.165.19.107"
)

if [ "${#scan_files[@]}" -gt 0 ]; then
  for pat in "${banned_content[@]}" "${banned_ips[@]}"; do
    hits=$(grep -RIn -- "$pat" "${scan_files[@]}" 2>/dev/null)
    if [ -n "$hits" ]; then
      echo "LEAK: banned content '$pat':"
      echo "$hits"
      status=1
    fi
  done
fi

if [ "$status" -eq 0 ]; then
  echo "leak-scan: clean"
fi
exit "$status"
