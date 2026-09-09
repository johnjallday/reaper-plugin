#!/usr/bin/env python3
"""Check real Ori quest ownership/resume in a disposable, disabled installation.

Run through scripts/with-local-artifact.sh. Requires a quest-capable Ori binary
and lsof. Never uses an existing Ori URL/profile, enables the service, sets the
reviewed-integration development override, or contacts REAPER.
"""

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def wait_for_server(process, lsof):
    # Let Ori bind port zero, then discover only this child process's listener.
    # Never race to claim a supposedly free port or contact another Ori instance.
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        require(process.poll() is None, "isolated Ori exited before becoming ready")
        result = subprocess.run(
            [lsof, "-nP", "-a", "-p", str(process.pid), "-iTCP", "-sTCP:LISTEN", "-Fn"],
            capture_output=True, text=True, check=False, timeout=5,
        )
        ports = {
            int(line.rsplit(":", 1)[1])
            for line in result.stdout.splitlines()
            if line.startswith("n") and line.rsplit(":", 1)[-1].isdigit()
        }
        require(len(ports) <= 1, "isolated Ori exposes ambiguous listeners")
        if ports:
            return "http://127.0.0.1:" + str(ports.pop())
        time.sleep(0.2)
    raise RuntimeError("isolated Ori did not open its listener")


def verify(binary, repo, manifest, lsof):
    with tempfile.TemporaryDirectory(prefix="reaper-quest-check-") as directory:
        sandbox = Path(directory)
        home = sandbox / "home"
        home.mkdir(mode=0o750)
        log_path = sandbox / "ori.log"
        with log_path.open("w") as log:
            # Do not inherit credentials, live-test flags, REAPER_PLUGIN_HOME,
            # provider configuration, or ORI_REVIEWED_INTEGRATION_DEV_SOURCE.
            process = subprocess.Popen(
                [str(binary), "-port", "0", "-no-browser", "-shutdown-timeout", "2s"],
                cwd=sandbox, stdout=log, stderr=subprocess.STDOUT,
                env={"PATH": os.environ.get("PATH", os.defpath), "HOME": str(home),
                     "ORI_DATA_DIR": str(sandbox / "data"), "PORT": "0"},
            )
            try:
                base = wait_for_server(process, lsof)
                client = urllib.request.build_opener(urllib.request.ProxyHandler({}))

                def request(path, body=None):
                    require(process.poll() is None, "isolated Ori is no longer running")
                    data = None if body is None else json.dumps(body).encode()
                    req = urllib.request.Request(base + path, data=data,
                                                 headers={"Content-Type": "application/json"})
                    with client.open(req, timeout=90) as response:
                        return json.load(response)

                root = "/api/setup-quests/reaper-plugin/reaper_setup"

                def catalog_quest():
                    quests = [q for q in request("/api/setup-quests")["quests"]
                              if q["plugin_id"] == "reaper-plugin" and q["id"] == "reaper_setup"]
                    require(len(quests) == 1, "expected one exact REAPER quest")
                    return quests[0]

                def assert_gated(journey):
                    integration = next(s for s in journey["steps"] if s["id"] == "integration")
                    require(integration["status"] != "complete", "unreviewed candidate passed installation")
                    require(not integration.get("integration", {}).get("verified", False),
                            "candidate incorrectly became release-verified")
                    # The explicitly confirmed install may record its observed
                    # plugin/version. It must not create a group, project or mode.
                    receipts = journey["receipts"]
                    require(set(receipts) <= {"integration_plugin_id", "integration_version"},
                            "discovery created a group/project/mode receipt")
                    for key, value in receipts.items():
                        expected = {"integration_plugin_id": "reaper-plugin",
                                    "integration_version": manifest["version"]}[key]
                        require(value == expected, "unexpected installation receipt " + key)

                require(not request("/api/plugins")["plugins"], "test profile was not empty")
                require(catalog_quest()["ownership"] == "host_compatibility", "bootstrap was not compatibility data")
                before = request(root)["setup_journey"]
                before = request(root + "/open", {"if_revision": before["state_revision"],
                                                  "idempotency_key": "migration-open"})["setup_journey"]
                before = request(root + "/dismiss", {"if_revision": before["state_revision"],
                                                     "idempotency_key": "migration-dismiss"})["setup_journey"]
                assert_gated(before)
                print("PASS: compatibility progress opened/dismissed without resource creation")

                preview = request("/api/plugins/install", {"source": str(repo), "confirm": False})
                require(preview["trust"]["Name"] == "reaper-plugin", "wrong preview owner")
                installed = request("/api/plugins/install", {"source": str(repo), "confirm": True})["plugin"]
                require(installed["version"] == manifest["version"], "wrong candidate version")
                require(installed["enabled"] is False, "installation enabled the native service")
                blueprint = installed["resolved_blueprints"][0]
                require(blueprint["qualified_id"] == "plugin:reaper-plugin:reaper-song", "foreign blueprint")
                require(blueprint["template"]["setup_quest"] == "reaper_setup", "quest reference was dropped")
                quest = catalog_quest()
                require(quest["ownership"] == "plugin", "Ori fell back to its own declaration")
                require(quest["title"] == manifest["setup_quests"][0]["title"], "wrong declaration copy")
                print("PASS: real plugin manifest/blueprint resolved; catalog ownership=plugin")

                after = request(root)["setup_journey"]
                for key in ("run_id", "root_run_id", "first_opened_at", "last_dismissed_at", "dismissed"):
                    require(after.get(key) == before.get(key), "migration changed saved " + key)
                require(not after.get("declaration_incompatible", False), "unchanged quest needs a migration")
                assert_gated(after)
                resumed = request(root + "/open", {"if_revision": after["state_revision"],
                                                   "idempotency_key": "migration-resume"})["setup_journey"]
                require(resumed["run_id"] == before["run_id"] and not resumed["dismissed"], "resume duplicated progress")
                require(resumed["first_opened_at"] == before["first_opened_at"], "resume rewrote first-open time")
                assert_gated(resumed)
                print("PASS: same progress/timestamps resumed; no group/project/mode receipts; reviewed-install gate intact")

                try:
                    request("/api/personal-assistant/setup-journey")
                except urllib.error.HTTPError as error:
                    require(error.code == 409, "unexpected assistant-alias error")
                else:
                    raise RuntimeError("quest synthesized an accepted assistant relationship")
                plugins = request("/api/plugins")["plugins"]
                require(len(plugins) == 1 and plugins[0]["enabled"] is False, "service became enabled")
                print("PASS: no assistant acceptance or plugin enablement; no REAPER access requested")
            except Exception:
                log.flush()
                print(log_path.read_text()[-10000:])
                raise
            finally:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)
    print("PASS: isolated Ori stopped and disposable profile removed")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("ori_binary", type=Path)
    args = parser.parse_args()
    binary = args.ori_binary.resolve(strict=True)
    require(binary.is_file() and os.access(binary, os.X_OK), "Ori binary is not executable")
    lsof = shutil.which("lsof")
    require(lsof, "lsof is required to identify the isolated server listener")
    repo = Path(__file__).resolve().parent.parent
    manifest = json.loads((repo / ".ori-plugin/plugin.json").read_text())
    require(all(a["source"]["kind"] == "bundled" for s in manifest["services"] for a in s["artifacts"]),
            "run via scripts/with-local-artifact.sh; this test never downloads a release")
    verify(binary, repo, manifest, lsof)


if __name__ == "__main__":
    main()
