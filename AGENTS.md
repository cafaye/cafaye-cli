# AGENTS.md

Agent instructions for the `cafaye-cli` repo.

## Command policy

- Prefer `make` targets over raw `go` commands for repeatable local and CI-like checks.
- Before pushing, run workflow checks locally with `make ci-local` (or `make ci-local-all` for broader workflow coverage).
- If a local workflow run pauses on failure, fix the issue and retry with `make ci-local-retry RUNNER=<runner-name>`.
- Use these targets during implementation:
  - `make test-cmd` for command-layer changes.
  - `make test` for full repository tests.
  - `make test-repeat-cmd` when fixing flaky command tests.
  - `make test-repeat` when validating broader stability.
  - `make verify` before final handoff (`fmt` + full tests).

## Typical loop

1. Make code changes.
2. Run `make test-cmd` (or `make test` if change is cross-cutting).
3. If behavior was flaky, run `make test-repeat-cmd` or `make test-repeat`.
4. Run `make verify` before commit.

## Release policy

- Do not auto-release on every `master` push.
- Only release after:
  - `internal/version/version.go` has been bumped
  - `CHANGELOG.md` has a matching entry
  - changes are committed and pushed
- Use explicit tag-based release flow:
  - `cleo release plan --version vX.Y.Z`
  - `cleo release cut --version vX.Y.Z`
  - `cleo release publish --version vX.Y.Z --final --summary "..." --highlights "..."`
  - `cleo release verify --version vX.Y.Z`
- The tag-triggered GitHub workflow publishes the release and updates Homebrew tap formula.
