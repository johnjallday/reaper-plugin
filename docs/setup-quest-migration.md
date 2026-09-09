# REAPER setup quest ownership migration

## What moved

`.ori-plugin/plugin.json` now contains the complete `setup_quests[0]` declaration.
`blueprints/reaper-song/template.json` selects it with
`"setup_quest": "reaper_setup"`. This is actual plugin data, not just a host
schema or a test-only authored declaration.

The declaration is unchanged from Ori's frozen compatibility JSON at PR #466,
commit `1846c497`. Schema/version **1**, quest ID `reaper_setup`, integration key
`ori_reaper`, blueprint ID `reaper-song`, and program ID
`music-producer-assistant` remain stable. The five durable step IDs remain
`integration`, `project`, `workspace`, `staffing`, `summary` in that order.

The plugin/CLI/service candidate is **0.5.1** and the blueprint is **v5**. These
release versions are distinct from quest version **1**; copying the declaration
does not require rewriting progress or receipts. Golden regression fixtures
record the original quest and the complete published v0.5.0 blueprint v4
(source `1f494db5`), proving that the quest reference is the template's only
change. The runtime's only Go change is its version identity.

## Ownership is not execution authority

Ori PR #466 supplies the host contract and independent discovery/resume routes.
Ori retains every UI, normalization, reviewed installation, group, path picker,
workspace, wizard, staffing, and permission owner. Quest fields cannot select
arbitrary commands, URLs, adapters, or grants. The plugin's required
`setup_quests_v1` feature makes an older host refuse this contribution.

Opening setup is not approval to install/enable software, create resources,
start REAPER, stage/register a runner, or grant access. File-only mode and exact
project live verification remain unchanged. The workspace `setup_wizard`
continues only after workspace creation. Plugin-owned declarations remain
read-only in Ori; this is not a user-authored quest editor.

The host compatibility copy is intentionally retained for pre-install discovery
and older installed plugins. Once this candidate is installed, its valid
manifest declaration must win and the catalog must report `ownership: plugin`.
The host's invalid-declaration and unsupported-migration behavior remains
fail-closed; compatibility is not an excuse to discard saved progress.

## Deterministic validation (no live REAPER)

Run from this plugin worktree with live-test opt-ins absent:

```sh
env -u ORI_REAPER_LIVE_PROJECT GOWORK=off GOTOOLCHAIN=go1.25.13 go test -race -count=1 ./...
GOWORK=off GOTOOLCHAIN=go1.25.13 go vet ./...
make test-ui
GOWORK=off make artifact-local
GOWORK=off make release-package VERSION=v0.5.1
govulncheck -mode=binary dist/reaper-plugin_v0.5.1_darwin_arm64
```

Packaging builds local files only. Never push a `v*` tag as a test: that triggers
publication. The candidate uses the existing pinned Go 1.25.13 build and is
8,780,098 bytes, SHA-256
`baeca80db6b156c25207784355d3c2a4717169533f02d85d543e4ea7a5dd6633`.
Binary scan results must be checked at release time, not inferred from an older
release's scan.

### Check the real host, not a mock declaration

Build an Ori binary containing PR #466, then run on macOS arm64 (Python 3 and
`lsof` required):

```sh
GOWORK=off ./scripts/with-local-artifact.sh \
  python3 scripts/verify-ori-quest.py /absolute/path/to/ori/bin/ori-agent
```

The existing wrapper temporarily selects this worktree's locally built artifact
and restores the manifest afterward. The verifier does not alter any other
source checkout or installed plugin. It:

1. Starts its own Ori child with a disposable HOME/data directory and port zero.
   It discovers only that child's actual listener with `lsof`; it accepts no
   existing Ori URL and inherits no credentials, live flags, or development
   verification override.
2. Opens/dismisses an inert compatibility root before installing the candidate.
3. Previews and confirms installation only in that disposable profile, leaving
   the plugin **disabled**. Ori's real manifest and blueprint normalizers run.
4. Requires catalog `ownership: plugin`, the exact blueprint quest reference,
   and the same saved root, timestamps, and dismissal state. The explicitly
   confirmed install may update the observed integration/version receipt.
5. Resumes the root and verifies that the local candidate remains
   **unverified**, no group/project/mode receipts appear, and no assistant
   relationship or service enablement is synthesized.
6. Stops its child and removes only the disposable profile, even on failure.

This proves source ownership and compatibility-to-plugin progress reuse, not
published-source trust or live readiness. The script never enables the service
or calls a REAPER operation. Existing host tests cover legacy assistant roots,
children, resource receipts, ambiguous identities, and cross-user isolation.

## Validation record — 2026-09-09

| Check | Result |
| --- | --- |
| Go 1.25.13 `go test -race -count=1 ./...`, live opt-in absent | Both packages passed |
| `go vet ./...` | Passed |
| `make test-ui` | 16 passed |
| `golangci-lint run --new-from-merge-base=origin/main --timeout=3m` | 0 issues |
| Scoped `gosec ./cmd/reaper-plugin/... ./internal/reaper/...` | 0 findings |
| Local `make release-package VERSION=v0.5.1`, twice | Byte-identical; manifest unchanged |
| `govulncheck -mode=binary dist/reaper-plugin_v0.5.1_darwin_arm64` | No vulnerabilities found |
| `with-local-artifact.sh python3 scripts/verify-ori-quest.py <Ori binary>` | All ownership/resume/gate checks passed |

The host check used Ori commit `1846c497` (PR #466). It loaded the actual plugin
candidate and blueprint, not mocked discovery responses. Native dependency
compiler/linker warnings from `go-m1cpu`/macOS were non-failing. No live test was
run. These local results are not remote-artifact or publication evidence.

## Remaining delivery boundaries

The release owner authorized committing and publishing **v0.5.1** on
2026-09-09. The validation above records local preparation, not a claim that
publication or remote-byte verification has already succeeded. The tag workflow
owns publication; its result and remote asset verification must be checked
separately. Installed plugins and Ori's reviewed registry remain unchanged.

Ori PR #466 remains open at preparation, with its README Contract CI check
unresolved. Plugin publication does not merge that PR or make an older Ori
build support `setup_quests_v1`. Host support and the reviewed-pin update remain
required before production rollout.

- Review the plugin source and host support, and record the final immutable
  source commit after merge. A squash merge changes that identity.
- After explicit release approval, use the normal publisher workflow. Verify
  actual remote asset bytes, digest, size, version, tag/source identity, and CI.
- In a separate Ori change, review the new exact source and artifact identity,
  update expected plugin/blueprint versions and registry revision, then test a
  fresh GitHub-backed install and existing-root upgrade **without** a development
  override. Do not point the host at an unpublished candidate or relax its gate.
- Existing v0.5.0 users continue through labeled host compatibility until they
  explicitly update. Do not rewrite their installed declarations or remove the
  compatibility bootstrap as part of source extraction.
