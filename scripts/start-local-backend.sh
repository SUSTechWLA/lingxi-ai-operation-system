#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

export TANGYING_WORKSPACE_ROOT="${TANGYING_WORKSPACE_ROOT:-${REPO_ROOT}}"
export TANGYING_LOCAL_DATA_DIR="${TANGYING_LOCAL_DATA_DIR:-${REPO_ROOT}/local-backend/data}"
export BIAOSHU_OUTPUT_DIR="${BIAOSHU_OUTPUT_DIR:-${REPO_ROOT}/biaoshu-tools/output}"

mkdir -p "${TANGYING_LOCAL_DATA_DIR}" "${BIAOSHU_OUTPUT_DIR}"

cd "${REPO_ROOT}/local-backend"
exec go run ./cmd/local-agent \
  -addr "${TANGYING_LOCAL_AGENT_ADDR:-127.0.0.1:18080}" \
  -workspace-root "${TANGYING_WORKSPACE_ROOT}" \
  -data-dir "${TANGYING_LOCAL_DATA_DIR}" \
  -biaoshu-output-dir "${BIAOSHU_OUTPUT_DIR}" \
  -cloud-api-base "${TANGYING_CLOUD_API_BASE:-${VITE_CLOUD_API_BASE:-}}"
