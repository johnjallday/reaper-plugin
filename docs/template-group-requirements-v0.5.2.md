# Reaper Song template group contract (0.5.2 candidate)

## Identity and compatibility

This candidate starts from plugin `0.5.1` source commit
`972c33fb50b813c73beaafd779c71ced266477fa`. It changes the plugin/service to
`0.5.2` and Reaper Song to blueprint v6. The `reaper_setup` quest remains schema
and version 1 with unchanged step identities.

The plugin now requires `template_group_requirements_v1` in addition to
`assistant_program_v1`, `specialist_setup_journey_v1`, and `setup_quests_v1`.
An older host must refuse installation or enablement. A compatible host validates
the strict declarations before cataloging the blueprint; the declarations do
not choose a route, workspace ID, owner, path, command, or permission.

## Required grouped source

The plugin original declares:

- policy `required`;
- target `music-producer-assistant`;
- missing-Home behavior `offer_create`; and
- display-only default `Music Production Home`.

Ori resolves the current user and trusted plugin owner, reviews Home creation or
exact reuse, then observes the child parent, Assistant Project link, and
reciprocal Home membership before reporting success. A same-name ordinary group,
a copied parent ID, or a Home owned by another user/program is not a match.
Renaming the exact Home does not change identity.

Ordinary folder grouping remains organizational. It grants no Home role,
portfolio, sibling-project, filesystem, runtime, capability, or agent authority.
Those effects require their existing exact links, grants, and confirmations.

## Custom standalone composition

The Required plugin original has no creation-time opt-out. Templates → Customize
creates a source-linked user variant whose policy may be changed to None or
Recommended. Selecting standalone uses the bounded v1 composition:

- remove the Assistant Program, pre-workspace `reaper_setup` quest, Home roles,
  portfolio/reflection/learning, shared stage, and all Home/link consequences;
- convert Producer, Mix Engineer, and Songwriter into ordinary agents scoped to
  the one project workspace with standalone prompts; and
- retain new/existing project modes, one authoritative `.rpp` entry, both
  mode-filtered starter tasks, REAPER skills, the `reaper-live-control`
  capability attachment, File-only and Ori-assisted runtime modes, and the
  post-workspace setup wizard.

File-only needs no live REAPER verification. Live actions still require the
existing workspace grant, fresh exact-project verification, operation policy,
and confirmation. Standalone does not broaden those gates.

## Clean start and lifecycle

No existing Ori REAPER workspace is backfilled, adopted by display name,
regrouped, reset, or deleted. A new-contract workspace stores its creation-time
policy, source identity, selected composition, target IDs, and review/operation
digests. Later template edits do not rewrite that snapshot.

Reviewed disconnect preserves a Required snapshot and leaves an explicit
unfulfilled state. Generic move, trash, and delete cannot erase the contract.
Reconnect is separately reviewed against the exact recorded Home. Home removal
preserves project workspaces and external project files; it does not fabricate a
standalone waiver.

A fresh existing-project connection selects one `.rpp` from a user-approved
external folder. Review is inert, commit leaves the external bytes in place, and
project-specific live control remains unapproved and unverified.

## Local candidate evidence

The deterministic macOS arm64 candidate built by `make artifact-local` is:

- path: `artifacts/reaper-plugin-darwin-arm64`;
- size: `8,780,098` bytes;
- SHA-256: `999dda3764c85487329322bbf7df775fb389316b9c8e4320bf8dd62c5fa91987`;
- expected CLI/service version: `0.5.2`; and
- mode after release packaging: executable, macOS arm64.

Run:

```bash
GOWORK=off make test
GOWORK=off make test-ui
GOWORK=off go vet ./...
GOWORK=off make artifact-local
GOWORK=off make release-package VERSION=v0.5.2
```

These are local candidate checks only. They do not prove a Git tag, GitHub
release, remote URL reachability, published-byte identity, Ori reviewed-registry
approval, installation into user state, or live REAPER operation. Publishing,
pushing, and updating Ori's reviewed pin require separate approval and exact
remote source/artifact evidence.
