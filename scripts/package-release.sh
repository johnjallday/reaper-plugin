#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=${1:-}
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "usage: $0 v<major>.<minor>.<patch>" >&2; exit 2 ;;
esac

cd "$repo_root"
manifest=.ori-plugin/plugin.json
before=$(mktemp "${TMPDIR:-/tmp}/reaper-plugin-manifest.XXXXXX")
trap 'rm -f "$before"' EXIT HUP INT TERM
cp "$manifest" "$before"

./scripts/build-local-artifact.sh
if ! cmp -s "$before" "$manifest"; then
  echo "release build does not match the committed manifest digest/size" >&2
  diff -u "$before" "$manifest" >&2 || true
  exit 1
fi
./scripts/verify-artifact.sh

artifact=artifacts/reaper-plugin-darwin-arm64
binary_version=$("./$artifact" version)
[ "v$binary_version" = "$version" ] || {
  echo "tag $version does not match binary version $binary_version" >&2
  exit 1
}

asset="reaper-plugin_${version}_darwin_arm64"
dist=${RELEASE_DIST_DIR:-$repo_root/dist}
mkdir -p "$dist"
rm -f "$dist/$asset" "$dist/$asset.sha256"
cp "$artifact" "$dist/$asset"
chmod 0755 "$dist/$asset"
(
  cd "$dist"
  shasum -a 256 "$asset" > "$asset.sha256"
)

printf 'release asset: %s\n' "$dist/$asset"
printf 'checksum: %s\n' "$dist/$asset.sha256"
