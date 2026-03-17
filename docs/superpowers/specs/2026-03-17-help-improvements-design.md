# Help & Usability Improvements — Design Spec

**Date:** 2026-03-17
**Status:** Approved

## Problem

Two pain points for both human and agent users:

1. Bare `./slackline` invocation gives generic help with no "what do I do first?" guidance.
2. Subcommands invoked without required flags (e.g. `slackline ask`) produce `{"error":"unknown_error",...}` because cobra's "required flag not set" error is not a `*errs.SlackError` and falls through to the generic catch-all in `Execute()`.

## Goals

- Fix the `unknown_error` bug for missing required flags.
- Make help text actionable for both humans and agents (exit codes, output format, examples).
- Make `init` non-interactive via env vars for agent use.
- Reorder exit codes to match Unix convention (2 = usage/misuse).

## Changes

### 1. Exit Code Reorder — `errs/errors.go`

Reorder the exit code constants to match Unix convention (2 = usage/misuse):

| Old | New | Meaning |
|-----|-----|---------|
| 0 | 0 | success |
| 1 | 1 | Slack API error |
| 2 | 2 | usage error (bad flags, missing args) — matches bash/grep convention |
| 3 | 3 | config error |
| 4 | 4 | auth error |

Current constants:
```go
Success  = 0
SlackAPI = 1
Auth     = 2
Config   = 3
Usage    = 4
```

New constants:
```go
Success  = 0
SlackAPI = 1
Usage    = 2
Config   = 3
Auth     = 4
```

Update all callsites that reference `errs.Auth` and `errs.Usage` — the integer values change, but the named constants don't, so only the constant definitions need updating. All callers using the named constants will automatically use the new values.

### 2. Bug Fix — `Execute()` in `cmd/root.go`

cobra's "required flag(s) not set" error is a plain `error`, not a `*errs.SlackError`. It falls through to the generic catch-all which emits `unknown_error` with exit code 1.

Fix: before the generic catch-all, check if `err.Error()` contains `"required flag(s)"` (cobra's internal error message prefix; not a public API, but pragmatically stable — a test must cover this path to catch regressions if cobra changes its message).

Test requirement: invoke a command (`ask`, `send`, or `read`) without its required `--channel` flag. Assert exit code 2. Assert stderr contains JSON with `"error":"usage_error"`. This test locks the behavior and will catch cobra message changes.

If matched, emit:

```json
{"error":"usage_error","detail":"required flag(s) \"channel\" not set — run 'slackline <cmd> --help' for usage"}
```

The `detail` value is constructed as `err.Error() + " — run 'slackline <cmd> --help' for usage"`. The command name comes from `rootCmd.Execute()`'s context — use `err.Error()` as-is and append the suffix without substituting the command name (cobra's message already names the flag). Return **exit code 2** (`errs.Usage`).

### 2. Root Command — `cmd/root.go`

The current `Long` is a single sentence. Extend it — do not replace it — with a getting-started block, error format note, and exit code reference:

```
A CLI tool for AI agents to send messages, read channels, and listen for events in Slack.

Getting started:
  slackline init       Configure with existing bot and app tokens
  slackline create     Provision a new Slack app (requires admin token)

All errors are written to stderr as JSON: {"error":"<code>","detail":"<message>"}

Exit codes: 0 success, 1 Slack API error, 2 usage error, 3 config error, 4 auth failure
```

### 3. Per-Command `Long` and `Example` Fields

Each command gets its `Long` description extended with output format and exit codes, and an `Example` field added. cobra renders `Example` automatically in `--help` output.

#### `send`

```
Long: Send a message to a channel. Message can be passed via --message or piped via stdin.
Note: --message is not marked required because stdin is a valid alternative.
Missing message is caught at runtime with exit code 2.

Output: {"ok":true,"channel":"C...","ts":"..."}
Exit codes: 0 success, 1 Slack API error, 2 usage, 3 config, 4 auth

Example:
  slackline send --channel '#ops' --message 'deploying now'
  echo 'deploying now' | slackline send --channel C1234567890
  slackline send --channel '#ops' --message 'reply' --thread 1234567890.123456
```

#### `ask`

```
Long: Sends a message to a channel and polls the thread for replies from other users.
Exits 0 when a reply is received. Exits 1 on timeout or Slack API error; distinguish
via the JSON error field ({"error":"timeout",...} vs {"error":"poll_failed",...}).
Note: --message is not marked required because stdin is a valid alternative.

Output: JSONL — one {"ts":"...","user":"...","text":"..."} per reply line
Exit codes: 0 reply received, 1 timeout or Slack API error, 2 usage, 3 config, 4 auth

Example:
  slackline ask --channel '#ops' --message 'ready?' --timeout 60
  echo 'ready?' | slackline ask --channel '#ops'
```

#### `read`

```
Long: Read messages from a channel or thread. Output is JSONL (one message per line).
--since accepts RFC 3339 timestamps (e.g. 2024-01-01T00:00:00Z).

Output: JSONL — one {"ts":"...","user":"...","text":"..."} per message line
Exit codes: 0 success, 1 Slack API error, 2 usage, 3 config, 4 auth

Example:
  slackline read --channel '#ops' --limit 50
  slackline read --channel '#ops' --thread 1234567890.123456
  slackline read --channel '#ops' --since 2024-01-01T00:00:00Z
```

Also update the `--since` flag description and the runtime error message in `read.go` from "ISO 8601" to "RFC 3339" to match Go's `time.RFC3339` parser.

#### `listen`

```
Long: Connect via Socket Mode and stream @mentions, DMs, and reactions as JSONL to stdout.
Requires app token (xapp-) for Socket Mode. Streams until interrupted.
No usage errors (no required flags).

Output: JSONL to stdout — {"type":"...","user":"...","text":"...","channel":"...","ts":"..."}
Exit codes: 0 clean exit, 1 connection error, 3 config, 4 auth

Example:
  slackline listen
  slackline listen 2>/dev/null | jq .
```

#### `channels`

```
Long: List Slack channels visible to the bot. Defaults to channels the bot has joined.
JSON output includes topic; the table omits topic for display width.

Output: table (ID, NAME, PURPOSE) by default; with --json: array of
        {"id":"C...","name":"...","topic":"...","purpose":"..."}
Exit codes: 0 success, 1 Slack API error, 3 config, 4 auth

Example:
  slackline channels
  slackline channels --json
  slackline channels --all --json
```

#### `auth status`

```
Long: Check whether configured tokens are valid and display bot identity.
Token validation errors are reported in the output text — this command always exits 0
unless the config file is missing or unreadable (exit 3). Exit codes 1, 2, and 4 are never returned.

Output: plain text — bot name, workspace, token status, config path
Exit codes: 0 success (including invalid tokens), 3 config error
Note: exit codes 1, 2, and 4 are never returned by this command.

Example:
  slackline auth status
```

#### `init`

```
Long: Set up slackline on a new machine using tokens provisioned by an admin.
Supports non-interactive mode via environment variables for agent use.

Environment variables (non-interactive mode):
  SLACKLINE_BOT_TOKEN     Bot token (xoxb-)
  SLACKLINE_APP_TOKEN     App token (xapp-)
  SLACKLINE_WORKSPACE_URL Workspace URL, e.g. https://myteam.slack.com (optional)

If both SLACKLINE_BOT_TOKEN and SLACKLINE_APP_TOKEN are set, all stdin prompts are skipped.
If exactly one is set, init exits with a usage error (exit 2).
Token validation via auth.test is performed in both interactive and non-interactive modes.

Exit codes: 0 success, 2 usage, 3 config, 4 auth

Example:
  slackline init
  SLACKLINE_BOT_TOKEN=xoxb-... SLACKLINE_APP_TOKEN=xapp-... slackline init
```

#### `create`

```
Long: Create a Slack app via the manifest API. Requires an App Configuration Token.
Interactive — not suitable for agent use.

Exit codes: 0 success, 1 Slack API error, 2 usage, 3 config, 4 auth

Example:
  slackline create --name mybot
  slackline create --init
```

### 4. Non-Interactive `init` Implementation

`runInit` in `cmd/initcmd.go` checks env vars **before** touching stdin:

1. Read `SLACKLINE_BOT_TOKEN`, `SLACKLINE_APP_TOKEN`, `SLACKLINE_WORKSPACE_URL` from env.
2. If both bot and app tokens are set: use them directly. Do not read from stdin at all. Apply the same prefix validation as the interactive path: bot token must start with `xoxb-`, app token must start with `xapp-`. Return `usage_error` (exit 2) if either prefix check fails.
3. If exactly one is set: return `&errs.SlackError{Code: errs.Usage, Err: "missing_token", Detail: "set both SLACKLINE_BOT_TOKEN and SLACKLINE_APP_TOKEN for non-interactive mode"}`.
4. If neither is set: fall through to existing interactive flow (no change).

In non-interactive mode, `auth.test` is called on the bot token exactly as in interactive mode. `SLACKLINE_WORKSPACE_URL` maps to the same `Workspace.URL` field as the interactive URL prompt. `Workspace.Name` and `Workspace.TeamID` are populated from `auth.test` response as usual.

All errors from non-interactive init are returned as `*errs.SlackError` (emitted as JSON to stderr by `Execute()`).

## Out of Scope

- Making `create` non-interactive (multi-step OAuth flow doesn't map cleanly to env vars).
- Machine-readable `--help-json` flag.
- Changes to output formats of any command.
- Distinct exit code for timeout (currently exit 1, distinguished by JSON `error` field).
