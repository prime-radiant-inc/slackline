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

### Admin: create a new bot

You need an **App Configuration Token** from [api.slack.com/apps](https://api.slack.com/apps) → "Your App Configuration Tokens" → "Generate Token".

```bash
# First time: store your App Configuration Token
slackline create --init

# Create the Slack app (interactive — walks you through installation and token collection)
slackline create --name my-bot-name
```

`create` handles everything: manifest API, app installation, bot token, app token, writes config.

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
slackline send --channel <channel> [--message <text>] [--thread <ts>]
echo "text" | slackline send --channel <channel>
```

`--channel` accepts name (`#ops`), ID (`C...`), or Slack URL. `--message` or piped stdin. Trailing newline stripped from stdin.

```json
{"ok":true,"channel":"C...","ts":"1234567890.123456"}
```

`thread_ts` is included in output only when `--thread` was passed.

### read

```bash
slackline read --channel <channel> [--limit 20] [--thread <ts>] [--since <RFC3339>]
```

Returns JSONL in **chronological order** (oldest first), up to `--limit` messages.

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

Sends a message and polls the thread for replies from other users. Outputs replies as JSONL when received. Exits 0 on reply, **exits 1 on timeout** (no stdout output on timeout — error goes to stderr).

### listen

```bash
slackline listen
```

Streams real-time events via Socket Mode to stdout as JSONL. Runs until interrupted. Requires both bot token and app token.

Event types:

```json
{"type":"mention","channel":"C...","user":"U...","text":"<@UBOT> help","ts":"...","thread_ts":"..."}
{"type":"dm","channel":"D...","user":"U...","text":"hello","ts":"..."}
{"type":"reaction","channel":"C...","user":"U...","emoji":"thumbsup","item_ts":"..."}
```

`thread_ts` on `mention` is omitted for top-level messages. Bot self-events are filtered. Status messages (`connected`, `reconnecting`, `disconnected`) go to stderr as plain text.

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

All errors write to stderr as JSON:

```json
{"error":"not_in_channel","detail":"..."}
```

To programmatically detect a broken bot token from `auth status` output, grep for `(invalid`.

## Development

Requires Go 1.25+.

```bash
make build                    # build ./slackline
make test                     # go test ./... -v
make vet                      # go vet ./...
make release VERSION=1.2.3    # tag + push (requires clean working tree)
```

CI runs on push/PR to `main` (vet, test, golangci-lint). Release binaries for `darwin/arm64` and `linux/amd64` are built and attached to GitHub Releases automatically when a `v*` tag is pushed.
