#!/usr/bin/env bash
# Local release dry run: proves the tree is clean enough and the three prod
# images build, without tagging, pushing, or signing anything.
set -euo pipefail

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "release dry-run refused: working tree is dirty (commit/stash first)" >&2
  exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "release dry-run refused: untracked files present" >&2
  git status --short >&2
  exit 1
fi

echo "== structural gates =="
python3 scripts/repo-counts.py --check
python3 scripts/wiki-lint.py --strict
bash scripts/secret-scan.sh
docker compose -f platform/docker/compose.prod.yml config >/dev/null
echo "compose config: ok"

echo "== build prod images =="
docker compose -f platform/docker/compose.prod.yml build

echo "== images (no push/sign) =="
for img in kubona/kubonad:prod kubona/console:prod kubona/bridge:prod; do
  docker image inspect "$img" --format '{{.RepoTags}} {{.Id}} {{.Created}}'
done

echo "release dry-run: ok — nothing tagged, pushed, or signed"
