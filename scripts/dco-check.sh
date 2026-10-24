#!/bin/sh
# Kubona DCO check (CONTRIBUTING.md: every commit carries Signed-off-by).
# Scope: only commits NEW to the target branch — history stays green.
#   push to main/develop: github.event.before .. github.sha
#   pull_request:         base SHA .. head SHA
# Merge commits are exempt (authorship is the merged commits', already
# checked). Dependabot is exempt at the workflow level (it cannot sign;
# its updates are reviewed, not authored) — every human commit signs.
#
# Env (set by the workflow): DCO_BASE, DCO_HEAD. Local use:
#   DCO_BASE=origin/main DCO_HEAD=HEAD ./scripts/dco-check.sh
set -eu

base="${DCO_BASE:-}"
head="${DCO_HEAD:-HEAD}"

if [ -z "$base" ]; then
  echo "DCO: DCO_BASE unset and no default — refusing to guess a range." >&2
  echo "Pass DCO_BASE=<merge-base-or-before-sha> explicitly." >&2
  exit 2
fi

if [ "$base" = "NONE" ]; then
  # workflow_dispatch and similar gateless triggers: nothing landed, so
  # there is nothing new to sign. The push/PR paths always pass a range.
  echo "DCO: no commit range for this trigger — nothing to check." >&2
  exit 0
fi

if ! git cat-file -e "$base" 2>/dev/null; then
  # Shallow or rewritten ref (rebase/ff edge): fall back to checking just
  # the head commit rather than failing the gate on plumbing.
  echo "DCO: base $base not in local history; checking HEAD only." >&2
  range="$head -1"
else
  range="$base..$head"
fi

fail=0
bad=0
checked=0
for sha in $(git rev-list --no-merges $range 2>/dev/null); do
  checked=$((checked + 1))
  body=$(git log -1 --format=%B "$sha")
  case "$body" in
    *"Signed-off-by:"*"@"*"."* )
      ;;
    *)
      echo "DCO FAIL: $sha lacks a Signed-off-by trailer:" >&2
      echo "$body" | head -1 | sed 's/^/  /' >&2
      bad=$((bad + 1))
      fail=1
      ;;
  esac
done

if [ "$checked" -eq 0 ]; then
  echo "DCO: no non-merge commits in range $range — nothing to check." >&2
  exit 0
fi

if [ "$fail" -ne 0 ]; then
  echo "DCO: $bad unsigned commit(s). Sign with: git commit -s --amend (or -s on new commits)." >&2
  exit 1
fi
echo "DCO: $checked commit(s) signed."
