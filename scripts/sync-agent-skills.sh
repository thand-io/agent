#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
shared_dir="$repo_root/.agent-shared/skills"
codex_dir="$repo_root/.agents/skills"
claude_dir="$repo_root/.claude/skills"

mkdir -p "$codex_dir" "$claude_dir"

for src in "$shared_dir"/*.md; do
  [ -e "$src" ] || continue

  name="$(basename "$src" .md)"
  codex_skill_dir="$codex_dir/$name"
  codex_out="$codex_skill_dir/SKILL.md"
  claude_out="$claude_dir/$name.md"
  source_ref=".agent-shared/skills/$name.md"

  mkdir -p "$codex_skill_dir"

  {
    printf '> Generated from `%s`. Edit the shared source and run `scripts/sync-agent-skills.sh`.\n\n' "$source_ref"
    cat "$src"
    printf '\n'
  } > "$codex_out"

  {
    printf '> Generated from `%s`. Edit the shared source and run `scripts/sync-agent-skills.sh`.\n\n' "$source_ref"
    cat "$src"
    printf '\n'
  } > "$claude_out"
done
