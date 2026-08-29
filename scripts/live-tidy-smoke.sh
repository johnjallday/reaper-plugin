#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture="$repo/scripts/spikes/reaper-project-tidy/fixtures/messy-project.RPP"
inspector="$repo/skills/reaper-project-tidy/scripts/inspect_project.lua"
applier="$repo/skills/reaper-project-tidy/scripts/apply_tidy_plan.lua"
switcher="$repo/scripts/live-project-smoke.sh"
root="$HOME/.ori-reaper"
work=$(mktemp -d "${TMPDIR:-/tmp}/ori-tidy-smoke.XXXXXX")
previous="$work/previous-project.txt"
opened=false
fixture_mtime=$(stat -f %m "$fixture")

cleanup() {
  if [ "$opened" = true ]; then
    "$switcher" restore "$fixture" "$previous" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM

port=$(/usr/sbin/lsof -nP -a -c REAPER -iTCP -sTCP:LISTEN -Fn 2>/dev/null | awk -F: '/^n/ {print $NF; exit}')
[ -n "$port" ] || { echo "REAPER listener unavailable" >&2; exit 1; }
runner_id=$(tr -d '[:space:]' < "$root/runner.id")

run_lua() {
  source=$1
  rm -f "$root/last_status.txt"
  cp "$source" "$root/inbox.lua"
  chmod 0600 "$root/inbox.lua"
  curl -sS -m 8 "http://127.0.0.1:$port/_/$runner_id" >/dev/null 2>&1 || true
  iteration=0
  while [ ! -f "$root/last_status.txt" ]; do
    iteration=$((iteration + 1))
    [ "$iteration" -lt 100 ] || { echo "runner status timed out" >&2; exit 1; }
    sleep 0.1
  done
  tr -d '\r\n' < "$root/last_status.txt"
}

inspect() {
  status=$(run_lua "$inspector")
  [ "$status" = ok ] || { echo "inspector status=$status" >&2; exit 1; }
  python3 -m json.tool "$root/state.json" >/dev/null
}

write_plan() {
  mode=$1
  python3 - "$mode" "$root/state.json" "$root/plan.json" <<'PY'
import json, os, sys, tempfile
mode, state_path, plan_path = sys.argv[1:]
state = json.load(open(state_path))
tracks = {track["name"]: track for track in state["tracks"]}
plan = {
    "schema_version": 1,
    "plan_id": "live-tidy-smoke",
    "inspected_project": {
        "name": state["project"]["name"],
        "path": state["project"]["path"],
        "project_change_count": state["project"]["project_change_count"],
    },
    "items": [
        {
            "id": "color-vocals-custom",
            "verb": "set_track_color",
            "target": {"track_guid": tracks["Vox 2"]["guid"]},
            "payload": {"color": {"red": 1, "green": 2, "blue": 3}},
            "reason": "live smoke: custom convention changed vocals to rgb(1, 2, 3)",
        },
        {
            "id": "rename-marker-1",
            "verb": "rename_marker",
            "target": {"marker_id": 1, "snapshot_name": "chorus"},
            "payload": {"new_name": "Chorus"},
            "reason": "live smoke: marker Title Case",
        },
        {
            "id": "rename-region-202",
            "verb": "rename_region",
            "target": {"marker_id": 202, "snapshot_name": "chorus 2"},
            "payload": {"new_name": "Chorus 2"},
            "reason": "live smoke: region Title Case",
        },
        {
            "id": "delete-marker-2",
            "verb": "delete_marker",
            "target": {"marker_id": 2, "snapshot_name": "chorus", "snapshot_position_seconds": 10},
            "payload": {"survivor_marker_id": 1},
            "reason": "live smoke: exact duplicate",
        },
    ],
}
if mode == "unknown":
    plan["items"][0]["verb"] = "set_track_volume"
elif mode == "malformed":
    plan["items"][0]["payload"]["volume_db"] = -3
elif mode == "stale":
    plan["items"][0]["target"]["track_guid"] = "{00000000-0000-0000-0000-000000000000}"
    plan["items"][1]["target"]["snapshot_name"] = "not-chorus"
    plan["items"][2]["target"]["snapshot_name"] = "not-region"
    plan["items"][3]["target"]["snapshot_position_seconds"] = 10.001
elif mode != "valid":
    raise SystemExit("unknown plan fixture mode")
fd, temp_path = tempfile.mkstemp(prefix=".plan-", dir=os.path.dirname(plan_path))
with os.fdopen(fd, "w") as handle:
    json.dump(plan, handle, separators=(",", ":"))
    handle.write("\n")
os.chmod(temp_path, 0o600)
os.replace(temp_path, plan_path)
PY
}

"$switcher" open "$fixture" "$previous"
opened=true
inspect
cp "$root/state.json" "$work/baseline.json"

# The read-only mode must not create an action undo/state-change point.
inspect
cmp "$work/baseline.json" "$root/state.json"

# Unknown verbs and malformed payloads are whole-plan refusals. Neither may
# create a result, mutate the project, or change its state count.
for mode in unknown malformed; do
  write_plan "$mode"
  rm -f "$root/apply_result.json"
  status=$(run_lua "$applier")
  case "$status" in "error: tidy prepare:"*) ;; *) echo "$mode plan unexpectedly returned $status" >&2; exit 1;; esac
  [ ! -e "$root/apply_result.json" ]
  inspect
  cmp "$work/baseline.json" "$root/state.json"
done

# A valid but fully stale plan returns one skipped row per reviewed item and no
# empty undo/state-change point. Pre-seeding the destination proves replacement
# exposes a complete new JSON document, never a partially rewritten old one.
write_plan stale
printf 'old result\n' > "$root/apply_result.json"
status=$(run_lua "$applier")
[ "$status" = ok ] || { echo "stale plan status=$status" >&2; exit 1; }
python3 - "$root/apply_result.json" <<'PY'
import json, os, sys
path = sys.argv[1]
result = json.load(open(path))
assert result["project_change_count_before"] == result["project_change_count_after"]
assert len(result["items"]) == 4
assert all(item["status"] == "skipped" and item["reason"] for item in result["items"])
assert os.path.getsize(path) <= 1024 * 1024
PY
[ ! -L "$root/apply_result.json" ]
[ -z "$(find "$root" -maxdepth 1 -name '.apply_result.json.tmp-*' -print -quit)" ]
inspect
cmp "$work/baseline.json" "$root/state.json"

# Apply four cosmetic changes in one runner-owned undo block.
write_plan valid
status=$(run_lua "$applier")
[ "$status" = ok ] || { echo "valid plan status=$status" >&2; exit 1; }
python3 - "$root/apply_result.json" <<'PY'
import json, sys
result = json.load(open(sys.argv[1]))
assert len(result["items"]) == 4
assert all(item["status"] == "applied" for item in result["items"])
print(json.dumps(result, indent=2))
PY
inspect
cp "$root/state.json" "$work/applied.json"
python3 - "$root/apply_result.json" "$work/applied.json" <<'PY'
import json, sys
result, state = (json.load(open(path)) for path in sys.argv[1:])
tracks = {track["name"]: track for track in state["tracks"]}
markers = {(marker["is_region"], marker["id"]): marker for marker in state["markers"]}
assert tracks["Vox 2"]["color"] == {"red": 1, "green": 2, "blue": 3}
assert tracks["DRUM BUS"]["color"] is None
assert markers[(False, 1)]["name"] == "Chorus"
assert markers[(True, 202)]["name"] == "Chorus 2"
assert (False, 2) not in markers
assert markers[(False, 4)]["name"] == "chorus"  # unchecked proposal item stayed untouched
assert result["project_change_count_after"] == state["project"]["project_change_count"]
PY

# The deferred inspector created no intervening undo point. One REAPER undo
# must therefore restore every applied color/rename/deletion together.
curl -fsS -m 8 "http://127.0.0.1:$port/_/40029" >/dev/null
sleep 0.2
inspect
python3 - "$work/baseline.json" "$root/state.json" <<'PY'
import json, sys
before, after = (json.load(open(path)) for path in sys.argv[1:])
for state in (before, after):
    state["project"].pop("project_change_count", None)
    state["project"].pop("save_dirty", None)
assert before == after
PY

[ "$(stat -f %m "$fixture")" = "$fixture_mtime" ]
"$switcher" restore "$fixture" "$previous"
opened=false
echo "live tidy smoke passed: refusal, stale skip, atomic result, four cosmetic changes, one undo, original project restored"
