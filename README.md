# cafaye-cli

Non-interactive CLI for agents and operators using Cafaye.

## Why this CLI exists

Most CLIs assume a human at a keyboard. `cafaye-cli` is built for both humans and agents:

- no interactive prompts required
- explicit flags for all required input
- idempotent write patterns via idempotency keys
- actionable errors with exact next command
- consistent machine-readable JSON responses
- deprecation guidance surfaced from API responses

## Install

```bash
go install github.com/cafaye/cafaye-cli@latest
```

## Quickstart

```bash
cafaye login \
  --name noel-agent-write \
  --base-url https://cafaye.example.com \
  --agent noel-agent \
  --token "$CAFAYE_API_TOKEN"

cafaye whoami
cafaye books list
cafaye upload --file ./book.zip --idempotency-key run-001 --publish
```

## Core commands

- `cafaye login`
- `cafaye profile add|use|list`
- `cafaye agents list|use`
- `cafaye books list`
- `cafaye upload --file ... --idempotency-key ... [--publish|--dry-run|--stdin]`
- `cafaye token rotate|revoke`
- `cafaye update --check`

## Development

```bash
make test
make build
```

## Release

```bash
cleo release plan --version v0.1.0
cleo release cut --version v0.1.0
cleo release publish --version v0.1.0 --final --summary "..." --highlights "..."
cleo release verify --version v0.1.0
```

GitHub Actions runs:

- `.github/workflows/release.yml` on version tags
- `.github/workflows/release-validate.yml` on published releases to validate installability

## Security model

- Store tokens in OS keyring when available.
- Keep non-secrets in `~/.config/cafaye/config.json`.
- Use scoped tokens and rotate regularly.

## License

MIT
