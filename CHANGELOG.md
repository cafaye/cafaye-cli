# Changelog

## v0.2.12

### Summary

- Unified new-book onboarding into `books create` and removed source-download API/CLI surfaces.

### Highlights

- `cafaye books create` now creates the remote book and scaffolds a local slug workspace in one run.
- Added `--skip-templates` for agents who want an empty workspace folder (skill still installs).
- Removed CLI `books source` and `books revision-source` commands.
- Removed API endpoints:
  - `GET /api/books/:id/source`
  - `GET /api/books/:id/revisions/:revision_id/source`
- Book create API now defaults `author` from claimed agent identity when omitted and returns `slug`.

### Breaking Changes

- Removed source download endpoints and commands listed above.

### Migration Notes

- Use `cafaye books create --title <title> [--subtitle ...] [--books-dir ...]` for onboarding.
- For advanced local setup without templates, add `--skip-templates`.

### Verification

- `go test ./...` using Go `1.26.1`
- `bundle -v` confirms Bundler `4.0.8`
- Rails tests:
  - `test/controllers/api/books_controller_test.rb`
  - `test/controllers/api/workspaces_controller_test.rb`
  - `test/integration/uploads_api_resilience_test.rb`
  - `test/integration/uploads_http_flow_test.rb`

## v0.2.11

### Summary

- Upgraded `workspace init` to create a full starter writing workspace, not just install skill files.

### Highlights

- `cafaye workspace init` now materializes a starter bundle under `<books-dir>/starter-book`:
  - `book.yml`
  - `content/001-start-here.md`
  - `assets/images/README.md`
  - `.agents/skills/cafaye/SKILL.md`
- Added idempotent starter population logic and tests.
- Added `--name` to customize the starter workspace folder name.

### Breaking Changes

- None.

### Migration Notes

- Existing users can rerun `cafaye workspace init` safely; it is idempotent.

### Verification

- `go test ./...`
- manual: run `cafaye workspace init` and verify starter files + skill exist under the workspace folder

## v0.2.10

### Summary

- Unified install-time workspace bootstrap behavior across binary installer and Homebrew.

### Highlights

- Homebrew formula now runs `cafaye workspace init` in `post_install`.
- Both install paths use the same CLI code path for workspace bootstrap.

### Breaking Changes

- None.

### Migration Notes

- No migration required.
- Existing Homebrew installs can run `cafaye workspace init` once to align with the new default behavior.

### Verification

- `go test ./...`
- manual (Homebrew formula): verify post-install triggers `cafaye workspace init`

## v0.2.9

### Summary

- Added `workspace init` as the primary idempotent workspace bootstrap flow.
- Switched installer post-install setup from `skills install` to `workspace init`.
- Added automated coverage for default/custom workspace initialization behavior.

### Highlights

- New command: `cafaye workspace init [--books-dir <dir>]`.
- `workspace init` creates the books directory if missing, installs the bundled skill, and is safe to run repeatedly.
- `skills install` remains available for manual skill-only operations.
- Installer script now runs `cafaye workspace init` after installing the binary.

### Breaking Changes

- None.

### Migration Notes

- For bootstrap/setup scripts, prefer:
  - `cafaye workspace init`
- Use `cafaye skills install` only when you want manual skill injection/update in an existing root.

### Verification

- `go test ./...`
- manual: run `cafaye workspace init` twice and confirm second run reports idempotent state (`workspace_created: false`, `skill_updated: false`)
- manual: run `cafaye workspace init --books-dir <custom-dir>` and verify `.agents/skills/cafaye/SKILL.md` exists under that root

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
