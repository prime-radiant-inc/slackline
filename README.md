# slackline

Give AI agents a Slack identity. Send messages, read channels, stream real-time events.

One binary. One config file. One bot identity.

## Install

**Private repo — requires `gh` CLI, authenticated:**

```bash
bash <(gh api repos/prime-radiant-inc/slackline/contents/install.sh --jq '.content | @base64d')
```

Installs to `~/.local/bin/slackline`. The script warns if that directory isn't in `$PATH`.

> When the repo goes public: `curl -fsSL https://raw.githubusercontent.com/prime-radiant-inc/slackline/main/install.sh | bash`

**Supported platforms:** `darwin/arm64`, `linux/amd64`

## Setup

Two paths depending on your role.

### Admin: provision a new bot

See [Provisioning a new bot](#provisioning-a-new-bot) for the full agentic recipe. The short version:

```bash
# One-time per machine: store App Configuration Tokens
slackline provision bootstrap

# Per bot: create the Slack app (non-interactive, machine-readable JSON output)
slackline provision my-bot-name > /tmp/prov.json
```

### Developer / agent: configure with existing tokens

Tokens must be provisioned by an admin first.

```bash
slackline init
```

Prompts for a bot token (`xoxb-`) and app token (`xapp-`), validates against the workspace, writes `~/.config/slackline/config.json`.

### Verify

```bash
slackline auth status
```

Note: App Token shows `(configured)` not `(valid)` — app tokens can't be validated via the REST API. `(configured)` means the `xapp-` prefix is present.

## Commands

### send

```bash
slackline send --channel <channel> [--message <text>] [--thread <ts>] [--attach <path>] ...
echo "text" | slackline send --channel <channel>
```

`--channel` accepts name (`#ops`), ID (`C...`), or Slack URL. `--message` or piped stdin. Trailing newline stripped from stdin. `--attach` may be repeated; message text is optional when at least one file is attached.

Text-only output:
```json
{"ok":true,"channel":"C...","ts":"1234567890.123456"}
```

With `--attach`:
```json
{"ok":true,"channel":"C...","thread_ts":"...","files":[{"id":"F...","title":"file.txt"}]}
```

`thread_ts` is included in output only when `--thread` was passed.

### read

```bash
slackline read --channel <channel> [--limit 20] [--thread <ts>] [--since <RFC3339>]
```

Returns the most recent `--limit` messages as JSONL in **chronological order** (oldest first). For both channel and thread reads, `--limit` counts back from the newest message, so the latest reply is always included.

```json
{"ts":"1234567890.123456","user":"U...","text":"hello"}
{"ts":"1234567890.654321","user":"U...","text":"reply","thread_ts":"1234567890.123456"}
```

`thread_ts` is omitted when a message is not a thread reply.

### ask

```bash
slackline ask --channel <channel> [--message <text>] [--timeout 300] [--poll 10]
echo "text" | slackline ask --channel <channel>
```

Sends a message and polls the thread for replies from other users. Outputs replies as JSONL when received. Exits 0 on reply, **exits 5 on timeout** (no stdout output on timeout — error goes to stderr).

### listen

```bash
slackline listen [--type mention,dm,...] [--threads] [--all-messages] [--include-bot-self]
```

Streams real-time events via Socket Mode to stdout as JSONL. Runs until interrupted. Requires both bot token and app token.

| Flag | Effect |
|------|--------|
| (none) | `mention`, `dm`, `reaction` only |
| `--type <list>` | emit only the named types (`mention`, `dm`, `thread_reply`, `channel_message`, `reaction`); emit-time filter, does not widen subscription; `channel_message` requires `--all-messages` |
| `--threads` | also emits `thread_reply` for threads the bot has participated in |
| `--all-messages` | firehose: every message in every channel the bot is in (implies `--threads`) |
| `--include-bot-self` | do not filter out events from the bot's own user ID |

See [Event reference](#event-reference) for full event shapes. Status messages (`connected`, `reconnecting`, `disconnected`) go to stderr as plain text.

### react

```bash
slackline react add    --channel <channel> --ts <ts> --emoji <name>
slackline react remove --channel <channel> --ts <ts> --emoji <name>
```

Add or remove an emoji reaction on a message. Idempotent: `already_reacted` (on add) and `no_reaction` (on remove) are treated as success.

```json
{"ok":true,"channel":"C...","ts":"...","emoji":"thumbsup","action":"added"}
{"ok":true,"no_op":true,"channel":"C...","ts":"...","emoji":"thumbsup","action":"added"}
```

`no_op: true` means the reaction was already in the desired state. Emoji colons are stripped automatically (`:thumbsup:` and `thumbsup` both work).

### download

```bash
slackline download --file <file-id> --out <path>
slackline download --file <file-id> --out -          # stream to stdout
slackline download --file <file-id> --out <path> --force  # overwrite existing
```

Download a Slack file by ID (from a `files` array on a listen event). File IDs start with `F`. Default size cap is 100 MB; override with `SLACKLINE_MAX_DOWNLOAD_BYTES`. Disk writes use atomic `.tmp` + rename. On disk-write success, a summary is written to stderr:

```json
{"ok":true,"file":"F...","name":"report.pdf","mimetype":"application/pdf","size":12345,"path":"/tmp/report.pdf"}
```

### channels

```bash
slackline channels [--all] [--json]
```

Default: table of channels the bot has joined. `--all`: all visible channels. `--json`: JSON array with `id`, `name`, `topic`, `purpose`.

## Configuration

**Config file:** `~/.config/slackline/config.json` (written by `init` or `create`, mode `0600`)

**Override precedence (highest wins):**

1. `--config <path>` flag
2. `SLACKLINE_CONFIG=<path>` env var
3. Default path (`~/.config/slackline/config.json`)

**Token env overrides** (bypass config file entirely):

```bash
SLACKLINE_BOT_TOKEN=xoxb-...
SLACKLINE_APP_TOKEN=xapp-...
```

**Multiple identities on one machine:** use separate config files.

```bash
SLACKLINE_CONFIG=~/.config/slackline/other-bot.json slackline send --channel '#ops' --message 'hi'
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Slack API error |
| 2 | Auth error (invalid/revoked token) |
| 3 | Config error (missing file, unreadable, no token) |
| 4 | Usage error (bad flags, missing required input) |
| 5 | Timeout (`ask` received no reply) |

All errors write to stderr as JSON:

```json
{"error":"not_in_channel","detail":"..."}
```

To programmatically detect a broken bot token from `auth status` output, grep for `(invalid`.

## Provisioning a new bot

The `slackline-provision-bot` skill (in `skills/slackline-provision-bot/SKILL.md`) documents the full agentic recipe that drives browser automation for the steps Slack requires through their admin UI. The core CLI flow:

```bash
# 1. One-time per machine: seed provision.json.
# Option A: env vars (CI/scripts).
SLACKLINE_CONFIG_TOKEN=xoxe.xoxp-... \
SLACKLINE_REFRESH_TOKEN=xoxe-... \
slackline provision bootstrap

# Option B: interactive (paste from https://api.slack.com/apps).
slackline provision bootstrap

# 2. Per bot: create the Slack app (no interaction, machine-readable JSON).
slackline provision my-bot-name > /tmp/prov.json
# stdout: {"ok":true,"app_id":"A...","install_url":"...","oauth_authorize_url":"...","oauth_page_url":"...","general_page_url":"..."}

INSTALL_URL=$(jq -r .oauth_authorize_url /tmp/prov.json)
# (browser automation: navigate to $INSTALL_URL, allow, collect xoxb- and xapp- tokens)

# 3. Write config for the new bot.
SLACKLINE_BOT_TOKEN="$BOT_TOKEN" \
SLACKLINE_APP_TOKEN="$APP_TOKEN" \
slackline init

# 4. Verify.
slackline auth status
```

The `slackline-provision-bot` skill contains the full browser selector reference and automation gotchas.

## Event reference

All `slackline listen` events are JSONL objects on stdout. Fields marked `?` are omitted when empty.

### mention

Emitted when the bot is @-mentioned in any channel it is in.

```json
{"type":"mention","channel":"C...","user":"U...","text":"<@UBOT> hello","ts":"...","thread_ts":"?","files":[{"id":"F...","name":"file.txt","mimetype":"text/plain","size":1234,"title":"file.txt"}]}
```

### dm

Emitted for direct messages to the bot.

```json
{"type":"dm","channel":"D...","user":"U...","text":"hello","ts":"...","thread_ts":"?","files":[...]}
```

### thread_reply

Emitted by default whenever someone replies in a thread the bot started (parent message authored by the bot). With `--all-messages`, also emitted for all thread replies in subscribed channels. `--threads` is accepted for backward compatibility but is currently a no-op — bot-parent thread replies are always emitted.

```json
{"type":"thread_reply","channel":"C...","user":"U...","text":"reply","ts":"...","thread_ts":"...","parent_user_id":"U...","files":[...]}
```

### channel_message

Emitted only with `--all-messages`. Top-level non-thread messages.

```json
{"type":"channel_message","channel":"C...","user":"U...","text":"hi","ts":"...","thread_ts":"?","parent_user_id":"?","files":[...]}
```

### reaction

Emitted when a reaction is added or removed. `action` is `added` or `removed`.

```json
{"type":"reaction","action":"added","channel":"C...","user":"U...","emoji":"thumbsup","item_ts":"..."}
```

### File schema

Files are present only when the sender attached files to the message. File objects contain no download URLs — use `slackline download --file ID --out PATH` to fetch content.

```json
{"id":"F...","name":"report.pdf","mimetype":"application/pdf","size":204800,"title":"Q1 Report"}
```

## Migration

### reaction_added / reaction_removed → reaction

The split `reaction_added` / `reaction_removed` listen events (introduced in 0.2.0) are unified back into a single `reaction` event carrying an `action` field (`"added"` | `"removed"`). Update listeners that match `"type":"reaction_added"` / `"reaction_removed"` to match `"type":"reaction"` and branch on `action`.

### slackline create removed

`slackline create` (both `--init` and `--name` forms) has been replaced:

| Old | New |
|-----|-----|
| `slackline create --init` | `slackline provision bootstrap` |
| `slackline create --name my-bot` | `slackline provision my-bot` (then `slackline init` for per-developer config) |

Running `slackline create` now exits with a usage error pointing to the new commands.

### Manifest scope changes

Existing bots provisioned before this release may need to be reinstalled (via their OAuth authorize URL) to pick up three new bot token scopes:

- `reactions:write` — required by `slackline react add/remove`
- `files:read` — required by `slackline download` and receiving file metadata in listen events
- `files:write` — required by `slackline send --attach`

Check current scopes at `https://api.slack.com/apps/<APP_ID>/oauth`. If the scopes are missing, re-run `slackline provision <name>` to get a fresh OAuth authorize URL, then reinstall.

## Development

Requires Go 1.25+.

```bash
make build                    # build ./slackline
make test                     # go test ./... -v
make vet                      # go vet ./...
make release VERSION=1.2.3    # tag + push (requires clean working tree)
```

CI runs on push/PR to `main` (vet, test, golangci-lint). Release binaries for `darwin/arm64` and `linux/amd64` are built and attached to GitHub Releases automatically when a `v*` tag is pushed.
