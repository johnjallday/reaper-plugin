#!/bin/sh
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"
artifact=artifacts/reaper-plugin-darwin-arm64
[ -x "$artifact" ] || { echo "artifact missing or not executable" >&2; exit 1; }
actual_sha=$(shasum -a 256 "$artifact" | awk '{print $1}')
actual_size=$(wc -c < "$artifact" | tr -d ' ')
# The version is read from the manifest rather than written out here: this
# check exists to catch the binary and the manifest disagreeing, and a literal
# would instead have to be remembered on every release — which is exactly how
# it went stale.
declared=$(python3 - <<'PY'
import json
manifest=json.load(open('.ori-plugin/plugin.json'))
artifact=manifest['services'][0]['artifacts'][0]
print(artifact['sha256'], artifact['size'], manifest['version'])
PY
)
set -- $declared
[ "$actual_sha" = "$1" ] || { echo "artifact digest mismatch" >&2; exit 1; }
[ "$actual_size" = "$2" ] || { echo "artifact size mismatch" >&2; exit 1; }
[ "$(stat -f '%Lp' "$artifact")" = 755 ] || { echo "artifact mode must be 0755 before packaging" >&2; exit 1; }
"./$artifact" version | grep -qx "$3" || { echo "artifact CLI version ($("./$artifact" version)) does not match the manifest ($3)" >&2; exit 1; }
printf 'verified %s bytes sha256=%s\n' "$actual_size" "$actual_sha"
