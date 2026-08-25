#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
release_toolchain=go1.25.0
artifact="$repo_root/artifacts/reaper-plugin-darwin-arm64"
manifest="$repo_root/.ori-plugin/plugin.json"
mkdir -p "$repo_root/artifacts"

(
  cd "$repo_root"
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOTOOLCHAIN="$release_toolchain" \
    go build -buildvcs=false -trimpath -ldflags='-s -w -buildid=' -o "$artifact" ./cmd/reaper-plugin
)
chmod 0755 "$artifact"
sha=$(shasum -a 256 "$artifact" | awk '{print $1}')
size=$(wc -c < "$artifact" | tr -d ' ')

python3 - "$manifest" "$sha" "$size" <<'PY'
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
document = json.loads(path.read_text())
matched = False
for service in document.get("services", []):
    for artifact in service.get("artifacts", []):
        if artifact.get("id") == "reaper-plugin-darwin-arm64":
            artifact["sha256"] = sys.argv[2]
            artifact["size"] = int(sys.argv[3])
            matched = True
if not matched:
    raise SystemExit("manifest artifact reaper-plugin-darwin-arm64 is missing")
path.write_text(json.dumps(document, indent=2) + "\n")
PY

printf '%s  %s\n' "$sha" "artifacts/reaper-plugin-darwin-arm64"
printf 'size=%s\n' "$size"
