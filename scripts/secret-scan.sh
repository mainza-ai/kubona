#!/bin/sh
# Kubona secret + local-path scan (K6: the CI mirror of .git/hooks/pre-commit).
# Hooks are unversioned (lost on re-clone); THIS script is versioned, runs in
# CI, and any human can run it. Same patterns as the hook — if you change one,
# change the other (hook path: .git/hooks/pre-commit).
#
# Fails on:
#   1. tracked dev-docs/ or .obsidian/ paths (local-only working files),
#   2. tracked .env files (except .env.example),
#   3. high-confidence secret VALUES in tracked files (prefixes + key blocks).
# Name references (KUBONA_API_TOKEN) are fine; values are not.
set -eu

fail=0

tracked_paths=$(git ls-files | grep -E '^(dev-docs/|\.obsidian/)' || true)
if [ -n "$tracked_paths" ]; then
  echo "FAIL: local-only paths tracked in git (dev-docs/ and .obsidian/ are never pushed):" >&2
  echo "$tracked_paths" | sed 's/^/  /' >&2
  fail=1
fi

tracked_env=$(git ls-files | grep -E '(^|/)\.env($|\.)' | grep -v '\.env\.example$' || true)
if [ -n "$tracked_env" ]; then
  echo "FAIL: .env files tracked in git:" >&2
  echo "$tracked_env" | sed 's/^/  /' >&2
  fail=1
fi

hits=$(git grep -n -iE 'oc_sk_[A-Za-z0-9]+|ghp_[A-Za-z0-9]+|gho_[A-Za-z0-9]+|sk-ant-[A-Za-z0-9-]+|AKIA[0-9A-Z]{10,}|xox[bap]-[A-Za-z0-9-]+|-----BEGIN .*PRIV''ATE KEY-----' -- . 2>/dev/null || true)
# NOTE: pattern lists must not contain self-matching literals. Prefix
# classes (trailing [A-Za-z0-9]+) are safe; complete historical values
# match their own listing. The PRIV fragment above is split as PRIV''ATE
# (shell-concatenated back at runtime) so this file never matches itself
# while still catching real key blocks.
if [ -n "$hits" ]; then
  echo "FAIL: tracked files appear to contain secret values:" >&2
  echo "$hits" | cut -c1-160 | sed 's/^/  /' >&2
  fail=1
fi

if [ "$fail" -eq 0 ]; then
  echo "secret scan clean: no local-only paths, no .env files, no secret values in tracked files"
fi
exit "$fail"
