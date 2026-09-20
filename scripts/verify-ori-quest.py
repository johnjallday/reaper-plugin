#!/usr/bin/env python3
"""Check real Ori quest ownership/version safety in a disposable, disabled installation.

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
                              if q.get("plugin_id") == "reaper-plugin" and q.get("id") == "reaper_setup"]
                    require(len(quests) == 1, "expected one exact REAPER quest")
                    return quests[0]

                def assert_gated(journey):
                    require([s["id"] for s in journey["steps"]] ==
                            ["project", "workspace", "staffing", "summary"],
                            "candidate did not expose the exact four project setup steps")
                    require(not any(s["status"] == "complete" for s in journey["steps"]),
                            "opening setup completed an unreviewed consequence")
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
                preinstall = request("/api/setup-quests")["quests"]
                require(not any(q.get("plugin_id") == "reaper-plugin" and
                                q.get("id") == "reaper_setup" for q in preinstall),
                        "plugin-owned quest appeared before installation")
                print("PASS: empty profile exposes no plugin-owned REAPER setup quest")

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

                fresh = request(root)["setup_journey"]
                require(not fresh.get("declaration_incompatible", False),
                        "fresh quest v3 was unexpectedly incompatible")
                fresh = request(root + "/open", {"if_revision": fresh["state_revision"],
                                                 "idempotency_key": "fresh-v3-open"})["setup_journey"]
                fresh = request(root + "/dismiss", {"if_revision": fresh["state_revision"],
                                                    "idempotency_key": "fresh-v3-dismiss"})["setup_journey"]
                assert_gated(fresh)
                require(fresh.get("dismissed", False), "fresh quest did not retain its dismissal")
                print("PASS: fresh v3 progress opened/dismissed; no Home/project/mode receipt synthesized")

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
