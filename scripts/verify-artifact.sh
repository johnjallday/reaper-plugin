#!/bin/sh
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"
artifact=artifacts/reaper-plugin-darwin-arm64
[ -x "$artifact" ] || { echo "artifact missing or not executable" >&2; exit 1; }
actual_sha=$(shasum -a 256 "$artifact" | awk '{print $1}')
actual_size=$(wc -c < "$artifact" | tr -d ' ')
declared=$(python3 - <<'PY'
import json
artifact=json.load(open('.ori-plugin/plugin.json'))['services'][0]['artifacts'][0]
print(artifact['sha256'], artifact['size'])
PY
)
set -- $declared
[ "$actual_sha" = "$1" ] || { echo "artifact digest mismatch" >&2; exit 1; }
[ "$actual_size" = "$2" ] || { echo "artifact size mismatch" >&2; exit 1; }
[ "$(stat -f '%Lp' "$artifact")" = 755 ] || { echo "artifact mode must be 0755 before packaging" >&2; exit 1; }
"./$artifact" version | grep -qx '0.5.0' || { echo "artifact CLI version contract changed unexpectedly" >&2; exit 1; }
printf 'verified %s bytes sha256=%s\n' "$actual_size" "$actual_sha"
