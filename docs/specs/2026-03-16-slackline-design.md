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

[PRI-722: Self-service Slack bot provisioning for Claude instances](https://linear.app/prime-radiant/issue/PRI-722/self-service-slack-bot-provisioning-for-claude-instances)

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

**Key dependency:** `github.com/slack-go/slack` — Go library with full Socket Mode support, manifest APIs, OAuth token exchange, and all messaging/history methods. 4,900+ stars, actively maintained (v0.19.0, March 2026). Pre-1.0: pin version in `go.mod` and wrap volatile types (especially manifest structs) in an internal adapter to insulate from breaking changes in minor versions.

### Identity Model

One slackline installation = one bot identity. The agent never selects which bot to use — the environment determines identity. This prevents agents from accidentally crossing identity silos.

"Installation" means one config file. Multiple configs on the same machine are possible via `SLACKLINE_CONFIG` env var or `--config` flag, but each agent process sees exactly one identity.

Override precedence (highest wins):
1. CLI flags (`--config /path/to/config.json`)
2. Env vars (`SLACKLINE_CONFIG`, `SLACKLINE_BOT_TOKEN`, `SLACKLINE_APP_TOKEN`)
3. Config file (`~/.config/slackline/config.json`)

Env vars allow running without a config file entirely (useful for CI/containers). When running with env vars only, metadata (bot name, workspace name) is derived from `auth.test` API call on first use.

## Error Handling

All commands follow a consistent error contract:

**Exit codes:**
- `0` — success
- `1` — Slack API error (channel not found, not in channel, rate limited, etc.)
- `2` — auth error (invalid token, token revoked)
- `3` — config error (missing config, invalid config, missing required fields)
- `4` — usage error (bad flags, missing required args)

**Error output:** Errors are written to stderr as JSON:
```json
{"error":"channel_not_found","detail":"Could not find channel #nonexistent"}
```

Success output goes to stdout. An agent can check exit code and parse stderr on failure.

**Token validation:** On any auth error (HTTP 401, `token_revoked`, `invalid_auth`), all commands print a clear message to stderr: `"Token invalid or revoked. Run 'slackline init' to reconfigure."`

## Commands

### `slackline create` — Create a New Bot (Admin)

Creates a Slack app via the manifest API and walks an admin through the token collection flow. Requires an App Configuration Token.

**Important:** App Configuration Tokens expire after 12 hours. The refresh token can renew them via `tooling.tokens.rotate`, but if slackline hasn't been used for >12 hours, it auto-rotates on next run. If the refresh token itself has expired (after extended inactivity), `create` detects this and prompts the admin to re-generate at api.slack.com. This is NOT a one-time setup — it's a recurring (but infrequent) tax for the admin who provisions bots.

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
$ slackline create --name "drew-claude"

Refreshing config token... ✓
Creating Slack app "drew-claude"... ✓ (app_id: A0123ABCDEF)

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

✓ drew-claude ready. Config written to ~/.config/slackline/config.json
```

Note: if the workspace has app approval enabled, the install step may require admin approval first. `create` detects this and prints guidance.

**Manifest template** embedded in the binary with scopes:
- `chat:write`, `channels:read`, `groups:read`, `channels:history`, `groups:history`
- `app_mentions:read`, `im:history`, `im:read`, `reactions:read` (for Socket Mode events)
- `users:read` (for user ID → display name resolution)

Socket Mode enabled: `settings.socket_mode_enabled: true`
Event subscriptions: `app_mention`, `message.im`, `reaction_added`

The `--workspace` flag is a label written to the config file for display purposes. The workspace is determined by the config token, not this flag.

### `slackline init` — Configure a Machine (Developer)

For developers who already have tokens (provisioned by an admin). No manifest API interaction.

```
$ slackline init
Workspace URL (e.g. https://myteam.slack.com): https://prime-radiant-inc.slack.com
Bot Token (xoxb-): xoxb-████
App Token (xapp-): xapp-████

✓ Config written to ~/.config/slackline/config.json
  Bot: drew-claude (via auth.test)
  Workspace: Prime Radiant
```

Token prefix validation: rejects tokens that don't start with the expected prefix (`xoxb-` for bot, `xapp-` for app) and tells the user what's wrong.

**Migration from slackcli:** Existing `xoxb-` tokens from slackcli are reusable — they're scoped to the Slack app, not the CLI tool. `init` accepts them directly. Users don't need to re-provision.

### `slackline send` — Send a Message

```bash
slackline send --channel "#ops" --message "simple message"
echo 'Message with special chars!' | slackline send --channel "#ops"
slackline send --channel C0A8LJZQSAX --thread 1769756026.624319 --message "thread reply"
```

**Channel flag** accepts: `#channel-name` (resolved via API), raw channel ID (`C...`), or Slack URL.

**Stdin for message content:** When `--message` is omitted, reads from stdin. This avoids Claude Code's Bash tool escaping `!` as `\!` in command arguments. Heredocs and single-quoted echo work cleanly:

```bash
slackline send --channel "#ops" <<'EOF'
Deploy complete! All tests passing!
EOF
```

If `--message` is omitted and stdin is a TTY (not piped), print usage error and exit 4. Don't block waiting for input.

Messages are sent as-is — Slack will parse mrkdwn formatting (`*bold*`, `_italic_`, etc.) automatically. No Block Kit support in v1.

**Output:**
```json
{"ok":true,"channel":"C0A8LJZQSAX","ts":"1769756026.624319","thread_ts":"1769756026.624319"}
```

`thread_ts` is included when the message is a thread reply.

### `slackline read` — Read Messages

```bash
slackline read --channel "#ops" --limit 10
slackline read --channel "#ops" --thread 1769756026.624319
slackline read --channel "#ops" --since 2026-03-16T10:00:00Z
```

`--since` accepts ISO 8601 timestamps, converted internally to Unix epoch for `conversations.history`'s `oldest` parameter. Results are returned in chronological order (oldest first), reversing Slack's default reverse-chronological order.

`--limit` caps total messages returned. The implementation paginates internally if needed (Slack returns max 100 per call). Default limit: 20.

**Output** is JSONL — one message per line, compact, nulls/empty fields stripped:
```json
{"ts":"1234","user":"U0123","text":"hello","thread_ts":"1234"}
{"ts":"1235","user":"U0456","text":"world"}
```

### `slackline ask` — Send and Wait for Reply

```bash
echo 'Ready to deploy?' | slackline ask --channel "#ops" --timeout 300 --poll 10
```

Sends the message, polls the thread every `--poll` seconds (default 10) via `conversations.replies`.

**What counts as a reply:**
- Messages in the thread from any user OTHER than the bot itself (filtered by bot user ID from `auth.test`)
- Bot messages from other integrations DO count
- Reactions do not count
- Thread broadcasts count (they appear in `conversations.replies`)

**Multiple replies:** When one or more replies are detected, ALL new replies are printed as JSONL to stdout, then the command exits 0. It does not wait for additional replies — first batch wins.

Exits 1 on timeout (default 300s).

### `slackline listen` — Real-Time Event Stream

Connects via Socket Mode WebSocket, emits JSONL events to stdout.

```bash
slackline listen
```

**Output:**
```json
{"type":"mention","channel":"C0A8LJZQSAX","user":"U0123","text":"hey @drew-claude check the logs","ts":"1769756026.624319","thread_ts":"1769756026.624319"}
{"type":"dm","channel":"D0AA4MWTX45","user":"U0456","text":"can you review this PR?","ts":"1769756030.111111"}
{"type":"reaction","channel":"C0A8LJZQSAX","user":"U0123","emoji":"eyes","item_ts":"1769756026.624319"}
```

**Event types (v1):**

| Type | Fields (guaranteed) | Optional Fields |
|------|-------------------|-----------------|
| `mention` | `type`, `channel`, `user`, `text`, `ts` | `thread_ts` |
| `dm` | `type`, `channel`, `user`, `text`, `ts` | `thread_ts` |
| `reaction` | `type`, `channel`, `user`, `emoji`, `item_ts` | |

Note: `reaction` events contain only the emoji and the timestamp of the reacted-to message. They do NOT include the message text. The consumer must call `slackline read` if it needs the message content.

**Self-message filtering:** Events originating from the bot itself are silently dropped. This prevents infinite loops where the agent hears its own messages and responds to them.

**Message edits and deletes:** Dropped in v1. Only new messages and reactions are emitted.

**Event delivery guarantees:** Socket Mode is at-most-once during reconnections. Events arriving while the WebSocket is disconnected are lost. For critical workflows, prefer `slackline read --since` (polling) which provides durable history. `listen` is for responsiveness, not reliability.

**Lifecycle:**
- Runs until stdin closes or SIGTERM/SIGINT
- Reconnects automatically on WebSocket drop (slack-go handles reconnection)
- Status logged to stderr (`connected`, `reconnecting`, `disconnected`)
- Stdout contains only valid JSONL, ever
- Exit 0 on clean shutdown, exit 1 on unrecoverable error

### `slackline channels` — List Channels

```bash
slackline channels          # channels the bot is in
slackline channels --all    # all public channels + private channels bot is in
slackline channels --json   # JSON output
```

Note: `--all` lists all public channels plus private channels the bot has been invited to. It cannot see private channels it's not a member of — this is a Slack permission constraint.

**Default output:**
```
C0A2GP2FRRC  #everybody       General announcements
C0A8LJZQSAX  #ops             Operations
C0AAPRRJFSL  #broken-things   Bug reports
```

### `slackline auth status` — Check Configuration

```
Bot:       drew-claude
Workspace: Prime Radiant (T0A2XMY5117)
Bot Token: xoxb-...XXXX (valid)
App Token: xapp-...XXXX (valid)
Config:    ~/.config/slackline/config.json
```

Validates bot token via `auth.test`. App token validation uses `auth.test` with the app token (not `apps.connections.open`, which has the side effect of opening a WebSocket connection).

## Channel Resolution

All commands accepting `--channel` resolve dynamically:
- `#channel-name` → resolved via `conversations.list` (paginated, cached in-memory)
- `C0A8LJZQSAX` → raw ID, used directly
- Slack URL → ID extracted from URL path

**Scaling note:** Channel name resolution via `conversations.list` requires paginating through all channels to find a match. This is fine for small workspaces but slow on large ones (1000+ channels). For automated workflows, prefer passing raw channel IDs. The skill should teach agents to resolve a channel name once via `slackline channels` and reuse the ID.

Channel name → ID mappings cached in memory for the process lifetime. No on-disk cache — keeps things simple and avoids stale mappings.

**Ambiguous matches:** If multiple channels match the same name (e.g., archived + active), prefer the active (non-archived) channel. If still ambiguous, error with the list of matches and ask the user to pass a channel ID instead.

## Config File

Located at `~/.config/slackline/config.json` (file `0600`, directory `0700`):

```json
{
  "version": 1,
  "workspace": {
    "name": "Prime Radiant",
    "team_id": "T0A2XMY5117",
    "url": "https://prime-radiant-inc.slack.com"
  },
  "bot": {
    "name": "drew-claude",
    "app_id": "A0123ABCDEF",
    "bot_token": "xoxb-...",
    "app_token": "xapp-..."
  }
}
```

The `version` field enables future config migrations without guessing the schema.

Provisioning credentials (`config_token`, `refresh_token`) are only written when `slackline create` is run, in a separate file (`~/.config/slackline/provision.json`). They are admin-level credentials that can create new apps and should not be distributed to developer machines.

Normal operation (`send`, `read`, `ask`, `listen`, `channels`) only needs the `bot` section.

**Future improvement:** Token storage should migrate to system keyring (macOS Keychain, Linux libsecret) or encrypted file for better security. The config module should be designed with a storage backend interface to make this swap straightforward.

## Skill

The `slack-messaging` skill is replaced with a `slackline` skill that teaches agents how to use the binary.

```yaml
---
name: slackline
description: Use when asked to send or read Slack messages, check Slack channels, listen for mentions, or interact with a Slack workspace.
user-invocable: false
allowed-tools: Bash(slackline:*, echo:*, cat:*)
---
```

Note: `allowed-tools` includes `echo` and `cat` to support the stdin piping pattern (`echo 'msg' | slackline send`). Without these, the pipe-based approach for avoiding shell escaping would be blocked.

**Key properties:**
- No hardcoded channel table — agents use `slackline channels` for discovery, then reuse IDs
- No auth instructions — assumes slackline is configured
- Documents stdin pattern for messages with special characters
- Documents when to use `ask` vs `send` + `read` vs `listen`
- Documents error exit codes and how to handle failures
- The full skill body content is defined during implementation, not in this spec

## Cross-Platform

- **macOS (ARM + Intel):** Primary development target
- **Linux (AMD64 + ARM64):** Headless/CI/container support
- **Windows:** Not a priority but Go cross-compilation makes it trivial to add later

Distributed as precompiled binaries via GitHub Releases + `go install`.

## Dependencies

- `github.com/slack-go/slack` — Slack API client + Socket Mode (pinned version)
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

Existing `xoxb-` tokens from slackcli are reusable via `slackline init`.

## Out of Scope (v1)

- Multi-bot management in a single config (by design — one identity per install)
- MCP server mode (agents use skill + CLI, not MCP)
- File uploads/downloads
- Slack workflow/automation triggers
- Message formatting beyond plain text (Block Kit etc.)
- Token rotation for bot tokens (xoxb tokens don't expire by default; handle `token_revoked` errors gracefully)
- Batch app pool provisioning
- DM sending (receiving DMs via `listen` is supported; initiating DMs requires `conversations.open` + additional scopes, deferred to v2)
- Message edit/delete events in `listen`
