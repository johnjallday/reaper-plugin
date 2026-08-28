# REAPER Project Tidy live spike findings

Observed: 2026-08-28 on REAPER 7.78/macOS-arm64 using the installed trusted
runner and a disposable two-track project in a separate REAPER project tab.
The original project tab was restored after the probe. The disposable tab is
still open in the background so the evidence can be inspected.

Executable probe fixtures and their captured output live together in this
directory. Run `./run-live-probe.sh` only when REAPER Web Remote and the Ori
runner are available and no earlier `Vox 2` spike tab remains open.

## Settled choices

### Marker and region identity

- `EnumProjectMarkers3` uses a zero-based **enumeration index**, but returns a
  separate `markrgnindexnumber`. Plans must persist and target the returned ID,
  never the enumeration index.
- Auto-assigned marker IDs were 1–5. Requested marker 101 and region 202 were
  returned and enumerated unchanged.
- A marker and a region can both have ID 303. The API enumerated both rows and
  `AddProjectMarker2` returned 303 for each. Identity is therefore `(is_region,
  id)`, not `id` alone. The separate rename verbs provide the type; deletion is
  marker-only and must pass `is_region=false`.
- Enumeration was position-ordered in this fixture. Order is presentation only
  and is not stable identity.

Evidence: `observed-setup.json`, `observed-marker-ids.json`.

### Track GUID and color representation

- Two reads of the same track GUID returned the identical braced string
  `{DB7F23BB-38B6-3844-8A87-22FD017EE05B}`. Persist the opaque GUID string and
  compare it exactly; do not derive identity from track index.
- On this macOS build, `ColorToNative(12, 34, 56)` returned `795192`
  (`0x0C2238`). A custom REAPER color is that native value OR'd with
  `0x1000000`, producing `17572408` (`0x10C2238`). Masking with `0xFFFFFF` and
  calling `ColorFromNative` recovered `[12, 34, 56]`; unset color enumerated as
  raw `0` with no custom flag.
- The external inspected-state and plan contracts should carry RGB channels,
  not platform-native packed integers. Canonical Lua converts at the REAPER
  boundary and sets the custom-color flag explicitly.

Evidence: `observed-setup.json`, `observed-read.json`.

### Project state-change counts and runner modes

- While the setup script was inside the current runner's undo block, the change
  count remained 1 across track insertion/naming/coloring, marker creation, and
  save. The count advanced only after the runner closed the block.
- The inspector read calls themselves left the count unchanged (`2 → 2`). The
  old trusted runner action then advanced the count even though the script had
  only read state.
- Removing the explicit undo/refresh calls was necessary but not sufficient:
  REAPER also creates an automatic action undo/state-change point for a
  non-deferred ReaScript. The audited path must schedule one empty
  `reaper.defer(function() end)` to suppress that host behavior, while still
  omitting `Undo_BeginBlock`, `Undo_EndBlock`, `TrackList_AdjustWindows`, and
  `UpdateArrange`.
- With that mode installed, two full canonical inspections and one rejected
  mutating inspector held the live disposable project's count at `18`; both
  successful `state.json` outputs were identical.
- An applier cannot write an authoritative `after` count before the runner has
  closed its undo block. The mutation runner must sample/finalize the result
  after `Undo_EndBlock`; code must not assume the count increases by exactly
  one.

Evidence: `observed-read.json`, `observed-post-runner.json`,
`observed-marker-post.json`, `observed-inspector-state.json`, and
`observed-inspector-repeat.json`.

### Atomic replacement

- After closing both handles, `os.rename(temp, destination)` in the same
  `~/.ori-reaper` subdirectory replaced an existing destination on macOS,
  returned `true`, removed the temp name, and exposed the complete new body.
- Canonical writes should use a unique temp in the destination directory,
  close/check it, and rename it into place. This result settles macOS behavior
  only; any supported non-POSIX platform needs its own replacement test or a
  platform-specific implementation.

Evidence: `observed-setup.json` (`atomic_replace`).

### Duplicate-position tolerance

- Three exact-position markers all enumerated at exactly `10` seconds.
  Separate markers created at +0.5 ms and +1 ms remained distinct and
  enumerated as `10.000500000000001` and `10.000999999999999`.
- A 1 ms dedupe epsilon is **not safe**: REAPER preserves distinct marker
  positions inside that window, so the tolerance could delete an intentional
  marker. v1 will use exact-position duplicates only, matching FR-20.
- Inspector JSON must serialize positions with enough precision for a binary64
  round trip (for example `%.17g`). Delete validation compares the current
  target and named survivor positions to their exact snapshot values; it does
  not apply a musical or millisecond tolerance.

Evidence: `observed-setup.json`, `observed-read.json`.

## Additional observation

The configured Web Remote port was 2307 while the running listener had fallen
back to 2308. The probe followed config first and then the actual REAPER
listener. Portable preflight should keep using the established discovery helper
rather than assuming the configured port is the reachable one.

## Isolation

The spike fixtures and evidence are checked into the isolated plugin worktree at
`scripts/spikes/reaper-project-tidy/` on `feature/reaper-away-mode`, created from
fresh `origin/main`. The unrelated `fix/release-toolchain` checkout remains
untouched.
