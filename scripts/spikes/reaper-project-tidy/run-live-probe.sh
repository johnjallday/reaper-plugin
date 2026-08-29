#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root="$HOME/.ori-reaper"
result_root="$root/reaper-away-spike"
runner_id=$(tr -d '[:space:]' < "$root/runner.id")

configured_port=$(grep -i '^csurf_' "$HOME/Library/Application Support/REAPER/reaper.ini" | awk 'NR == 1 {print $3}')
port=$configured_port
port_source=config
if ! curl -fsS -m 2 "http://127.0.0.1:$port/_/TRANSPORT" >/dev/null 2>&1; then
  port=$(/usr/sbin/lsof -nP -a -c REAPER -iTCP -sTCP:LISTEN -Fn 2>/dev/null | awk -F: '/^n/ {print $NF; exit}')
  port_source=listener
fi
[ -n "$port" ] || { echo 'REAPER Web Remote listener unavailable' >&2; exit 1; }
curl -fsS -m 5 "http://127.0.0.1:$port/_/TRANSPORT" >/dev/null

mkdir -p "$result_root"
chmod 0750 "$result_root"
printf '{"configured_port":%s,"reachable_port":%s,"port_source":"%s"}\n' \
  "$configured_port" "$port" "$port_source" > "$result_root/preflight.json"
chmod 0600 "$result_root/preflight.json"

run_lua() {
  source=$1
  rm -f "$root/last_status.txt"
  cp "$source" "$root/inbox.lua"
  chmod 0600 "$root/inbox.lua"
  curl -sS -m 8 "http://127.0.0.1:$port/_/$runner_id" >/dev/null 2>&1 || true
  iteration=0
  while [ ! -f "$root/last_status.txt" ]; do
    iteration=$((iteration + 1))
    [ "$iteration" -lt 100 ] || { echo 'runner status timed out' >&2; exit 1; }
    sleep 0.1
  done
  status=$(tr -d '\r\n' < "$root/last_status.txt")
  [ "$status" = ok ] || { echo "runner status=$status" >&2; exit 1; }
}

run_lua "$script_dir/live-api-probe.lua"
run_lua "$script_dir/read-only-runner-probe.lua"
run_lua "$script_dir/post-runner-probe.lua"
run_lua "$script_dir/marker-id-namespace-probe.lua"
run_lua "$script_dir/restore-original-probe.lua"

cp "$result_root/preflight.json" "$script_dir/observed-preflight.json"
cp "$result_root/setup-result.json" "$script_dir/observed-setup.json"
cp "$result_root/read-result.json" "$script_dir/observed-read.json"
cp "$result_root/post-runner-result.json" "$script_dir/observed-post-runner.json"
cp "$result_root/marker-id-result.json" "$script_dir/observed-marker-ids.json"
cp "$result_root/marker-post-result.json" "$script_dir/observed-marker-post.json"
chmod 0600 "$script_dir"/observed-*.json
printf 'Live probe complete; original project tab restored.\n'
