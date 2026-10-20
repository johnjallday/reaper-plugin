# REAPER Studio inventory spike findings

Observed: 2026-10-21 on REAPER **7.80/macOS-arm64** (`/Applications/REAPER.app`,
per-user resource folder, no `reaper.ini` beside the bundle) with the installed
0.10.1 Ori runner. REAPER was closed before the spike, launched for the live
probes, and quit again at the end with nothing dirty. No source was created,
written, opened for write or followed; no cataloged script was executed, no FX
loaded, nothing installed, staged or registered. The probes in this directory
and the raw captures (kept out of the repository under `tasks/evidence-reaper-studio/`)
are the evidence.

Probe files: `snapshot.py` (non-creating file/transport/selection snapshot),
`state-probe.lua` (audited no-undo path, 0.10.1 allowlist only),
`fx-audit-probe.lua` (audited path asking for `EnumInstalledFX`),
`fx-enum-probe.lua` (ordinary inbox path, for format evidence only),
`run-live-probe.sh` (stages one probe in the runner inbox and collects the outcome).

## Decision for contract §4.4: the audio-plugin source is **live**

`audio_plugins` has `access: "live"`, read through `EnumInstalledFX` in a new
fixed read-only inspector entry. A closed REAPER reports `app_closed`.

Why not the cache files:

- **Coverage depends on scan history.** With REAPER closed there was **no AU
  cache at all** (`reaper-auplugins_arm64.ini` did not exist), while the live
  list reported 362 AU entries. The AU cache appeared only when this REAPER
  session exited (16,260 bytes, 392 rows). `reaper-fxtags.ini` was rewritten at
  exit too; `reaper.ini` as always. A file reader therefore sees what the last
  session happened to write, and races REAPER's own rewrite at exit.
- **Four undocumented formats.** `reaper-vstplugins_arm64.ini` (`[vstcache]`,
  `file=hex,id{classid,Display Name (Vendor) (N out)!!!VSTi`, 1,118 rows, two
  sections), `reaper-clap-macos-aarch64.ini` (one `[bundle.clap]` section per
  bundle, `id=0|Name (Vendor)`), `reaper-jsfx.ini` (`VERSION 1` then
  `NAME path "JS: Name (Vendor)"` lines), `reaper-auplugins_arm64.ini`
  (`[auplugins]`, `Vendor: Name=<inst>` or `<!inst>`). Cache keys mangle file
  names (`WaveShell1-VST3 16.7.vst3` is `WaveShell1_VST3_16.7.vst3`), so they do
  not even agree with the live identifiers.
- **One live shape.** `EnumInstalledFX(index)` returned `retval, name, ident`
  for 1,453 entries in 0.30 s of CPU (0.008 s on the second, cached run).
  Prefix counts: AU 342, AUi 20, CLAP 7, JS 224, VST 509, VST3 316, VST3i 18,
  VSTi 15, and 2 entries without a prefix (`Video processor`, `Container`).
  Longest name 75 bytes, longest ident 110 bytes, no control or non-ASCII
  characters, no duplicate ident, one duplicate display name (two UAD builds of
  the same plugin at different paths).

## Live rows: observed shapes and the mapping to the contract

| Prefix | `name` example | `ident` example | ident shape |
| --- | --- | --- | --- |
| `VST3:` | `VST3: ANIMATE (Mastering the Mix)` | `/Library/Audio/Plug-Ins/VST3/ANIMATE.vst3` | absolute bundle path |
| `VST3:` (shell) | `VST3: Aphex Vintage Exciter Mono (Waves)` | `/Library/Audio/Plug-Ins/VST3/WaveShell1-VST3 16.7.vst3<1189814795` | absolute path `<` shell id |
| `VST3i:` | `VST3i: Battery 4 (Native Instruments) (64 out)` | `/Library/Audio/Plug-Ins/VST3/Battery 4.vst3` | absolute bundle path |
| `VSTi:` | `VSTi: FM8 (x86_64) (Native Instruments GmbH)` | `/Library/Audio/Plug-Ins/VST/FM8.vst` | absolute bundle path |
| `AU:` / `AUi:` | `AU: AUDelay (Apple)` | `Apple: AUDelay` | `Manufacturer: Name` |
| `CLAP:` | `CLAP: Pro-Q 4 (FabFilter)` | `com.FabFilter.Pro-Q.4` | reverse-DNS id |
| `JS:` | `JS: Pitch Shifter 2` | `pitch/superpitch` | path relative to `Effects` |
| none | `Video processor` | `Video processor` | built-in name |

Mapping (implemented in group 5):

- `kind`: prefix ending in `i` (`AUi`, `VSTi`, `VST3i`, `CLAPi`) → `instrument`,
  everything else `effect`.
- `format` fact: the prefix without its trailing `i` (`VST`, `VST3`, `AU`,
  `CLAP`, `JS`); omitted for the two unprefixed built-ins.
- `vendor` fact: the last parenthesised group of the display text that is not a
  channel or architecture descriptor. Descriptors seen: `(mono)` ×208,
  `(x86_64)` ×34, `(32 out)`, `(4 out)`, `(64 out)`, `(18 out)`, `(2->16ch)`,
  `(16ch)`, `(8ch)`. 202 rows have no vendor group (JS without `(Cockos)`, the
  built-ins); the fact is omitted there and Ori renders "Unknown".
- `name`: the display text after the prefix with the vendor group removed, so
  `Battery 4 (64 out)`, `Bite (x86_64)`, `Pro-Q 3`, `Pitch Shifter 2`.
- `native_id`: the contract forbids absolute paths, and 858 of the 1,453 idents
  are absolute. The id is `<FORMAT>:<REAPER identifier without its folder>`:
  `VST3:ANIMATE.vst3`, `VST3:WaveShell1-VST3 16.7.vst3<1189814795`,
  `VST:Battery 4.vst`, `AU:Apple: AUDelay`, `CLAP:com.FabFilter.Pro-Q.4`,
  `JS:pitch/superpitch`, `REAPER:Video processor`. REAPER's own `[vstcache]`
  is keyed by file name, so dropping the folder loses nothing REAPER itself
  keeps apart; there were no base-name collisions here, and a collision is
  reported as one skipped `unsupported_name` rather than a duplicate id.

## Runner paths: what each one does to the open project

Measured on a new, unsaved project tab (`project_path` `""`, 0 tracks) opened
beside the project REAPER restored at launch:

| Step | path | change count | dirty | undo label |
| --- | --- | --- | --- | --- |
| state probe (allowlisted reads) | audited no-undo | 1 | 0 | — |
| `fx-audit-probe.lua` | audited no-undo | rejected before running | | |
| `fx-enum-probe.lua`, run 1 | ordinary inbox | 1 → **2** | 0 | `""` at start |
| state probe | audited no-undo | 2 | 0 | |
| `fx-enum-probe.lua`, run 2 | ordinary inbox | 2 → **3** | 0 | `""` at start |
| state probe | audited no-undo | 3 | 0 | |

- The ordinary (undo-block-wrapped) path advanced `GetProjectStateChangeCount`
  by one per run although the script only read; `Undo_CanUndo2` stayed empty
  and the dirty flag stayed 0. The audited no-undo path left the count unchanged
  across every run. The product reader therefore goes through
  `RunReadOnlyInspection` with `EnumInstalledFX` added to both allowlists, and
  never through generic draft execution (brief item 4).
- Transport (`TRANSPORT 0 0.000000 0 1.1.00 1.1.00`) and selection (master
  only, nothing selected) were identical before and after every probe.
- The restored saved project was untouched: change count 2 and dirty 0 before
  the new tab was opened and after it was closed. REAPER quit without a prompt.

## Reader availability: `reader_outdated` and `reader_missing`

- The installed 0.10.1 runner answered the audited `EnumInstalledFX` request
  with `error: read-only audit: API EnumInstalledFX is not read-only` and did
  not run the body. That exact status is the `reader_outdated` signal.
- `~/.ori-reaper/runner.id` was **stale**: it held a command ID REAPER no longer
  knew, Web Remote still answered HTTP 200 and nothing ran (no
  `last_status.txt` within 10 s). The ID in `reaper-kb.ini`'s `SCR` row for
  `ori-reaper-runner.lua` worked, and the runner then rewrote `runner.id`. So a
  present `runner.id` does not prove a reachable reader; a trigger that produces
  no status within the runner timeout is `reader_missing`.
- Design for 0.11.0: the runner writes `~/.ori-reaper/runner.version` on every
  run; the Go probe reports `reader_missing` without a valid `runner.id`,
  `reader_outdated` when `runner.version` is absent (a 0.10.1 runner never
  writes it) or below the version that allowlists `EnumInstalledFX`, and maps
  the audit error above to `reader_outdated` and a trigger timeout to
  `reader_missing` at inventory time. Nothing is installed or staged by the
  catalog; the existing `runtime.repair` / `install-runner` path stays the way
  to update the file in place (the registered action re-reads it).
- Audit gotcha for inspector authors: the allowlist regex treats `reaper.`
  followed by whitespace and a word as an API call, even inside a comment. A
  comment ending in `~/.ori-reaper.` made the next line's `local` read as
  `reaper.local` and was rejected. Keep that spelling out of inspector scripts.

## File sources

Snapshots with REAPER closed, launched, after the probes and closed again gave
identical membership and content hashes for all four source roots; launch,
probes and quit changed no template or script file.

| Source | state | observed |
| --- | --- | --- |
| `ProjectTemplates` | real dir | `Default.RPP` (REAPER 7.55 header), 1 file |
| `TrackTemplates` | real dir | empty |
| `Scripts` | real dir | 455 files in 3 subfolders, depth 3: 6 scripts (`.lua`), 449 theme-adjuster PNGs, 1 hidden `.DS_Store`, 0 symlinks |
| `~/Ori Scripts/reaper` | real dir | 1 `.lua` with the Ori `--[[ori … ]]--` frontmatter |

`reaper-kb.ini` (559 bytes, written 2026-06-22, unchanged through launch,
probes and quit): 5 `SCR` rows of the form
`SCR <flags> <section> <RS-id> "<description>" <path>`, every path **relative to
the Scripts folder** (`Cockos/lyrics.lua`, `ori-reaper-runner.lua`), one script
registered twice in sections `0` (Main) and `32060` (MIDI editor) with distinct
IDs. REAPER writes the file only when the action list changes, so it can lag the
live list; the registration fact is reported from this inspected file with the
observation time, `Yes` when a row's path resolves to the entry, `No` when the
file was read and none does, omitted when the file is absent, unreadable or over
the 4 MiB bound.

Package origin: no ReaPack on this machine (`reapack.ini` and
`ReaPack/registry.db` absent), so every script is `package` Unknown here.
ReaPack's installed-file index is `ReaPack/registry.db` (SQLite, not readable
without a driver this binary does not carry); its repository list is the
`[remotes]` section of `reapack.ini`, and it installs scripts under
`Scripts/<repository name>/…`. v1 reports `package` only when a script's
top-level folder equals an enabled remote name in `reapack.ini`; otherwise it is
omitted. This stays an assumption until a ReaPack machine is observed.

Script descriptions: none of the six REAPER scripts carries a ReaPack
`@description`; the Ori library script carries `description:` in its
frontmatter. v1 reads at most 8 KiB and reports only tagged headers (ReaPack
`@description`, Ori `description:`, EEL `desc:`) as "Script header"; untagged
first comments are not promoted to descriptions.

## Caps and timing to carry into the implementation

- Live list: 1,453 entries, 0.30 s CPU per full enumeration, idents ≤ 110 bytes,
  names ≤ 75 bytes. Stateless per-page re-enumeration (6 pages of 256) is cheap.
- Web Remote on the configured port 2307 answered 3 s after launch; the runner
  round trip for a full enumeration was 3 s wall clock including the 50 ms poll.
- AU cache rows carry `<inst>` / `<!inst>`; the vstcache marks instruments with
  a trailing `!!!VSTi`. Neither is read by the product; recorded for the owner.

## Isolation

Fixtures and this record live in the plugin worktree on `feature/reaper-studio`
under `scripts/spikes/reaper-studio/`. The runner exchange keeps its usual
files; the spike's scratch folder `~/.ori-reaper/reaper-studio-spike/` holds
only the probe outputs. The user's REAPER configuration was not modified by the
spike; REAPER itself rewrote `reaper.ini`, `reaper-fxtags.ini` and created
`reaper-auplugins_arm64.ini` when it quit, as it does after every session.
