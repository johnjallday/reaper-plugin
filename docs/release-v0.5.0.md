# v0.5.0 release preparation

This is a source-review candidate, **not publication approval**. Ori's production
reviewed-integration registry remains `ReleaseReady: false`. No real REAPER
session was controlled during this release-preparation pass.

## Candidate identity

- Plugin, portable manifest, CLI, and service version: `0.5.0`
- Blueprint: `reaper-song`, version `4`
- Assistant Program: `music-producer-assistant`, schema `2`
- Host features: `assistant_program_v1`, `specialist_setup_journey_v1`
- Artifact platform: `darwin/arm64` only; executable mode `0755`
- Build toolchain: Go `1.25.13`, CGO off, VCS stamping/build ID disabled,
  trimmed paths and stripped symbols (see `scripts/build-local-artifact.sh`)
- Asset: `reaper-plugin_v0.5.0_darwin_arm64`
- Size: **8,780,098 bytes**
- SHA-256: `2bbf6b77418119cb21e827a407c8d5886e3effdb593ec0ad274e20d7d69c2ca9`
- Intended URL:
  `https://github.com/johnjallday/reaper-plugin/releases/download/v0.5.0/reaper-plugin_v0.5.0_darwin_arm64`

The source PR includes specialist setup declarations, Home/project role scopes,
Sample Library capability binding, staged-runner disclosures, and bounded
verification reasons from the earlier local candidate. Its final reviewed commit
must replace Ori's old `a5f4149f1aaf64611e90ff9484e37f7854c828b9` source pin in a
**later** enablement change; do not simply toggle the old registry entry.

## Security and metadata correction

The original candidate's Go 1.25.0 binary produced **45 standard-library
vulnerability findings** with `govulncheck v1.3.0` (database updated
2026-09-02). The patched Go 1.25.13 binary reports **no vulnerabilities found**.
That is a scan result, not a guarantee of exploitability assessment or complete
security coverage. Both CI and the tag workflow now scan the actual binary;
publication cannot proceed after that command fails.

The rebuilt artifact supersedes the original candidate's
`107708ce680c78aa9756385b1b0bd2af23590901007a458aa61b313a46a36a15`
(8,763,554 bytes). README/release notes also previously retained an older
8,763,458-byte digest. The committed manifest and current documentation now
agree. Regression tests check service/CLI/manifest/documentation identity and
keep the module, build script, and documented patch toolchain aligned.

Gosec 2.29.0 initially reported one G703 finding on the existing operator-only
`REAPER_PLUGIN_HOME` override. Its existing narrow G304 annotation now also
covers G703: the path comes from the process operator, never a manifest, browser,
workspace, or service argument. Tests exercise no override, an explicit directory,
and rejection of missing paths, files, and final-component symlinks without
changing HOME on failure. This annotation does not add new filesystem protection
or claim TOCTOU/ancestor-symlink containment. The final scan reports zero findings.

## Validation performed

| Check | Result |
| --- | --- |
| `go test -race -count=1 ./...` with live-test environment disabled | Pass, both Go packages |
| `go vet ./...` | Pass |
| `make test-ui` | Pass, 16 tests |
| `golangci-lint run --new-from-merge-base=origin/main --timeout=3m` | Zero new issues |
| Gosec 2.29.0 over `./cmd/reaper-plugin/... ./internal/reaper/...` | Zero findings, no package loading errors |
| `actionlint -shellcheck= -pyflakes=` on CI/release workflows | Pass; external shellcheck/pyflakes integrations disabled |
| `make release-package VERSION=v0.5.0`, twice, then `cmp` | Byte-identical output, committed manifest retained |
| `govulncheck -mode=binary dist/reaper-plugin_v0.5.0_darwin_arm64` | No vulnerabilities found |
| Ori `TestReviewedCandidateHostContract`, using `with-local-artifact.sh` | Pass |
| `verify-ori-install.sh`, explicit disposable Ori URL, bundled artifact | Pass; preview/confirmed install remained disabled; cleanup completed |
| Current Ori domain-specialist browser suite, fresh preinstalled sandbox | 11/11 pass, including creation/reload and a second independently staffed project; 19 captures |
| Original coordinated REAPER browser command | **3/4 pass**, one stale host assertion (below) |

Host compatibility was checked against Ori commit
`a50396544988c55cd2b4575baf22c308d1b553e3` (merged PR #452). The local-development
override was explicit; these tests do **not** establish GitHub download/release
verification. Sandbox ports were checked before use; all test listeners were
stopped. No application opening or live REAPER control was enabled.

The original coordinated host case still expects
`4 shared assistant roles will be created and linked`. The actual schema-v2
review says `1 group role and 3 new project-only roles`. Its later expectations
also describe legacy shared staffing. This host test was **not modified or
silently made passing** in the plugin PR. Reconcile that test with the approved
scoped-team contract before claiming the full coordinated command is green.
The current scoped-team journey suite passes independently.

An initial attempt to combine the domain suite with `reaper-demo.sh test` lacked
its preinstalled-plugin fixture: that mode delegates installation to the original
coordinated tests. It failed at installation and skipped nine serial cases. The
correct rerun used fresh `reaper-demo.sh serve` state, which explicitly preinstalls
the development copy, and all 11 domain cases passed. Compiler/linker warnings
from `go-m1cpu`/macOS were non-failing and retained in the local logs.

## Remaining release boundaries

1. Review and merge this plugin source PR. Record the final immutable source
   commit; a squash merge changes it. CI must pass on the reviewed source.
2. Reconcile the stale coordinated host browser test. Obtain separate approval
   before live testing against a disposable REAPER project. The live-smoke
   scripts can change the real app and `$HOME/.ori-reaper`; deterministic tests
   above do not replace that validation.
3. After release review and explicit human publication approval, use the existing
   tag workflow. **Pushing any `v*` tag is the publication boundary**, not a dry
   run. Local `make release-package VERSION=v0.5.0` creates files only.
4. Download the actual published asset and checksum; verify size, digest, mode,
   CLI version, and tag/source commit against the reviewed manifest. Do not use
   a locally staged binary as proof of the remote download.
5. In a separate Ori change, pin the final reviewed plugin source, deliberately
   update registry expectations, and set `ReleaseReady: true` only after the
   remote bytes are verified. Test a fresh GitHub-backed install **without** the
   development override, then ship the enabling Ori build separately.
