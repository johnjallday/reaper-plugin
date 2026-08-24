#!/bin/sh
set -eu

config="$HOME/Library/Application Support/REAPER/reaper.ini"
configured=$(awk -F'[ =]' '/^csurf_[^=]*=(HTTP|WEBR) / { for (i=1;i<=NF;i++) if ($i ~ /^[0-9]+$/ && $i > 1024 && $i < 65536) { print $i; exit } }' "$config" 2>/dev/null || true)
port=${configured:-2307}
transport=$(curl -fsS -m 3 "http://127.0.0.1:$port/_/TRANSPORT" 2>/dev/null || true)
if [ -z "$transport" ]; then
  port=$(/usr/sbin/lsof -nP -a -c REAPER -iTCP -sTCP:LISTEN -Fn 2>/dev/null | awk -F: '/^n/ {print $NF; exit}')
  [ -n "$port" ] || { echo "REAPER Web Remote is unavailable" >&2; exit 1; }
  transport=$(curl -fsS -m 3 "http://127.0.0.1:$port/_/TRANSPORT")
fi
printf 'connected port=%s transport=%s\n' "$port" "$(printf '%s' "$transport" | cut -f2)"
original_state=$(printf '%s' "$transport" | cut -f2)

curl -fsS -m 3 "http://127.0.0.1:$port/_/1016" >/dev/null
stopped=$(curl -fsS -m 3 "http://127.0.0.1:$port/_/TRANSPORT" | cut -f2)
[ "$stopped" = "0" ] || { echo "stop check failed" >&2; exit 1; }
curl -fsS -m 3 "http://127.0.0.1:$port/_/1007" >/dev/null
playing=$(curl -fsS -m 3 "http://127.0.0.1:$port/_/TRANSPORT" | cut -f2)
[ $((playing & 1)) -eq 1 ] || { echo "play check failed" >&2; exit 1; }
curl -fsS -m 3 "http://127.0.0.1:$port/_/1016" >/dev/null

# Toggle metronome twice: exercise one undoable catalog action with no lasting state change.
curl -fsS -m 3 "http://127.0.0.1:$port/_/40364" >/dev/null
curl -fsS -m 3 "http://127.0.0.1:$port/_/40364" >/dev/null

runner_root="$HOME/.ori-reaper"
runner_id=$(tr -d '[:space:]' < "$runner_root/runner.id")
case "$runner_id" in
  _[A-Za-z0-9]*|[1-9][0-9]*) ;;
  *) echo "runner id is unavailable" >&2; exit 1 ;;
esac
backup=$(mktemp -d)
cleanup() {
  for name in inbox.lua last_status.txt; do
    if [ -f "$backup/$name" ]; then cp "$backup/$name" "$runner_root/$name"; else rm -f "$runner_root/$name"; fi
  done
  rm -rf "$backup"
  if [ $((original_state & 1)) -eq 1 ]; then curl -fsS -m 3 "http://127.0.0.1:$port/_/1007" >/dev/null || true; fi
}
trap cleanup EXIT INT TERM
for name in inbox.lua last_status.txt; do [ ! -f "$runner_root/$name" ] || cp "$runner_root/$name" "$backup/$name"; done
rm -f "$runner_root/last_status.txt"
printf '%s\n' 'reaper.ShowConsoleMsg("Ori plugin live smoke: draft runner OK\\n")' > "$runner_root/inbox.lua"
chmod 0600 "$runner_root/inbox.lua"
curl -fsS -m 3 "http://127.0.0.1:$port/_/$runner_id" >/dev/null
for _ in 1 2 3 4 5 6 7 8 9 10; do
  [ ! -f "$runner_root/last_status.txt" ] || break
  sleep 0.1
done
status=$(tr -d '\r\n' < "$runner_root/last_status.txt")
[ "$status" = "ok" ] || { echo "draft runner status=$status" >&2; exit 1; }
printf 'play=ok stop=ok safe_action=ok draft=ok\n'
