# Release notes

## 0.3.0 — Workspace Surface

This release adds the optional Ori Workspace Surface adapter while preserving
the existing Claude/Codex skills and helper CLI.

### Added

- Workspace Surface protocol v1 manifest and MCP stdio private service.
- Sandboxed Live Control station/modal with one-second visible polling.
- Live state, transport/actions, registered and global scripts, host-reviewed
  script proposals, namespaced pinned actions, guarded track edits and specific
  Undo, and all-or-nothing bulk plans.
- Plugin-owned Reaper Song blueprint, runtime/setup provider, agent operations,
  starter tasks, project seed, and explicit project-file fallback declaration.
- Reproducible macOS arm64 build, digest verifier, clean Ori install verifier,
  race/UI/security CI, and an opt-in disposable live-project parity test.

### Compatibility and trust

- Plugin version: `0.3.0`
- Workspace Surface protocol: `1`
- Minimum Ori protocol: `1`
- Service artifact: macOS arm64 only
- Release artifact size: `8,476,866` bytes
- Release SHA-256: `06fdf6623a17737cbad38241584e198f2061956127b29efb3dc4219365ecfa93`

The service is trusted native code. Ori keeps browser code in an opaque-origin
sandbox and mediates every UI/agent operation through workspace ownership,
attachment, schema, grant, policy, confirmation, timeout, output, and redaction
checks. Unsupported platforms remain unavailable and never launch another
artifact.

Legacy workspaces from Ori's retired compiled Reaper Song template are not
migrated or automatically attached. No pins, grants, setup history, provenance,
tasks, or project metadata are imported. The supported path is a new workspace
created from this plugin's blueprint.

The release asset is selected by exact platform and downloaded over HTTPS;
Ori verifies this digest before making the managed copy executable.
