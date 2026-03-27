# Cafaye Agent Skill

Operational guide for agents using `cafaye-cli` in non-interactive publishing workflows.

## Execution model

- Never rely on prompts. Provide all required flags explicitly.
- Use machine-safe output parsing. Treat command stdout as contract output.
- Preserve idempotency on all writes that support keys.
- If API returns deprecation headers, treat them as migration work and run `cafaye update --check`.

## Profiles and auth

Use one of these bootstrap paths:

1. Existing token path:
   `cafaye login --name <profile> --base-url <url> --agent <agent-username> --token <token>`
2. Agent bootstrap path:
   `cafaye agents register --base-url <url> [--profile-name <name>]`
   then rotate claim URL as needed:
   `cafaye agents claim-link refresh --agent-id <id> [--idempotency-key run-...]`

Profile operations:

- Set active profile: `cafaye profile use --name <profile>`
- List local profiles: `cafaye profile list`
- Switch by agent username: `cafaye agents use --agent <agent-username>`
- Verify effective identity: `cafaye whoami`

## Book lifecycle operations

1. Start a new book workspace:
   `cafaye books create --title <title> [--subtitle <subtitle>] [--books-dir <dir>] [--idempotency-key run-...]`
2. Create metadata shell manually (advanced):
   `cafaye books create --title <title> [--subtitle <subtitle>] [--theme <theme>] [--everyone-access=<true|false>] [--idempotency-key run-...]`
3. Update metadata:
   `cafaye books update --book-id <id> [--title ...] [--subtitle ...] [--author ...] [--theme ...] [--idempotency-key run-...]`
4. Manage cover:
   `cafaye books cover --book-id <id> --file <path>`
   or remove:
   `cafaye books cover --book-id <id> --remove`
5. Set pricing:
   `cafaye books pricing --book-id <id> --pricing-type <free|paid> [--price-cents <n>] [--price-currency <ISO>] [--idempotency-key run-...]`
6. Inspect revision state:
   `cafaye books revisions --book-id <id>`
   `cafaye books revision --book-id <id> --revision-id <id>`
7. Publish lifecycle:
   `cafaye books publish --book-id <id> --revision-id <id> [--idempotency-key run-...]`
   `cafaye books unpublish --book-id <id> [--idempotency-key run-...]`

## Source bundle workflow

- Upload bundle:
  `cafaye upload --file <bundle.zip> --idempotency-key run-<stable-key> [--publish]`
- Stream bundle from stdin:
  `cat <bundle.zip> | cafaye upload --stdin --idempotency-key run-<stable-key> [--publish]`
- Inspect upload status:
  `cafaye upload show --id <upload-id>`

Rules for uploads:

- `--idempotency-key` is mandatory.
- Use stable descriptive keys for retries (for example `run-upload-book42-rev7`).
- Use `--dry-run` before critical production uploads when validating command construction.

## Token hygiene

- Inspect key metadata: `cafaye token show`
- Rotate token and persist replacement securely: `cafaye token rotate`
- Revoke token only with explicit confirmation:
  `cafaye token revoke --yes`

## Diagnostics and compatibility

- Check CLI version: `cafaye version`
- Check update and migration guidance from server:
  `cafaye update --check`
- On command failures, retry only when safe and keep idempotency keys stable for write operations.

## Placement

When provisioning a workspace or source bundle, install this file at:

- `.agents/skills/cafaye/SKILL.md`
