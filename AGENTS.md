# AGENTS.md

Guidance for an agent session working in this repository. It is authoritative
for building, testing, versioning and releasing the plugin, and for the
boundary with Ori. Start the session inside a worktree of this repository so
this file and `.claude/settings.json` apply.

## What this repository is

`reaper-plugin` is the Ori / Claude / Codex plugin for REAPER. One Go module
produces one binary, `reaper-plugin`, that is both the helper CLI and the
private MCP stdio service Ori installs. Around it:

| Path | Role |
| --- | --- |
| `cmd/reaper-plugin/` | CLI entry point, `serve`, the `version` constant, release-identity tests |
| `internal/reaper/` | Web Remote client, runner, profile, tidy, service operations |
| `.ori-plugin/plugin.json` | The Ori manifest: capability, surfaces, service, artifact, blueprint, setup quest |
| `.claude-plugin/plugin.json` | Portable identity for Claude and Codex |
| `blueprints/reaper-song/` | The Reaper Song project blueprint (`template.json`) |
| `skills/` | Portable skills: `reaper-web-remote`, `reaper-session-setup`, `reaper-project-tidy` |
| `ui/live-control/`, `ui/project-tidy/` | The two Workspace Surfaces (plain ES modules with Node tests) |
| `protocol/workspace-surface-v1/` | Wire-contract fixtures shared with Ori |
| `scripts/` | Artifact build, release packaging, verification, live smoke |

The service only talks to REAPER's Web Remote on localhost and writes only
under `~/.ori-reaper/`. Every new operation needs a declared input and output
schema, policy and timeout class in the manifest; nothing reaches an agent
without one.

## Checkouts and worktrees

- Main checkout: `/Users/jjdev/Projects/ori/plugins/reaper-plugin`, kept on
  `main`. Use it to read and to `git fetch`; never commit there.
- Work happens in its own worktree under
  `/Users/jjdev/Projects/ori/plugins/reaper-plugin-worktrees/<name>`, on a
  branch cut from `origin/main`:

  ```bash
  git -C /Users/jjdev/Projects/ori/plugins/reaper-plugin fetch origin
  git -C /Users/jjdev/Projects/ori/plugins/reaper-plugin worktree add \
    ../reaper-plugin-worktrees/<name> -b <type>/<name> origin/main
  ```

- `reaper-plugin-worktrees/v0.9.0` is a detached checkout of the released tag
  that Ori's harness specs stage. Leave it alone.
- `tasks/` is ignored. Checklists (`tasks/tasks-<feature>.md`), findings and
  evidence live there and never land in a commit.
- This repository is never added to an Ori session with `/add-dir`, and an
  Ori builder never edits this checkout. A session here never edits Ori or
  the Music Project Management package.
- `go.work` is ignored here on purpose. If a Go command complains about a
  workspace it found in a parent directory, run it with `GOWORK=off`.

## Build and test

```bash
make build          # bin/reaper-plugin
make test           # go test ./...
go vet ./...
make test-ui        # node --test ui/**/*.test.js
npx prettier --check 'ui/**/*.{js,css,html}'
make fmt            # gofmt -w ./cmd ./internal
```

CI (`ci.yml`, macOS 14, on every push and pull request) runs
`go test -race ./...`, `go vet ./...`, the UI tests, then
`./scripts/package-release.sh v<manifest version>` and `govulncheck` on the
built binary. Run the race tests and vet before opening a pull request. The
packager only passes when the committed manifest digest matches a fresh build,
so run it as well whenever Go code or the manifest changed.

Scripts that change files:

- `make artifact-local` (`scripts/build-local-artifact.sh`) builds
  `artifacts/reaper-plugin-darwin-arm64` with the pinned toolchain and
  rewrites `sha256` and `size` in `.ori-plugin/plugin.json`. Run it only when
  the new digest is meant to be committed, that is as part of a version bump.
  Any other time it dirties the manifest; revert that with
  `git checkout -- .ori-plugin/plugin.json`.
- `./scripts/with-local-artifact.sh <command>` points the manifest at the
  locally built bytes for the duration of `<command>` and restores it on exit.
  Never commit what it leaves behind if it is interrupted.
- `scripts/live-smoke.sh`, `scripts/live-project-smoke.sh` and
  `scripts/live-tidy-smoke.sh` drive a running REAPER through Web Remote and
  change the open project inside undo blocks. Run them only when the owner
  asks, with a throwaway project open.
- `scripts/verify-ori-install.sh` and `scripts/verify-ori-quest.py` read an
  Ori server (`ORI_URL`, default `http://127.0.0.1:8931`). Point them at Ori's
  isolated demo server, never at a real one.

## Versions

`cmd/reaper-plugin/release_metadata_test.go` holds the release identity
together. A version bump is one commit that changes all of:

- `cmd/reaper-plugin/main.go` `const version`
- `.claude-plugin/plugin.json` `version`
- `.ori-plugin/plugin.json` `version` and the artifact `url`
  (`.../releases/download/v<version>/reaper-plugin_v<version>_darwin_arm64`)
- `.ori-plugin/plugin.json` artifact `sha256` and `size`, written by
  `make artifact-local`
- `README.md` and a new `## <version> — <title>` section at the top of
  `RELEASE_NOTES.md`, each stating the artifact's size in bytes and its sha256

The build is reproducible (`CGO_ENABLED=0`, `-trimpath`, `-buildid=`, the
`GOTOOLCHAIN` pinned in `scripts/build-local-artifact.sh`), so the digest you
commit is the digest CI and the release rebuild. The `go` line in `go.mod`, the
`release_toolchain` in the build script and the README's "pins Go" sentence
must agree; a toolchain bump touches all three.

Other versioned things, each a contract with Ori: the Reaper Song blueprint
`version` in `blueprints/reaper-song/template.json`, the `reaper_setup` quest
schema and version, the Workspace Surface `protocol` range, and
`requires_host_features`.

## Release

Pushing a tag `v<X.Y.Z>` runs `release.yml`: tests, vet, UI tests,
`package-release.sh` (which refuses a tag that differs from the binary's
version or a manifest digest that differs from the rebuild), `govulncheck`,
then `gh release create` with `RELEASE_NOTES.md` as the notes.

A builder session never creates or pushes a tag and never runs `gh release`.
It lands the version bump on `main` through a pull request; the owner tags.
Ori's reviewed floor for this plugin (`MinimumVersion`, `FallbackCommit` and
`MinimumBlueprintVersion` in Ori's `internal/reviewedintegration/entries.go`)
is bumped by the Ori integration owner once the release exists, in Ori, never
from here.

## Contract with Ori

- `requires_host_features` in the manifest must each exist as a constant in
  Ori's `internal/plugin/surface_descriptor.go`. Today:
  `independent_program_homes_v1`, `specialist_setup_journey_v1`,
  `setup_quests_v2`, `template_group_requirements_v1`, `blueprint_inputs_v1`
  and `home_profile_v1`. A new feature is an Ori change first; require it only
  once an Ori release advertises it, and say which Ori version in the README.
- Ori's reviewed floor pins a minimum plugin version, a fallback commit, the
  `reaper-song` blueprint at a minimum version, protocol 1 and `darwin/arm64`.
  An older Ori keeps using the floor release, so a release must either stay
  installable there or raise the floor through Ori.
- `protocol/workspace-surface-v1/` fixtures are the wire contract both sides
  test against. Changing one is a contract change that Ori must mirror.
- Music Project Management allowlists exactly `reaper-song` and
  `reaper-song-team` v1 for attachment. Renaming either, or changing the team
  schema, is a Music change too.

When a feature spans repositories, the Ori integration owner writes the
`Cross-Repository Contract` section of the Ori task list (host features,
versions, releases, candidate path, order) and tells this session which fields
changed. This session keeps its own `tasks/tasks-<feature>.md` and does not
read the Ori checklist.

## How Ori tests a candidate

From an Ori worktree:

```bash
./scripts/reaper-demo.sh serve --reaper-source /path/to/this/worktree
./scripts/reaper-demo.sh test  --reaper-source /path/to/this/worktree
./scripts/reaper-demo.sh test  --reaper-source /path/to/this/worktree \
  --music-source /path/to/music/worktree --install-order music-first
```

The harness refuses a dirty worktree, exports the committed `HEAD` tree into a
disposable sandbox and builds the artifact there; it never runs the build
helpers inside this checkout. Commit before handing the path over; uncommitted
edits are invisible to it. Nothing is installed under the real `HOME`. A
candidate staged this way is evidence for an Ori task; the published release
and the reviewed floor are later, separately approved states.

## Commits and pull requests

- The branch prefix says intent: `feature/`, `fix/`, `docs/`, `chore/`.
- Commit subjects are present tense and specific.
- Pull requests target `main` and are squash-merged, so the PR title is the
  durable message. The PR body names the manifest, blueprint, protocol or
  host-feature changes and says whether the version was bumped.
- Push and open the PR only when the owner asks. Never force-push `main`.
