#!/usr/bin/env python3
"""Non-creating snapshot of everything the REAPER Studio catalog would read.

Usage: snapshot.py <label> <output-dir>

It records, without creating, opening for write or following anything:
  - whether REAPER is running and whether Web Remote answers (TRANSPORT,
    TRACK, NTRACK), so transport and selection can be compared before/after;
  - each catalog source root (lstat, recursive membership with per-file
    sha256, symlinks never followed) and aggregate membership/content hashes;
  - reaper-kb.ini SCR rows (the registration source) and the plugin cache files;
  - the runner exchange under ~/.ori-reaper (runner.id, last_status, inbox hash).

Only <output-dir>/<label>.json is written; the directory must already exist.
"""

import hashlib
import json
import os
import plistlib
import re
import shlex
import stat
import subprocess
import sys
import time
import urllib.request

HOME = os.path.expanduser("~")
RESOURCE = os.path.join(HOME, "Library", "Application Support", "REAPER")
ORI_SCRIPTS = os.path.join(HOME, "Ori Scripts", "reaper")
RUNNER_ROOT = os.path.join(HOME, ".ori-reaper")
APP = "/Applications/REAPER.app"
MAX_HASH_BYTES = 8 << 20
MAX_ENTRIES = 5000


def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        while True:
            chunk = handle.read(1 << 16)
            if not chunk:
                break
            digest.update(chunk)
    return digest.hexdigest()


def lstat_info(path):
    try:
        info = os.lstat(path)
    except FileNotFoundError:
        return {"exists": False}
    except PermissionError:
        return {"exists": True, "denied": True}
    kind = (
        "symlink" if stat.S_ISLNK(info.st_mode)
        else "dir" if stat.S_ISDIR(info.st_mode)
        else "file" if stat.S_ISREG(info.st_mode)
        else "other"
    )
    record = {
        "exists": True, "kind": kind, "dev": info.st_dev, "ino": info.st_ino,
        "mode": oct(stat.S_IMODE(info.st_mode)), "size": info.st_size,
        "mtime": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(info.st_mtime)),
    }
    if kind == "symlink":
        record["target"] = os.readlink(path)
    if kind == "file" and info.st_size <= MAX_HASH_BYTES:
        try:
            record["sha256"] = sha256_file(path)
        except OSError as error:
            record["sha256_error"] = error.__class__.__name__
    return record


def walk_source(root):
    summary = lstat_info(root)
    if not summary.get("exists") or summary.get("kind") != "dir":
        return {"root": summary, "entries": [], "counts": {}}
    entries = []
    counts = {"file": 0, "dir": 0, "symlink": 0, "other": 0, "hidden": 0, "unreadable": 0, "beyond_cap": 0}
    max_depth = 0

    def visit(directory, relative, depth):
        nonlocal max_depth
        max_depth = max(max_depth, depth)
        try:
            with os.scandir(directory) as iterator:
                children = sorted(iterator, key=lambda entry: entry.name)
        except PermissionError:
            counts["unreadable"] += 1
            entries.append({"path": relative or ".", "kind": "dir", "unreadable": True})
            return
        for child in children:
            rel = child.name if not relative else relative + "/" + child.name
            if len(entries) >= MAX_ENTRIES:
                counts["beyond_cap"] += 1
                continue
            if child.name.startswith("."):
                counts["hidden"] += 1
            record = lstat_info(child.path)
            record["path"] = rel
            entries.append(record)
            counts[record.get("kind", "other")] += 1
            if record.get("kind") == "dir":
                visit(child.path, rel, depth + 1)

    visit(root, "", 1)
    membership = hashlib.sha256()
    content = hashlib.sha256()
    for record in sorted(entries, key=lambda item: item["path"]):
        membership.update((record["path"] + "\0" + record.get("kind", "") + "\n").encode("utf-8"))
        content.update((record["path"] + "\0" + record.get("sha256", "") + "\n").encode("utf-8"))
    counts["max_depth"] = max_depth
    return {
        "root": summary, "entries": entries, "counts": counts,
        "membership_sha256": membership.hexdigest(), "content_sha256": content.hexdigest(),
    }


def keyboard_rows(path):
    record = lstat_info(path)
    rows = []
    if record.get("kind") == "file":
        with open(path, "r", encoding="utf-8", errors="replace") as handle:
            for line in handle:
                line = line.rstrip("\r\n")
                if not line.startswith("SCR "):
                    continue
                try:
                    fields = shlex.split(line)
                except ValueError:
                    rows.append({"raw": line, "malformed": True})
                    continue
                rows.append({
                    "fields": len(fields),
                    "flags": fields[1] if len(fields) > 1 else None,
                    "section": fields[2] if len(fields) > 2 else None,
                    "command_id": fields[3] if len(fields) > 3 else None,
                    "description": fields[4] if len(fields) > 4 else None,
                    "path": fields[5] if len(fields) > 5 else None,
                    "path_is_absolute": fields[5].startswith("/") if len(fields) > 5 else None,
                })
    record["scr_rows"] = rows
    return record


def plugin_caches():
    names = [
        "reaper-vstplugins_arm64.ini", "reaper-vstplugins64.ini", "reaper-vstshells_arm64.ini",
        "reaper-vstrenames_arm64.ini", "reaper-auplugins_arm64.ini", "reaper-auplugins64.ini",
        "reaper-clap-macos-aarch64.ini", "reaper-clap-rename-macos-aarch64.ini", "reaper-jsfx.ini",
        "reaper-lv2plugins.ini", "reaper-fxtags.ini", "reaper-fxfolders.ini", "reaper-vstfxppath.ini",
    ]
    caches = {}
    for name in names:
        path = os.path.join(RESOURCE, name)
        record = lstat_info(path)
        if record.get("kind") == "file":
            with open(path, "r", encoding="utf-8", errors="replace") as handle:
                lines = handle.read().split("\n")
            record["lines"] = len(lines)
            record["sections"] = [line for line in lines if line.startswith("[")][:12]
            record["section_count"] = sum(1 for line in lines if line.startswith("["))
            record["vsti_marker_rows"] = sum(1 for line in lines if line.endswith("!!!VSTi"))
            record["assignment_rows"] = sum(1 for line in lines if "=" in line and not line.startswith("["))
            record["max_line_bytes"] = max((len(line.encode("utf-8")) for line in lines), default=0)
        caches[name] = record
    return caches


def web_remote():
    port = None
    ini = os.path.join(RESOURCE, "reaper.ini")
    try:
        with open(ini, "r", encoding="utf-8", errors="replace") as handle:
            for line in handle:
                match = re.match(r"^csurf_\d+=HTTP\s+\S+\s+(\d+)", line.strip())
                if match:
                    port = int(match.group(1))
                    break
    except OSError:
        pass
    result = {"configured_port": port, "reachable": False}
    if port is None:
        return result
    for command in ("TRANSPORT", "TRACK", "NTRACK"):
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{port}/_/{command}", timeout=2) as response:
                body = response.read(64 << 10).decode("utf-8", errors="replace")
        except Exception as error:  # noqa: BLE001 -- offline is a normal observation
            result["error"] = error.__class__.__name__
            return result
        result["reachable"] = True
        result[command] = body
        if command == "TRACK":
            selected = []
            for row in body.split("\n"):
                fields = row.split("\t")
                if len(fields) > 3 and fields[0] == "TRACK":
                    try:
                        flags = int(fields[3])
                    except ValueError:
                        continue
                    if flags & 2:
                        selected.append(fields[1])
            result["selected_track_indices"] = selected
    return result


def process_state():
    try:
        output = subprocess.run(["/usr/bin/pgrep", "-x", "REAPER"], capture_output=True, text=True, timeout=5)
        pids = [line for line in output.stdout.split() if line.strip()]
    except Exception as error:  # noqa: BLE001
        return {"error": error.__class__.__name__}
    return {"running": bool(pids), "pid_count": len(pids)}


def app_state():
    record = lstat_info(APP)
    plist = os.path.join(APP, "Contents", "Info.plist")
    if record.get("kind") == "dir" and os.path.isfile(plist):
        with open(plist, "rb") as handle:
            info = plistlib.load(handle)
        record["short_version"] = info.get("CFBundleShortVersionString")
        record["bundle_version"] = info.get("CFBundleVersion")
    record["portable_ini_beside_app"] = lstat_info("/Applications/reaper.ini").get("exists", False)
    return record


def runner_exchange():
    record = lstat_info(RUNNER_ROOT)
    if record.get("kind") != "dir":
        return record
    names = sorted(os.listdir(RUNNER_ROOT))
    record["entries"] = names
    for name in ("runner.id", "last_status.txt", "inbox.lua", "runner.version"):
        path = os.path.join(RUNNER_ROOT, name)
        item = lstat_info(path)
        if item.get("kind") == "file" and name != "inbox.lua" and item["size"] <= 4096:
            with open(path, "r", encoding="utf-8", errors="replace") as handle:
                item["content"] = handle.read()
        record[name] = item
    return record


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: snapshot.py <label> <output-dir>")
    label, output_dir = sys.argv[1], sys.argv[2]
    snapshot = {
        "label": label,
        "taken_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "app": app_state(),
        "process": process_state(),
        "web_remote": web_remote(),
        "resource_folder": lstat_info(RESOURCE),
        "sources": {
            "project_templates": walk_source(os.path.join(RESOURCE, "ProjectTemplates")),
            "track_templates": walk_source(os.path.join(RESOURCE, "TrackTemplates")),
            "reaper_scripts": walk_source(os.path.join(RESOURCE, "Scripts")),
            "ori_scripts": walk_source(ORI_SCRIPTS),
        },
        "keyboard_config": keyboard_rows(os.path.join(RESOURCE, "reaper-kb.ini")),
        "plugin_caches": plugin_caches(),
        "reapack": {
            "reapack_ini": lstat_info(os.path.join(RESOURCE, "reapack.ini")),
            "registry_db": lstat_info(os.path.join(RESOURCE, "ReaPack", "registry.db")),
        },
        "runner_exchange": runner_exchange(),
    }
    target = os.path.join(output_dir, label + ".json")
    with open(target, "w", encoding="utf-8") as handle:
        json.dump(snapshot, handle, indent=1, sort_keys=True)
        handle.write("\n")
    print(target)


if __name__ == "__main__":
    main()
