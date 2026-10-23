#!/usr/bin/env bash
# Trusted checkpoint publisher. Never call from within a model-controlled shell.
set -Eeuo pipefail
: "${FLEET_WORKTREE:?}" "${FLEET_BRANCH:?}" "${FLEET_ISSUE:?}" "${FLEET_ALLOWED_PATHS:?}"
: "${RUNNER_TEMP:?}" "${GITHUB_REPOSITORY:?}" "${GH_TOKEN:?}"
[[ "$FLEET_ISSUE" =~ ^[1-9][0-9]*$ ]] || exit 2
[[ "$FLEET_BRANCH" == "agent/fleet/issue-$FLEET_ISSUE" ]] || exit 2
[[ -d "$FLEET_WORKTREE" && "$FLEET_WORKTREE" == "$RUNNER_TEMP"/* ]] || exit 2
exec 9>"$RUNNER_TEMP/fleet-$FLEET_ISSUE.lock"
flock -x 9

read -r -a allowlist <<< "$FLEET_ALLOWED_PATHS"
selected=()
for path in "${allowlist[@]}"; do
  [[ "$path" =~ ^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$ ]] || exit 2
  if [[ -e "$FLEET_WORKTREE/$path" ]] || [[ -n "$(git -C "$FLEET_WORKTREE" ls-files -- "$path")" ]]; then
    selected+=("$path")
  fi
done
if (( ${#selected[@]} == 0 )); then
  echo "No allowlisted paths to checkpoint."
  exit 0
fi
git -C "$FLEET_WORKTREE" -c core.hooksPath=/dev/null add -A -- "${selected[@]}"
# Deny symlinks and known secret-like paths even inside the source allowlist.
while IFS= read -r path; do
  [[ -n "$path" ]] || continue
  case "/$path" in
    */.env|*/.env.*|*/credentials.json|*/token.json|*.pem|*.key|*id_rsa*) echo "::error::Sensitive path staged"; exit 1 ;;
  esac
  [[ ! -L "$FLEET_WORKTREE/$path" ]] || { echo "::error::Symlink staged"; exit 1; }
done < <(git -C "$FLEET_WORKTREE" diff --cached --name-only)
if git -C "$FLEET_WORKTREE" diff --cached --quiet; then
  echo "No new changes; previous checkpoint, if any, remains intact."
  exit 0
fi
git -C "$FLEET_WORKTREE" diff --cached --check || echo "::warning::WIP has whitespace findings, saving it anyway"
git -C "$FLEET_WORKTREE" -c core.hooksPath=/dev/null \
  commit -m "wip(agent): checkpoint issue #$FLEET_ISSUE (run $GITHUB_RUN_ID)"
# Reset any agent-modified origin URL, forbid force push; only the scoped branch.
git -C "$FLEET_WORKTREE" remote set-url origin "https://github.com/$GITHUB_REPOSITORY.git"
git -C "$FLEET_WORKTREE" -c credential.helper='!gh auth git-credential' \
  push origin "HEAD:refs/heads/$FLEET_BRANCH"
sha="$(git -C "$FLEET_WORKTREE" rev-parse HEAD)"
echo "Published checkpoint $sha to $FLEET_BRANCH"
if [[ "${FLEET_CREATE_DRAFT:-false}" == true ]]; then
  number="$(gh pr list --repo "$GITHUB_REPOSITORY" --state open \
    --head "$FLEET_BRANCH" --json number --jq '.[0].number // empty')"
  if [[ -z "$number" ]]; then
    gh pr create --repo "$GITHUB_REPOSITORY" --draft --base "$FLEET_BASE" \
      --head "$FLEET_BRANCH" --title "WIP(agent): Issue #$FLEET_ISSUE fleet checkpoint" \
      --body "Draft checkpoint for #$FLEET_ISSUE. This is unreviewed, possibly incomplete agent output. Review, tests, and human merge approval are mandatory. See the Actions run for execution status."
  else
    echo "Existing Draft PR #$number updated."
  fi
fi
