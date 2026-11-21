#!/usr/bin/env bash
# Install Kubona's external AI agent skills into common local agent host
# directories (Claude Code, Cursor, Codex, Antigravity).
#
# Copies files (never symlinks) so hosts that refuse symlinked skills load
# cleanly. Safe to re-run to update installed skills.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="${REPO_ROOT}/skills"

TARGET="${1:-}"
if [[ -z "${TARGET}" ]]; then
  cat <<'USAGE'
Usage: scripts/install-skills.sh <host>

Hosts:
  claude       Install into ~/.claude/skills/kubona*
  cursor       Install into ~/.cursor/skills/kubona*
  codex        Install into ~/.codex/skills/kubona*
  antigravity  Install into ~/.gemini/antigravity/skills/kubona*
  project      Install into ./.agents/skills/kubona* (current directory)
USAGE
  exit 1
fi

case "${TARGET}" in
  claude)      DEST_ROOT="${HOME}/.claude/skills" ;;
  cursor)      DEST_ROOT="${HOME}/.cursor/skills" ;;
  codex)       DEST_ROOT="${HOME}/.codex/skills" ;;
  antigravity) DEST_ROOT="${HOME}/.gemini/antigravity/skills" ;;
  project)     DEST_ROOT="$(pwd)/.agents/skills" ;;
  *)
    echo "Unknown host '${TARGET}'. Run without arguments for usage." >&2
    exit 2
    ;;
esac

mkdir -p "${DEST_ROOT}"
count=0
for skill_dir in "${SRC_DIR}"/kubona*; do
  [[ -d "${skill_dir}" && -f "${skill_dir}/SKILL.md" ]] || continue
  name="$(basename "${skill_dir}")"
  mkdir -p "${DEST_ROOT}/${name}"
  cp "${skill_dir}/SKILL.md" "${DEST_ROOT}/${name}/SKILL.md"
  count=$((count + 1))
done

echo "Installed ${count} Kubona skills into ${DEST_ROOT}"
