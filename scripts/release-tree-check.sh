#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
cd "$repo_root"

forbidden_paths=(
  ".codex"
  ".superpowers"
  ".workspace-archive"
  "assets/characters"
  "docs/content-series"
  "docs/superpowers"
  "logs"
  "outputs"
  "test"
  "test全流程"
  "tmp"
  "ip-assets/main-ip/backgrounds"
  "ip-assets/main-ip/renders"
  "ip-assets/main-ip/turnaround"
  "ip-assets/main-ip/scenes/assets"
  "ip-assets/main-ip/scenes/references"
)

failed="false"
for path in "${forbidden_paths[@]}"; do
  tracked="$(git ls-files -- "$path")"
  if [[ -n "$tracked" ]]; then
    echo "ERROR: release contains forbidden tracked path: $path" >&2
    printf '%s\n' "$tracked" >&2
    failed="true"
  fi
done

unexpected_ip_assets="$(
  git -c core.quotepath=false ls-files -- 'ip-assets' |
    grep -Ev '^ip-assets/main-ip/' || true
)"
if [[ -n "$unexpected_ip_assets" ]]; then
  echo "ERROR: release contains non-canonical IP assets:" >&2
  printf '%s\n' "$unexpected_ip_assets" >&2
  failed="true"
fi

non_english_paths="$(
  git -c core.quotepath=false ls-files |
    python3 -c 'import sys; [print(path.rstrip()) for path in sys.stdin if any(ord(char) > 127 for char in path)]'
)"
if [[ -n "$non_english_paths" ]]; then
  echo "ERROR: release contains non-English tracked path names:" >&2
  printf '%s\n' "$non_english_paths" >&2
  failed="true"
fi

generated_files="$(git ls-files | grep -E '(^|/)(\.DS_Store|.*\.blend1)$' || true)"
if [[ -n "$generated_files" ]]; then
  echo "ERROR: release contains generated backup files:" >&2
  printf '%s\n' "$generated_files" >&2
  failed="true"
fi

required_files=(
  ".github/workflows/branch-guard.yml"
  ".github/workflows/ci.yml"
  ".github/workflows/release.yml"
  "docs/BETA_RUNBOOK.md"
  "frontend/package-lock.json"
  "ip-assets/main-ip/character-profile.json"
  "ip-assets/main-ip/manifests/default-aroll-assets.json"
  "ip-assets/main-ip/models/main-ip-aroll-master-20260720.blend"
  "ip-assets/main-ip/models/main-ip-rigged.glb"
  "ip-assets/main-ip/scenes/warm-sloth-studio-20260720.blend"
  "ip-assets/main-ip/voice/reference/main_ip_voice_ref_v1.wav"
  "scripts/beta-smoke-check.sh"
  "scripts/release-version-check.sh"
)
for path in "${required_files[@]}"; do
  if [[ ! -f "$path" ]]; then
    echo "ERROR: release is missing required file: $path" >&2
    failed="true"
  fi
done

required_release_workflow_contracts=(
  "workflow_dispatch:"
  "electron_arch: x64"
  "go_arch: amd64"
  "CGO_ENABLED=0 GOOS=darwin GOARCH=\${{ matrix.go_arch }}"
  "gh release view \"\$RELEASE_TAG\""
)
for contract in "${required_release_workflow_contracts[@]}"; do
  if ! grep -Fq "$contract" .github/workflows/release.yml; then
    echo "ERROR: desktop release workflow is missing contract: $contract" >&2
    failed="true"
  fi
done

bash scripts/release-version-check.sh

if [[ "$failed" == "true" ]]; then
  exit 1
fi

echo "release tree contract passed"
