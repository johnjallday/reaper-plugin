#!/bin/sh
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ori_url=${ORI_URL:-http://127.0.0.1:8931}

curl -fsS "$ori_url/health" >/dev/null
body=$(python3 - "$repo_root" <<'PY'
import json,sys
print(json.dumps({'source':sys.argv[1],'confirm':False}))
PY
)
preview=$(curl -fsS -X POST "$ori_url/api/plugins/install" -H 'Content-Type: application/json' --data "$body")
printf '%s' "$preview" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["trust"]["Name"]=="reaper-plugin"; assert d["trust"]["Services"][0]["Transport"]=="mcp_stdio"'
body=$(printf '%s' "$body" | python3 -c 'import json,sys; d=json.load(sys.stdin); d["confirm"]=True; print(json.dumps(d))')
installed=$(curl -fsS -X POST "$ori_url/api/plugins/install" -H 'Content-Type: application/json' --data "$body")
printf '%s' "$installed" | python3 -c 'import json,sys,os; p=json.load(sys.stdin)["plugin"]; a=p["resolved_artifacts"][0]; assert a["available"] and os.path.isfile(a["managed_path"]); assert p["enabled"] is False; assert p["resolved_blueprints"][0]["qualified_id"]=="plugin:reaper-plugin:reaper-song"'
curl -fsS -X DELETE "$ori_url/api/plugins/reaper-plugin" >/dev/null
printf 'Ori confirmed-install dry run passed\n'
