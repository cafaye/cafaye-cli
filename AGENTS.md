# AGENTS.md

Agent instructions for the `cafaye-cli` repo.

## Command policy

- Prefer `make` targets over raw `go` commands for repeatable local and CI-like checks.
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
