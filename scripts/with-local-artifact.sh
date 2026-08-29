#!/bin/sh
set -eu

[ "$#" -gt 0 ] || { echo "usage: $0 COMMAND [ARG ...]" >&2; exit 2; }
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
manifest="$repo_root/.ori-plugin/plugin.json"
artifact="$repo_root/artifacts/reaper-plugin-darwin-arm64"
backup=$(mktemp "${TMPDIR:-/tmp}/reaper-plugin-manifest.XXXXXX")
cp "$manifest" "$backup"
restore() {
  if [ -f "$backup" ]; then
    cp "$backup" "$manifest"
    rm -f "$backup"
  fi
}
trap restore EXIT
trap 'exit 130' HUP INT TERM

(
  cd "$repo_root"
  ./scripts/build-local-artifact.sh >/dev/null
)
python3 - "$manifest" "$artifact" <<'PY'
import hashlib
import json
import sys
from pathlib import Path

manifest = Path(sys.argv[1])
artifact = Path(sys.argv[2])
data = json.loads(manifest.read_text())
payload = artifact.read_bytes()
entry = data["services"][0]["artifacts"][0]
entry["source"] = {
    "kind": "bundled",
    "path": "artifacts/reaper-plugin-darwin-arm64",
}
entry["sha256"] = hashlib.sha256(payload).hexdigest()
entry["size"] = len(payload)
manifest.write_text(json.dumps(data, indent=2) + "\n")
PY

"$@"
