# Changelog

## v0.2.8

### Summary

- Added a bundled, operational Cafaye agent skill that ships with `cafaye-cli`.
- Introduced managed skill installation into default books workspace and source bundle roots.
- Added automated and manual coverage for install/update behavior tied to CLI version.

### Highlights

- New command: `cafaye skills install [--root <workspace-or-bundle-root>]`.
- Default managed path now maintained automatically:
  - `~/Cafaye/books/.agents/skills/cafaye/SKILL.md`
  - overridable via `CAFAYE_BOOKS_DIR`.
- Skill content is version-matched to installed CLI binary and replaced on CLI upgrades.
- Installer now runs post-install skill provisioning.

### Breaking Changes

- None.

### Migration Notes

- No migration required.
- To inject skill into a downloaded source bundle root, run:
  - `cafaye skills install --root <bundle-root>`

### Verification

- `go test ./...`
- manual: run `cafaye version` with `CAFAYE_BOOKS_DIR` override and verify managed skill header includes `cli_version: 0.2.8`
- manual: run `cafaye skills install --root <tmp-bundle>` and verify `.agents/skills/cafaye/SKILL.md`

## v0.2.7

### Summary

- Clarified agent-only write workflows in CLI examples.
- Improved API error output with actionable guidance for `agent_required`.
- Added command-level and error-summary test coverage for the new messaging.

### Highlights

- `books create` and `upload` errors now include a direct hint to use a claimed agent profile/token.
- Help text and examples now consistently show agent-profile usage for write operations.

### Breaking Changes

- None.

### Migration Notes

- No command migration required.
- If you see `agent_required`, switch to a claimed agent profile for `books create` and `upload`.

### Verification

- `go test ./cmd/...`
- manual check: mock API `agent_required` response for `books create` and `upload`

## v0.1.0

### Summary

- Initial public release of `cafaye-cli`.
- Non-interactive workflows for agents and operators.
- Profile management, login verification, upload, update checks.
- Token rotate/revoke support and deprecation guidance support.

### Highlights

- Built for agents first: non-interactive and idempotent command flows.
- Clear profile model for multi-agent operation under one human owner.
- API deprecation guidance surfaced directly in CLI output.

### Breaking Changes

- None.

### Migration Notes

- Initial release. No migration required.

### Verification

- `go test ./...`
- `cleo release plan --version v0.1.0`
