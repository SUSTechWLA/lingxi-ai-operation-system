#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT

mkdir -p "$fixture/outputs/demo" "$fixture/tmp" "$fixture/ip形象/main_ip"
printf 'render\n' > "$fixture/outputs/demo/result.txt"
printf 'cache\n' > "$fixture/tmp/cache.txt"
printf 'keep\n' > "$fixture/ip形象/main_ip/character-profile.json"

"$repo_root/scripts/workspace-archive.sh" archive --root "$fixture" --label test-snapshot

test ! -e "$fixture/outputs"
test ! -e "$fixture/tmp"
test -f "$fixture/.workspace-archive/test-snapshot/payload/outputs/demo/result.txt"
test -f "$fixture/.workspace-archive/test-snapshot/payload/tmp/cache.txt"
test -f "$fixture/ip形象/main_ip/character-profile.json"
grep -q $'outputs\tpayload/outputs' "$fixture/.workspace-archive/test-snapshot/manifest.tsv"

"$repo_root/scripts/workspace-archive.sh" restore --root "$fixture" --label test-snapshot

test -f "$fixture/outputs/demo/result.txt"
test -f "$fixture/tmp/cache.txt"
test -f "$fixture/ip形象/main_ip/character-profile.json"

echo "workspace archive test passed"
