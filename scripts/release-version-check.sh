#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
cd "$repo_root"

root_version="$(node -p "require('./package.json').version")"
frontend_version="$(node -p "require('./frontend/package.json').version")"
render_version="$(node -p "require('./hyperframes-render-service/package.json').version")"
root_lock_version="$(node -p "require('./package-lock.json').version")"
frontend_lock_version="$(node -p "require('./frontend/package-lock.json').version")"
render_lock_version="$(node -p "require('./hyperframes-render-service/package-lock.json').version")"
desktop_product_name="$(node -p "require('./frontend/package.json').build.productName")"
desktop_artifact_name="$(node -p "require('./frontend/package.json').build.artifactName")"

if [[ "$root_version" != "$frontend_version" ]] || [[ "$root_version" != "$render_version" ]]; then
  echo "ERROR: package versions do not match: root=$root_version frontend=$frontend_version render=$render_version" >&2
  exit 1
fi

if [[ "$root_version" != "$root_lock_version" ]] || [[ "$root_version" != "$frontend_lock_version" ]] || [[ "$root_version" != "$render_lock_version" ]]; then
  echo "ERROR: lockfile versions do not match v$root_version: root=$root_lock_version frontend=$frontend_lock_version render=$render_lock_version" >&2
  exit 1
fi

if [[ "$desktop_product_name" != "Tangying AI Video Creator" ]]; then
  echo "ERROR: desktop productName must use the English release name: $desktop_product_name" >&2
  exit 1
fi

if [[ "$desktop_artifact_name" != 'Tangying-AI-Video-Creator-${version}-${os}-${arch}.${ext}' ]]; then
  echo "ERROR: desktop artifactName does not match the English release contract: $desktop_artifact_name" >&2
  exit 1
fi

grep -Fq "Release-v${root_version}" README.md || {
  echo "ERROR: README release badge does not match v${root_version}" >&2
  exit 1
}

grep -Fq "Current release: \`v${root_version}\`" docs/RELEASE_STATUS.md || {
  echo "ERROR: docs/RELEASE_STATUS.md does not match v${root_version}" >&2
  exit 1
}

grep -Fq "## v${root_version} -" CHANGELOG.md || {
  echo "ERROR: CHANGELOG.md does not contain v${root_version}" >&2
  exit 1
}

if [[ "${GITHUB_REF_TYPE:-}" == "tag" && "${GITHUB_REF_NAME:-}" != "v${root_version}" ]]; then
  echo "ERROR: tag ${GITHUB_REF_NAME:-<unset>} does not match package version v${root_version}" >&2
  exit 1
fi

echo "release version consistency passed: v${root_version}"
