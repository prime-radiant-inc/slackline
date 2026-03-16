# Slackline — Design Spec

## Overview

A cross-platform Go binary that gives AI agents (and humans) a Slack identity. Handles app provisioning, messaging, channel discovery, and real-time event listening via Socket Mode. Replaces the current `slackcli` + shell scripts approach with a single self-contained tool.

## Motivation

The current slack-messaging skill depends on a third-party TypeScript/Bun binary (`shaharia-lab/slackcli`), stores tokens in two places, hardcodes channel IDs, and has no real-time event capability. Each new Claude instance requires manual token setup with no guided flow.

Slackline solves this with:
- Guided provisioning that creates a Slack app via the manifest API
- Single config file with clean token storage
- Dynamic channel resolution (no hardcoded tables)
- Socket Mode listening for real-time @mentions and DMs
- Stdin-based message input to avoid shell escaping issues

## Linear Ticket

[REDACTED: Self-service Slack bot provisioning for Claude instances](https://linear.app/prime-radiant/issue/REDACTED/self-service-slack-bot-provisioning-for-claude-instances)

## Architecture

Single Go binary using Cobra for subcommands and `slack-go/slack` for all Slack API interaction. One binary, one config, one bot identity per installation.

```
slackline (Go binary)
├── cmd/           ← Cobra subcommands (create, init, send, read, ask, listen, channels, auth)
├── slack/         ← Shared Slack API client wrapping slack-go/slack
├── provision/     ← Manifest API + guided token flow
├── config/        ← Config file read/write + env var override
└── listen/        ← Socket Mode + JSONL output
```

**Key dependency:** `github.com/slack-go/slack` — mature Go library with full Socket Mode support, manifest APIs, OAuth token exchange, and all messaging/history methods. 4,900+ stars, actively maintained (v0.19.0, March 2026).

### Identity Model

One slackline installation = one bot identity. The agent never selects which bot to use — the environment determines identity. This prevents agents from accidentally crossing identity silos.

Override precedence (highest wins):
1. CLI flags (`--config /path/to/config.json`)
2. Env vars (`SLACKLINE_CONFIG`, `SLACKLINE_BOT_TOKEN`, `SLACKLINE_APP_TOKEN`)
3. Config file (`~/.config/slackline/config.json`)

Env vars allow running without a config file entirely (useful for CI/containers).

## Commands

### `slackline create` — Create a New Bot (Admin)

Creates a Slack app via the manifest API and walks an admin through the token collection flow. Requires an App Configuration Token (one-time bootstrap).

**First-time bootstrap:**
```
$ slackline create --init
No config token found. You need to generate one:

  1. Go to https://api.slack.com/apps
  2. Scroll to "Your App Configuration Tokens"
  3. Click "Generate Token" for your workspace

Paste your config token (starts with xoxe-): xoxe-████
Paste your refresh token (starts with xoxe-): xoxe-████

✓ Config token stored.
```

**Per-bot creation:**
```
$ slackline create --name "my-bot" --workspace "My Workspace"

Refreshing config token... ✓
Creating Slack app "my-bot"... ✓ (app_id: A0123ABCDEF)

Step 1: Install the app
  → https://api.slack.com/apps/A0123ABCDEF/install-on-team
  Click "Allow", then press Enter.

Step 2: Paste Bot Token (xoxb-)
  → https://api.slack.com/apps/A0123ABCDEF/oauth
  Token: xoxb-████

Step 3: Paste App Token (xapp-)
  → https://api.slack.com/apps/A0123ABCDEF/general
  Click "Generate Token", add connections:write scope.
  Token: xapp-████

✓ my-bot ready. Config written to ~/.config/slackline/config.json
```

**Manifest template** embedded in the binary with scopes:
- `chat:write`, `channels:read`, `groups:read`, `channels:history`, `groups:history`
- `app_mentions:read`, `im:history`, `reactions:read` (for Socket Mode events)

Socket Mode enabled: `settings.socket_mode_enabled: true`
Event subscriptions: `app_mention`, `message.im`, `reaction_added`

Config token is auto-rotated via `tooling.tokens.rotate` before each creation run.

### `slackline init` — Configure a Machine (Developer)

For developers who already have tokens (provisioned by an admin). No manifest API interaction.

```
$ slackline init
Workspace URL (e.g. https://myteam.slack.com): https://myteam.slack.com
Bot Token (xoxb-): xoxb-████
App Token (xapp-): xapp-████

✓ Config written to ~/.config/slackline/config.json
  Bot: my-bot (via auth.test)
  Workspace: My Workspace
```

### `slackline send` — Send a Message

```bash
slackline send --channel "#ops" --message "simple message"
echo 'Message with special chars!' | slackline send --channel "#ops"
slackline send --channel C01OPS12345 --thread 1769756026.624319 --message "thread reply"
```

**Channel flag** accepts: `#channel-name` (resolved via API), raw channel ID (`C...`), or Slack URL.

**Stdin for message content:** When `--message` is omitted, reads from stdin. This avoids Claude Code's Bash tool escaping `!` as `\!` in command arguments. Heredocs and single-quoted echo work cleanly:

```bash
slackline send --channel "#ops" <<'EOF'
Deploy complete! All tests passing!
EOF
```

**Output:**
```json
{"ok":true,"channel":"C01OPS12345","ts":"1769756026.624319"}
```

### `slackline read` — Read Messages

```bash
slackline read --channel "#ops" --limit 10
slackline read --channel "#ops" --thread 1769756026.624319
slackline read --channel "#ops" --since 2026-03-16T10:00:00Z
```

**Output** is JSONL — one message per line, compact, nulls/empty fields stripped:
```json
{"ts":"1234","user":"U0123","text":"hello","thread_ts":"1234"}
{"ts":"1235","user":"U0456","text":"world"}
```

### `slackline ask` — Send and Wait for Reply

```bash
echo 'Ready to deploy?' | slackline ask --channel "#ops" --timeout 300 --poll 10
```

Sends the message, polls the thread every `--poll` seconds (default 10), prints replies when someone responds. Exits 0 on reply, 1 on timeout (default 300s).

### `slackline listen` — Real-Time Event Stream

Connects via Socket Mode WebSocket, emits JSONL events to stdout.

```bash
slackline listen
```

**Output:**
```json
{"type":"mention","channel":"C01OPS12345","user":"U0123","text":"hey @my-bot check the logs","ts":"1769756026.624319","thread_ts":"1769756026.624319"}
{"type":"dm","channel":"D01DIRECTMSG","user":"U0456","text":"can you review this PR?","ts":"1769756030.111111"}
{"type":"reaction","channel":"C01OPS12345","user":"U0123","emoji":"eyes","item_ts":"1769756026.624319"}
```

**Event types (v1):** `mention`, `dm`, `reaction`

**Lifecycle:**
- Runs until stdin closes or SIGTERM/SIGINT
- Reconnects automatically on WebSocket drop (slack-go handles reconnection)
- Status logged to stderr (`connected`, `reconnecting`, `disconnected`)
- Stdout contains only valid JSONL, ever
- Exit 0 on clean shutdown, exit 1 on unrecoverable error

### `slackline channels` — List Channels

```bash
slackline channels          # channels the bot is in
slackline channels --all    # all visible channels
slackline channels --json   # JSON output
```

**Default output:**
```
C01GENERAL12  #everybody       General announcements
C01OPS12345  #ops             Operations
C01BUGS12345  #broken-things   Bug reports
```

### `slackline auth status` — Check Configuration

```
Bot:       my-bot
Workspace: My Workspace (T012AB3CD45)
Bot Token: xoxb-...XXXX (valid)
App Token: xapp-...XXXX (valid)
Config:    ~/.config/slackline/config.json
```

Validates tokens by calling `auth.test` (bot token) and `apps.connections.open` (app token).

## Channel Resolution

All commands accepting `--channel` resolve dynamically:
- `#channel-name` → resolved via `conversations.list`
- `C01OPS12345` → raw ID, used directly
- `https://team.slack.com/archives/C01OPS12345` → ID extracted from URL

Channel name → ID mappings cached in memory for the process lifetime. No on-disk cache.

## Config File

Located at `~/.config/slackline/config.json` (file `0600`, directory `0700`):

```json
{
  "workspace": {
    "name": "My Workspace",
    "team_id": "T012AB3CD45",
    "url": "https://myteam.slack.com"
  },
  "bot": {
    "name": "my-bot",
    "app_id": "A0123ABCDEF",
    "bot_token": "xoxb-...",
    "app_token": "xapp-..."
  }
}
```

Provisioning credentials (`config_token`, `refresh_token`) are only written when `slackline create` is run. They are not required for normal operation — `send`, `read`, `ask`, `listen`, and `channels` only need the `bot` section.

**Future improvement:** Token storage should migrate to system keyring (macOS Keychain, Linux libsecret) or encrypted file for better security. The config module should be designed with a storage backend interface to make this swap straightforward.

## Skill

The `slack-messaging` skill is replaced with a `slackline` skill that teaches agents how to use the binary.

```yaml
---
name: slackline
description: Use when asked to send or read Slack messages, check Slack channels, listen for mentions, or interact with a Slack workspace.
user-invocable: false
allowed-tools: Bash(slackline:*)
---
```

**Key properties:**
- No hardcoded channel table — agents use `slackline channels` for discovery
- No auth instructions — assumes slackline is configured
- Documents stdin pattern for messages with special characters
- Documents when to use `ask` vs `send` + `read` vs `listen`

## Cross-Platform

- **macOS (ARM + Intel):** Primary development target
- **Linux (AMD64 + ARM64):** Headless/CI/container support
- **Windows:** Not a priority but Go cross-compilation makes it trivial to add later

Distributed as precompiled binaries via GitHub Releases + `go install`.

## Dependencies

- `github.com/slack-go/slack` — Slack API client + Socket Mode
- `github.com/spf13/cobra` — CLI framework
- Go standard library for everything else (JSON, HTTP, OS, signals)

No CGo, no system library dependencies. Static binary.

## What This Replaces

| Current | Slackline |
|---------|-----------|
| `shaharia-lab/slackcli` binary | `slackline` binary |
| `~/.config/slackcli/workspaces.json` | `~/.config/slackline/config.json` |
| `~/.config/claude-slack/token` (duplicate) | Eliminated — single config |
| `scripts/extract-tokens` | `slackline init` |
| `scripts/slack-ask` | `slackline ask` |
| Hardcoded channel table in SKILL.md | `slackline channels` |
| `curl` for reading messages | `slackline read` |
| No real-time events | `slackline listen` |
| No provisioning automation | `slackline create` |

## Out of Scope (v1)

- Multi-bot management in a single config (by design — one identity per install)
- MCP server mode (agents use skill + CLI, not MCP)
- File uploads/downloads
- Slack workflow/automation triggers
- Message formatting beyond plain text (Block Kit etc.)
- Token rotation for bot tokens (xoxb tokens don't expire)
- Batch app pool provisioning
