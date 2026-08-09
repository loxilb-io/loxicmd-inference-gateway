#!/usr/bin/env bash
#
# release-hygiene.sh — repeatable hygiene gate for the public repository.
#
# Fails non-zero on any finding. Run locally before pushing and in CI on
# every PR. Checks the *committed* tree (HEAD) and the commit range being
# published, not the working tree.
#
# All checks here are generic by design. Organization-specific deny-lists
# (which cannot themselves be published) are supplied at run time via
# HYGIENE_EXTRA_PATTERNS: a newline-separated list of extended-regex
# patterns, sourced from a CI secret or a local gitignored file. When the
# variable is empty the extra check is skipped. Secret detection is
# delegated to gitleaks (see .gitleaks.toml and check 6).
#
# Usage:
#   scripts/release-hygiene.sh [--base <ref>] [--skip-gitleaks]
#
#   --base <ref>      history range to audit is <ref>..HEAD
#                     (default: origin/main merge-base; falls back to full
#                     history)
#   --skip-gitleaks   skip the gitleaks secret scan (e.g. when the binary
#                     is unavailable in a restricted CI job)
set -u
cd "$(dirname "$0")/.."

BASE=""
SKIP_GITLEAKS=0
while [ $# -gt 0 ]; do
  case "$1" in
    --base) BASE="$2"; shift 2 ;;
    --skip-gitleaks) SKIP_GITLEAKS=1; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

FAIL=0
fail() { FAIL=1; printf 'FAIL: %s\n' "$1"; }
pass() { printf 'ok:   %s\n' "$1"; }

# Resolve the history range to audit.
if [ -z "$BASE" ]; then
  if git rev-parse -q --verify origin/main >/dev/null 2>&1; then
    BASE=$(git merge-base origin/main HEAD 2>/dev/null) || BASE=""
  fi
fi
RANGE=${BASE:+$BASE..}HEAD

# 1. AI attribution in commit messages ----------------------------------------
if git log --format='%an %ae %B' "$RANGE" 2>/dev/null \
     | grep -qiE 'claude|anthropic|copilot|openai codex|co-authored-by:.*(bot|ai)'; then
  fail "AI attribution found in commit messages ($RANGE)"
else
  pass "commit messages clean ($RANGE)"
fi

# 2. AI tooling artifacts tracked ---------------------------------------------
if git ls-files | grep -qE '(^|/)(CLAUDE\.md|AGENTS\.md|GEMINI\.md|\.mcp\.json)$|(^|/)(\.claude|claudedocs|\.planning|\.serena|claude-agents|\.codegraph)/'; then
  fail "AI tooling artifacts are tracked:"
  git ls-files | grep -E '(^|/)(CLAUDE\.md|AGENTS\.md|GEMINI\.md|\.mcp\.json)$|(^|/)(\.claude|claudedocs|\.planning|\.serena|claude-agents|\.codegraph)/' | head -10
else
  pass "no AI tooling artifacts tracked"
fi

# 3. Personal identifiers (home directories, personal paths) ------------------
if git grep -Iqn -E '/Users/[A-Za-z0-9._-]+/|/home/[a-z][a-z0-9._-]*/(go|src|work)/' HEAD -- \
     ':(exclude)scripts/release-hygiene.sh' 2>/dev/null; then
  fail "personal paths in tracked files:"
  git grep -In -E '/Users/[A-Za-z0-9._-]+/|/home/[a-z][a-z0-9._-]*/(go|src|work)/' HEAD -- \
     ':(exclude)scripts/release-hygiene.sh' | head -20
else
  pass "no personal paths"
fi

# 4. Links into docs/internal/ from tracked files -----------------------------
# .gitignore, .gitleaks.toml and this script legitimately *name* the
# docs/internal/ path (to exclude it); only real content links are flagged.
DOCS_INTERNAL_EXCLUDES=(
  ':(exclude).gitignore' ':(exclude).gitleaks.toml'
  ':(exclude)scripts/release-hygiene.sh'
)
if git grep -Iqn 'docs/internal/' HEAD -- "${DOCS_INTERNAL_EXCLUDES[@]}" 2>/dev/null; then
  fail "tracked files link into docs/internal/:"
  git grep -In 'docs/internal/' HEAD -- "${DOCS_INTERNAL_EXCLUDES[@]}" | head -20
else
  pass "no docs/internal references"
fi

# 5. Private key material (belt-and-braces on top of gitleaks) ----------------
if git grep -Iqn -E 'BEGIN (RSA |EC |OPENSSH |)PRIVATE KEY' HEAD 2>/dev/null; then
  fail "private key material in tracked files:"
  git grep -In -E 'BEGIN (RSA |EC |OPENSSH |)PRIVATE KEY' HEAD | head -10
else
  pass "no private key material"
fi

# 6. Secrets — gitleaks over the published history ----------------------------
if [ "$SKIP_GITLEAKS" = 1 ]; then
  echo "skip: gitleaks (--skip-gitleaks)"
elif command -v gitleaks >/dev/null 2>&1; then
  if [ -n "$BASE" ]; then LOGOPTS="$BASE..HEAD"; else LOGOPTS="--all"; fi
  if gitleaks detect --source . --config .gitleaks.toml --log-opts="$LOGOPTS" --no-banner --redact -v >/dev/null 2>&1; then
    pass "gitleaks clean ($LOGOPTS)"
  else
    fail "gitleaks found candidate secrets — run: gitleaks detect --source . --config .gitleaks.toml --log-opts=\"$LOGOPTS\" --redact -v"
  fi
else
  fail "gitleaks not installed (use --skip-gitleaks to bypass locally)"
fi

# 7. Organization-supplied extra patterns (never published) -------------------
# HYGIENE_EXTRA_PATTERNS holds newline-separated extended regexes; in CI it
# comes from a repository secret, locally from e.g.
#   HYGIENE_EXTRA_PATTERNS="$(cat docs/internal/hygiene-patterns.txt)" \
#     scripts/release-hygiene.sh
if [ -n "${HYGIENE_EXTRA_PATTERNS:-}" ]; then
  extra_hit=0
  while IFS= read -r pat; do
    [ -z "$pat" ] && continue
    if git grep -Iqn -E "$pat" HEAD -- ':(exclude)scripts/release-hygiene.sh' 2>/dev/null; then
      # Report the file locations but never echo the pattern itself.
      fail "extra-pattern match in tracked files:"
      git grep -Iln -E "$pat" HEAD -- ':(exclude)scripts/release-hygiene.sh' | head -10
      extra_hit=1
    fi
    if git log --format='%B' "$RANGE" 2>/dev/null | grep -qiE "$pat"; then
      fail "extra-pattern match in commit messages ($RANGE)"
      extra_hit=1
    fi
  done <<EOF
${HYGIENE_EXTRA_PATTERNS}
EOF
  [ "$extra_hit" = 0 ] && pass "no extra-pattern matches"
else
  echo "skip: extra patterns (HYGIENE_EXTRA_PATTERNS unset)"
fi

exit $FAIL
