#!/usr/bin/env bash

set -euo pipefail

REPO="${GITHUB_REPOSITORY:-SUSTechWLA/tangying-ai-operation-system}"
REQUIRED_CHECKS="${REQUIRED_CHECKS:-branch-guard,lint,build,test}"
ALLOW_ADMIN_BYPASS="${ALLOW_ADMIN_BYPASS:-true}"
DRY_RUN="${DRY_RUN:-false}"

usage() {
  cat <<'EOF'
Usage: .github/setup/initialize_gitops.sh [options]

Configures GitHub branch governance rulesets for this repository.

Options:
  --repo OWNER/REPO          Repository to configure. Defaults to GITHUB_REPOSITORY
                             or SUSTechWLA/tangying-ai-operation-system.
  --required-checks LIST     Comma-separated required status checks.
                             Defaults to branch-guard,lint,build,test.
  --no-admin-bypass          Do not add the current GitHub user as a ruleset bypass actor.
  --dry-run                  Print the ruleset payloads without changing GitHub.
  -h, --help                 Show this help.

Examples:
  .github/setup/initialize_gitops.sh
  .github/setup/initialize_gitops.sh --dry-run
  REQUIRED_CHECKS=branch-guard,lint,build,test .github/setup/initialize_gitops.sh
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repo)
        REPO="${2:-}"
        shift 2
        ;;
      --required-checks)
        REQUIRED_CHECKS="${2:-}"
        shift 2
        ;;
      --no-admin-bypass)
        ALLOW_ADMIN_BYPASS="false"
        shift
        ;;
      --dry-run)
        DRY_RUN="true"
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "ERROR: unknown option '$1'." >&2
        usage >&2
        exit 1
        ;;
    esac
  done
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: required command '$1' was not found." >&2
    exit 1
  fi
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

status_checks_json() {
  local checks_json="["
  local first=1
  local check

  IFS=',' read -r -a checks <<< "$REQUIRED_CHECKS"
  for check in "${checks[@]}"; do
    check="$(trim "$check")"
    if [[ -z "$check" ]]; then
      continue
    fi

    if [[ "$first" -eq 0 ]]; then
      checks_json+=","
    fi
    checks_json+="{\"context\":\"${check}\"}"
    first=0
  done

  checks_json+="]"
  printf '%s' "$checks_json"
}

bypass_actors_json() {
  if [[ "$ALLOW_ADMIN_BYPASS" != "true" ]]; then
    printf '[]'
    return
  fi

  local actor_id
  actor_id="$(gh api user --jq '.id')"
  printf '[{"actor_id":%s,"actor_type":"User","bypass_mode":"always"}]' "$actor_id"
}

ruleset_id_by_name() {
  local name="$1"
  gh api "repos/${REPO}/rulesets" --jq ".[] | select(.name == \"${name}\") | .id" 2>/dev/null || true
}

upsert_ruleset() {
  local name="$1"
  local branch_ref="$2"
  local ruleset_id
  local payload
  local status_checks
  local bypass_actors

  ruleset_id="$(ruleset_id_by_name "$name" | head -n 1)"
  payload="$(mktemp)"
  status_checks="$(status_checks_json)"
  bypass_actors="$(bypass_actors_json)"

  cat > "$payload" <<JSON
{
  "name": "${name}",
  "target": "branch",
  "enforcement": "active",
  "bypass_actors": ${bypass_actors},
  "conditions": {
    "ref_name": {
      "include": ["${branch_ref}"],
      "exclude": []
    }
  },
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    { "type": "update" },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 1,
        "dismiss_stale_reviews_on_push": true,
        "require_code_owner_review": true,
        "require_last_push_approval": false,
        "required_review_thread_resolution": true,
        "allowed_merge_methods": ["merge", "squash", "rebase"],
        "required_reviewers": []
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "do_not_enforce_on_create": false,
        "required_status_checks": ${status_checks}
      }
    }
  ]
}
JSON

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "Dry run for ruleset '${name}' (${branch_ref}):"
    cat "$payload"
    rm -f "$payload"
    return
  fi

  if [[ -n "$ruleset_id" ]]; then
    echo "Updating ruleset '${name}' (${ruleset_id}) for ${branch_ref}"
    gh api "repos/${REPO}/rulesets/${ruleset_id}" --method PUT --input "$payload" --silent
  else
    echo "Creating ruleset '${name}' for ${branch_ref}"
    gh api "repos/${REPO}/rulesets" --method POST --input "$payload" --silent
  fi

  rm -f "$payload"
}

main() {
  parse_args "$@"
  require_cmd gh

  gh auth status >/dev/null

  local permission
  permission="$(gh repo view "$REPO" --json viewerPermission --jq '.viewerPermission')"
  if [[ "$permission" != "ADMIN" ]]; then
    echo "ERROR: ${REPO} requires ADMIN permission to manage rulesets. Current permission: ${permission}" >&2
    exit 1
  fi

  echo "Repository: ${REPO}"
  echo "Required checks: ${REQUIRED_CHECKS}"
  echo "Admin bypass actor enabled: ${ALLOW_ADMIN_BYPASS}"
  echo "Dry run: ${DRY_RUN}"

  upsert_ruleset "protect-release" "refs/heads/release"
  upsert_ruleset "protect-develop-go" "refs/heads/develop_go"

  if [[ "$DRY_RUN" == "true" ]]; then
    return
  fi

  echo "Current managed rulesets:"
  gh api "repos/${REPO}/rulesets" --jq '.[] | select(.name == "protect-release" or .name == "protect-develop-go") | {id, name, target, enforcement, conditions}'
}

main "$@"
