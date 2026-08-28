#!/bin/sh
set -eu

mode=${1:-}
target=${2:-}
state_file=${3:-}
case "$mode" in open|restore) ;; *) echo "usage: $0 open|restore TARGET STATE_FILE" >&2; exit 2;; esac
case "$target$state_file" in *"\n"*|*"\r"*|*"'"*) echo "unsafe path" >&2; exit 2;; esac
[ -n "$target" ] && [ -n "$state_file" ] || exit 2
port=$(/usr/sbin/lsof -nP -a -c REAPER -iTCP -sTCP:LISTEN -Fn 2>/dev/null | awk -F: '/^n/ {print $NF; exit}')
[ -n "$port" ] || { echo "REAPER listener unavailable" >&2; exit 1; }
root="$HOME/.ori-reaper"
runner_id=$(tr -d '[:space:]' < "$root/runner.id")
if [ "$mode" = open ]; then
  cat > "$root/inbox.lua" <<LUA
local _, previous = reaper.EnumProjects(-1, "")
local state = io.open('$state_file', 'w')
if not state then error('state file unavailable') end
state:write(tostring(previous or ''))
state:close()
reaper.Main_openProject('$target')
LUA
else
  previous=$(cat "$state_file")
  case "$previous" in *"'"*|*"\n"*|*"\r"*) echo "unsafe previous path" >&2; exit 1;; esac
  cat > "$root/inbox.lua" <<LUA
-- The smoke target is disposable and may remain dirty after undo. Restore the
-- previously open project without raising a modal save prompt for that target.
reaper.Main_openProject('noprompt:$previous')
LUA
fi
chmod 0600 "$root/inbox.lua"
rm -f "$root/last_status.txt"
# Opening a project can hold the Web Remote response while REAPER loads. The
# runner status file, not curl completion, is the authority on script outcome.
curl -sS -m 8 "http://127.0.0.1:$port/_/$runner_id" >/dev/null 2>&1 || true
for _ in $(jot 100 1); do
  [ ! -f "$root/last_status.txt" ] || break
  sleep 0.1
done
status=$(tr -d '\r\n' < "$root/last_status.txt")
[ "$status" = ok ] || { echo "runner status=$status" >&2; exit 1; }
