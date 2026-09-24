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

## 0.6.0 addendum: install step moved to Ori

This addendum supersedes the five-step contract above for 0.6.0. The sections
above remain the record of the 0.5.1 extraction.

### What changed

Ori's `setup-quest-install-split` feature splits guided setup in two:

1. **Install quest, owned by Ori.** Ori generates `install_ori_reaper` from its
   reviewed-integration registry: install the plugin, then a ready summary that
   continues into this plugin's quest. It is the only guidance shown before the
   plugin is installed.
2. **Setup quest, owned by this plugin.** `reaper_setup` version **2** declares
   four steps in the `project_setup` order: `project`, `workspace`, `staffing`,
   `summary`. It is listed only once the plugin is installed, and Ori re-checks
   the integration on every read. If the plugin is later disabled or removed,
   the quest shows the integration problem and offers the install quest.

The `integration` step and the `runtime_title` and `runtime_instructions`
launch fields are removed. Every other field, step ID and display string is the
v1 declaration unchanged; `TestPluginOwnsFourStepSetupQuestV2` derives the
expected declaration from the frozen v1 fixture to prove that.

The Web Remote preparation screen is gone from Ori, along with its
acknowledgement gate. The workspace `setup_wizard` `runtime_readiness` step is
the one place live control is set up and verified.

### Compatibility

- The manifest requires `setup_quests_v2` and no longer declares
  `setup_quests_v1`. An Ori build without `setup_quests_v2` refuses 0.6.0.
- An Ori build with `setup_quests_v2` retires `setup_quests_v1` and the host
  compatibility copy, so installed 0.5.x plugins fail closed there until
  updated.
- Quest progress does not migrate from version 1 to version 2: the step count
  changes, and Ori's compiled migrations require equal step counts. Ori offers
  "Start over", which deletes only setup progress. Groups, projects, teams and
  plugins are untouched, so those steps read as complete on the fresh run.

### Validation record — 2026-09-15

Run from this plugin checkout with live-test opt-ins absent:

| Check | Result |
| --- | --- |
| `env -u ORI_REAPER_LIVE_PROJECT GOWORK=off go test -race -count=1 ./...` | Both packages passed |
| `GOWORK=off go vet ./...` | Passed |
| `make test-ui` | 16 passed |
| `GOWORK=off make artifact-local` | 8,780,098 bytes, SHA-256 `4def4fec14ecf083b0358c686c608514d4b9afff99dd810f1184213312770119` |
| `scripts/verify-artifact.sh` | Digest, size, mode and CLI version `0.6.0` verified |

`scripts/verify-ori-quest.py` checks the 0.5.1 compatibility-to-plugin root
reuse, which a `setup_quests_v2` host no longer has; it was not run for 0.6.0.
No `v*` tag was created or pushed and no release package was published. Remote
asset verification, Ori's reviewed pin update to 0.6.0 and blueprint 7, and live
REAPER validation remain separate steps.

## 0.8.0 addendum: Home ownership moved to Music Project Management

Quest schema 1 advances from declaration version 2 to **3**. The four durable
step IDs and kinds stay `project`, `workspace`, `staffing`, `summary`; only the
staffing title and description change. They now name the project-local Producer,
Mix Engineer, and Songwriter and state that Music Production Home roles are
staffed separately by Music Project Management.

The blueprint change is intentionally larger and separately versioned as v9:
REAPER's combined `assistant_program` is replaced by project-only
`assistant_project` `reaper-song-team`, which references the independent Home.
This is fresh-setup ownership separation, not migration of an existing Home or
quest receipt. Ori has no compiled version-2-to-3 quest migration, so a saved
older quest is shown as incompatible until the user explicitly chooses the
existing Start over action. That action resets setup progress only; plugin,
Home, workspace, project, root, team, task, and external `.rpp` data remain.

`scripts/verify-ori-quest.py` checks a fresh version-3 quest against the real
host in disposable state. It verifies that the plugin-owned quest appears only
after installation, opens and dismisses without creating a Home, project, mode,
role, assistant acceptance, capability, or REAPER access, and leaves the native
service disabled. Existing-version incompatibility remains covered by Ori's
setup-journey migration tests. Combined candidate acceptance is owned by Ori's
isolated music/REAPER demo, not by this quest check.

Release order is a compatible Ori host, then the independently published Music
Project Management package, then REAPER 0.8.0, followed only later by a reviewed
registry/floor update based on verified published bytes. Local candidates are
not release evidence. No live REAPER check is implied by manifest, quest, or
browser acceptance.

## 0.9.0 addendum: one REAPER Assistant per project

Quest schema 1 advances from declaration version 3 to **4**. The four durable
step IDs and kinds stay `project`, `workspace`, `staffing`, `summary`; only the
staffing title and description change. They now name the single project-local
REAPER Assistant and still state that Music Production Home roles are staffed
separately by Music Project Management.

The blueprint change is separately versioned as v10: `reaper-song-team` declares
one required, primary role, `reaper-assistant`, in place of Producer, Mix
Engineer, and Songwriter, and `standalone_composition` carries that one role's
Home-free prompt. The team ID, schema, and version 1 are unchanged, so the
published Music Project Management v0.1.0 Home continues to authorize the
attachment without a Home release. This is fresh-setup staffing, not migration:
Ori binds an existing linked project to its recorded plugin version and team
digest, so projects created from v9 keep their recorded three-role owner and are
not restaffed, relinked, or renamed. As with version 3, Ori has no compiled
quest migration, so a saved version-3 quest is shown as incompatible until the
user explicitly chooses Start over; that resets setup progress only.

No host feature is added. The `setup_quests_v2`, `independent_program_homes_v1`,
`specialist_setup_journey_v1`, `template_group_requirements_v1`, and
`blueprint_inputs_v1` requirements are the same as 0.8.0, and Ori's reviewed
floor (0.8.0, blueprint 9) admits this release. Ori's own browser acceptance
still names the three v9 roles and its reviewed pin still points at 0.8.0; both
move in a separate Ori change after the published bytes are verified.
