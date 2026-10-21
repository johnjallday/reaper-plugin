#!/bin/sh
# REAPER Studio spike: run one probe Lua through the installed Ori runner and
# collect its outcome. Usage:
#
#   run-live-probe.sh <probe.lua> <evidence-dir> <label> [runner-command-id]
#
# It writes only under ~/.ori-reaper (the inbox the runner already owns and the
# reaper-studio-spike/ scratch folder) and under <evidence-dir>. It never
# installs, stages or registers anything and never touches REAPER's resource
# folder. Run it only while REAPER is open with a disposable or empty project.
# The optional fourth argument overrides the command ID read from runner.id,
# for example with the SCR row of ori-reaper-runner.lua in reaper-kb.ini when
# runner.id turns out to be stale.
set -eu

probe=$1
evidence=$2
label=$3
override=${4:-}
root="$HOME/.ori-reaper"
scratch="$root/reaper-studio-spike"
resource="$HOME/Library/Application Support/REAPER"

[ -f "$probe" ] || { echo "probe $probe is missing" >&2; exit 2; }
[ -d "$evidence" ] || { echo "evidence dir $evidence is missing" >&2; exit 2; }
[ -d "$root" ] && [ ! -L "$root" ] || { echo "runner exchange $root is missing or a link" >&2; exit 2; }
mkdir -p "$scratch"

# Port discovery: the configured HTTP interface first, then REAPER's listening
# sockets, the same order the Go probe uses.
configured=$(grep -E '^csurf_[0-9]+=HTTP ' "$resource/reaper.ini" | head -1 | awk '{print $3}')
candidates="$configured $(/usr/sbin/lsof -nP -a -c REAPER -iTCP -sTCP:LISTEN -Fn 2>/dev/null | sed -n 's/^n.*:\([0-9]*\)$/\1/p' | sort -u | tr '\n' ' ')"
port=""
for candidate in $candidates; do
  if curl -s -m 2 "http://127.0.0.1:$candidate/_/TRANSPORT" | grep -q '^TRANSPORT'; then
    port=$candidate
    break
  fi
done
[ -n "$port" ] || { echo "REAPER Web Remote is not reachable (tried: $candidates)" >&2; exit 3; }

runner_id=$(tr -d ' \n\r' < "$root/runner.id")
[ -z "$override" ] || runner_id=$override
case "$runner_id" in
  _*) ;;
  *) runner_id="_$runner_id" ;;
esac

# Transport and selection as Web Remote sees them, before the run.
curl -s -m 2 "http://127.0.0.1:$port/_/TRANSPORT" > "$evidence/$label-transport-before.txt"
curl -s -m 2 "http://127.0.0.1:$port/_/TRACK" > "$evidence/$label-track-before.txt"

# Stage the probe atomically in the runner's inbox and clear the status file.
tmp=$(mktemp "$root/.ori-studio-spike-XXXXXX")
cat "$probe" > "$tmp"
chmod 0600 "$tmp"
mv "$tmp" "$root/inbox.lua"
rm -f "$root/last_status.txt"
rm -f "$scratch/state.json" "$scratch/fx-list.json" "$scratch/fx-audit-probe-ran.txt"

started=$(date +%s)
curl -s -m 5 -o /dev/null -w '%{http_code}\n' "http://127.0.0.1:$port/_/$runner_id" > "$evidence/$label-trigger-http.txt"

status=""
tries=0
while [ $tries -lt 200 ]; do
  if [ -f "$root/last_status.txt" ]; then
    status=$(cat "$root/last_status.txt")
    [ -n "$status" ] && break
  fi
  tries=$((tries + 1))
  sleep 0.05
done
finished=$(date +%s)

printf 'port=%s\nrunner_id=%s\nhttp=%s\nstatus=%s\nwaited_seconds=%s\nrunner_id_after=%s\n' \
  "$port" "$runner_id" "$(cat "$evidence/$label-trigger-http.txt")" "${status:-<none within 10s>}" \
  "$((finished - started))" "$(tr -d ' \n\r' < "$root/runner.id")" > "$evidence/$label-status.txt"

for output in state.json fx-list.json fx-audit-probe-ran.txt; do
  if [ -f "$scratch/$output" ]; then
    cp "$scratch/$output" "$evidence/$label-$output"
  fi
done

curl -s -m 2 "http://127.0.0.1:$port/_/TRANSPORT" > "$evidence/$label-transport-after.txt"
curl -s -m 2 "http://127.0.0.1:$port/_/TRACK" > "$evidence/$label-track-after.txt"

cat "$evidence/$label-status.txt"
