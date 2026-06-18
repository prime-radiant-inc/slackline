---
name: using-slack
description: Use when sending, reading, or reacting to Slack messages, watching for bot events (mentions, DMs, reactions, thread replies), downloading Slack files, or provisioning/creating/deploying a new Slack bot — all from the command line via the slackline CLI.
allowed-tools: Bash(slackline:*)
---

# Using Slack via slackline

`slackline` gives an AI agent its own Slack identity. One binary, one config file, one bot. Messages appear from the bot, not from a human.

## Prerequisites

- `slackline` on PATH (`~/.local/bin/slackline`). Install: see the repo README.
- A configured bot: `slackline auth status` should print valid bot and app tokens. Use `slackline auth whoami` when you only need the validated bot/workspace identity. If not configured, run `slackline init` (needs already-provisioned `xoxb-`/`xapp-` tokens). To create a brand-new bot, see the Provisioning section below.
- `jq` only if you want to reshape output — most tasks no longer need it.

## The membership gotcha (read this first)

The bot can only `read`, `ask`, and `listen` in channels it has **joined**. Otherwise these fail with `{"error":"not_in_channel"}`. `slackline channels` lists the channels it's in. Fix: a human runs `/invite @bot-name` in the target channel. (Posting with `send` to a public channel often works without joining; reading never does.)

`--channel` accepts a name (`#ops`), an ID (`C...`), or a pasted Slack URL.

## Commands

| Task | Command |
|------|---------|
| Post a message | `slackline send --channel '#ops' --message 'text'` |
| Post from stdin | `echo text \| slackline send --channel '#ops'` |
| Reply in a thread | `slackline send --channel '#ops' --thread <ts> --message 'text'` |
| Attach files | `slackline send --channel '#ops' --attach a.png --attach b.pdf` |
| Read recent messages | `slackline read --channel '#ops' --limit 10` |
| Read the single newest message | `slackline read --channel '#ops' --limit 1` |
| Read since a time | `slackline read --channel '#ops' --since 2026-03-17T00:00:00Z` |
| Read a thread (newest reply: `--limit 1`) | `slackline read --channel '#ops' --thread <ts> --limit 20` |
| Ask and wait for a reply | `slackline ask --channel '#ops' --message 'q?' --timeout 120` |
| React to a message | `slackline react add --channel '#ops' --ts <ts> --emoji white_check_mark` |
| List the bot's channels | `slackline channels` (`--all`, `--json`) |
| Download a file | `slackline download --file <F...> --out path` (`--out -` to stdout) |
| Stream live events | `slackline listen` (filter with `--type`) |

Run `slackline <command> --help` for every flag.

## Reading messages

`read` emits compact text, oldest-first. To get just the newest, use `--limit 1` (it returns the most recent N counted from the newest) — no `tail` needed:

```bash
slackline read --channel '#ops' --limit 1
```

- Text output starts with `<ts> <user>`, followed by `thread=<ts>` when present, then message text. Attached files appear as indented `file <id> ...` continuation lines.
- This holds for threads too: `read --thread <ts> --limit 1` is the newest reply. A thread read includes the parent, which counts toward `--limit`; a line is a real reply when its timestamp differs from the thread parent timestamp.
- The user token is a Slack user **ID** (`U...`), not a display name.
- Use `--format json` when a script needs exact JSON fields.

## Ask: reply vs. timeout

`ask` posts a message, then polls the thread for a reply from someone other than the bot.

- **Got a reply:** exit `0`, the reply printed as JSONL on stdout.
- **Timed out:** exit `5`, `{"error":"timeout",...}` on stderr.
- Other failures (API/auth/config) use exit `1`/`2`/`3`.

Branch cleanly on the exit code — no stderr parsing:

```bash
if out=$(slackline ask --channel '#ops' --message 'ready?' --timeout 120); then
  echo "reply: $out"
elif [ $? -eq 5 ]; then
  echo "timed out"
else
  echo "error"
fi
```

The poll interval (`--poll`, default 10s) means the wait can overshoot `--timeout` by up to one interval; use `--poll 5` for tighter windows.

## Listening for events

`slackline listen` streams compact event lines to **stdout**; connection status goes to **stderr** (`connecting`, `connected` = websocket open, `ready` = subscribed and events will now flow, `reconnecting`, `disconnected`). Wait for `ready` before expecting events. Requires both bot and app tokens (Socket Mode). Socket Mode connection failures exit non-zero with `socket_mode_failed`.

Use `--type` to emit only what you care about:

```bash
slackline listen --type mention | while read -r type ch user ts text; do
  slackline react add --channel "$ch" --emoji eyes --ts "$ts"
done
```

- Valid `--type` values: `mention`, `dm`, `thread_reply`, `channel_message`, `reaction`. Comma-separate for several (`--type mention,dm`). Unknown values error.
- `--type` is an emit-time filter, not a subscription widener: `--type thread_reply` still only sees the bot's own threads unless you add `--all-messages`. `channel_message` **requires** `--all-messages`.
- Default (no `--type`, no flags): `mention`, `dm`, `reaction`, and bot-parent `thread_reply`. Bot self-events are filtered unless `--include-bot-self`.

A reaction event is a single `reaction` line with an action:

```text
reaction added C... U... item=... thumbsup
```

Use `--format json` if you need the older JSONL event objects.

`react` is idempotent; `--emoji` takes the bare name (`white_check_mark`, not `:white_check_mark:`).

## Files

A file shows up as an indented `file <id> ...` line by default, or as a `files` array with `--format json` (`id`/`name`/`mimetype`/`size`/`title`, **no URL**). Download by ID with `slackline download --file <id> --out <path>`. File uploads arrive as a `dm`/`channel_message`/`thread_reply` event (Slack subtype `file_share`), **never as a `mention`** — to catch them in a channel, `listen --all-messages` (optionally `--type channel_message,thread_reply`).

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Slack API error |
| 2 | Auth error (invalid/revoked token) |
| 3 | Config error (missing file/token) |
| 4 | Usage error (bad flags) |
| 5 | Timeout (`ask` received no reply) |

All errors are written to stderr as JSON: `{"error":"...","detail":"..."}`.

## Provisioning a new bot

Creating a brand-new bot is an admin flow (browser automation for Slack's install + token-collection screens). See **`provisioning.md`** next to this file for the end-to-end recipe, and `copy-buttons.md` for the selector reference.

## Common mistakes

- **Reading a channel the bot hasn't joined.** Returns `not_in_channel`; the bot must be invited.
- **Parsing `listen` stdout as if it included status.** Status is on stderr; stdout is only event lines.
- **Filtering `listen` for `mention` to catch file uploads.** Files come on `message`-family events with `--all-messages`, not on mentions.
- **Passing emoji with colons.** Use the bare name.
- **Matching `reaction_added`/`reaction_removed`.** It's one `reaction` event now — branch on `action`.

---
<!-- doc-audit:last-reviewed -->
_Last reviewed: 2026-06-11 · commit `e4f4b21` · verified against code (2 claims deferred to review)._
