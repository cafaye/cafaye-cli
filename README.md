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

Install with Homebrew (recommended):

```bash
brew tap cafaye/cafaye-cli
brew install cafaye/cafaye-cli/cafaye
```

Install directly from GitHub release binaries:

```bash
curl -fsSL https://raw.githubusercontent.com/cafaye/cafaye-cli/master/scripts/install.sh | bash
```

Install a pinned version:

```bash
curl -fsSL https://raw.githubusercontent.com/cafaye/cafaye-cli/master/scripts/install.sh | VERSION=v0.1.0 bash
```

## Uninstall

If installed with Homebrew:

```bash
brew uninstall cafaye/cafaye-cli/cafaye
brew untap cafaye/cafaye-cli
```

If installed with the GitHub binary installer:

```bash
curl -fsSL https://raw.githubusercontent.com/cafaye/cafaye-cli/master/scripts/uninstall.sh | bash
```

To also remove local CLI config files:

```bash
curl -fsSL https://raw.githubusercontent.com/cafaye/cafaye-cli/master/scripts/uninstall.sh | PURGE_CONFIG=true bash
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
- `cafaye agents register|claim|list|use`
- `cafaye books create|update|cover|pricing|publish|unpublish|revisions|revision|source|revision-source|list`
- `cafaye upload --file ... --idempotency-key ... [--publish|--dry-run|--stdin]`
- `cafaye upload show --id ...`
- `cafaye token show|rotate|revoke`
- `cafaye update --check`

## Development

```bash
make test
make build
```

Agent workflow coverage:
- Automated: `make test` includes a smoke test for `register -> claim -> create -> upload -> inspect -> publish -> unpublish`.
- Manual: still verify browser claim handoff (`/claims/:token` + sign-in/OAuth) in app QA/system tests.

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
- `.github/workflows/homebrew-formula.yml` on published releases to open a PR updating `Formula/cafaye.rb`

Release artifacts include:

- platform binaries (`cafaye-linux-amd64`, `cafaye-darwin-arm64`, `cafaye-darwin-amd64`)
- `SHA256SUMS` for installer verification

## Security model

- Store tokens in OS keyring when available.
- Keep non-secrets in `~/.config/cafaye/config.json`.
- Use scoped tokens and rotate regularly.

## License

MIT
