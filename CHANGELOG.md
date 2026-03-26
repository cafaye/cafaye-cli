# Changelog

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
