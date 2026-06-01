---
name: using-slack
description: Use when sending or reading Slack messages, checking channels, watching for bot events (mentions, DMs, reactions, thread replies), adding reactions, or downloading Slack files from the command line via the slackline CLI.
allowed-tools: Bash(slackline:*)
---

# Using Slack via slackline

`slackline` gives an AI agent its own Slack identity. One binary, one config file, one bot. Messages appear from the bot, not from a human.

## Prerequisites

- `slackline` on PATH (`~/.local/bin/slackline`). Install: see the repo README.
- A configured bot: `slackline auth status` should print `Bot:` and a `(valid)` token. If not, run `slackline init` (needs already-provisioned `xoxb-`/`xapp-` tokens). To create a brand-new bot, use the `slackline-provision-bot` skill.
- `jq` for parsing JSONL output.

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
| Read since a time | `slackline read --channel '#ops' --since 2026-03-17T00:00:00Z` |
| Read a thread | `slackline read --channel '#ops' --thread <ts> --limit 20` |
| Ask and wait for a reply | `slackline ask --channel '#ops' --message 'q?' --timeout 120` |
| React to a message | `slackline react add --channel '#ops' --ts <ts> --emoji white_check_mark` |
| Remove a reaction | `slackline react remove --channel '#ops' --ts <ts> --emoji white_check_mark` |
| List the bot's channels | `slackline channels` (`--all` for every visible channel, `--json` for scripting) |
| Download a file | `slackline download --file <F...> --out path` (`--out -` streams to stdout) |
| Stream live events | `slackline listen` |

Run `slackline <command> --help` for every flag.

## Reading messages

`read` emits JSONL, one message per line, in **chronological order — oldest first, so the newest message is the LAST line.** Get the newest with `tail -n1`:

```bash
slackline read --channel '#ops' --limit 5 | tail -n1
```

- `--limit N` returns the most recent N messages (counted back from the newest), still printed oldest→newest. This holds for both channel and thread reads, so the latest reply is always included.
- A thread read **includes the parent message**, and the parent counts toward `--limit`. To confirm a line is an actual reply, check that its `ts` differs from the thread parent `ts`.
- The `user` field is a Slack user **ID** (`U...`), not a display name — slackline does not resolve names.

## Ask: reply vs. timeout

`ask` posts a message, then polls the thread for a reply from someone other than the bot.

- **Got a reply:** exit code `0`, the reply printed as JSONL on stdout.
- **Timed out:** exit code `1`, nothing on stdout, `{"error":"timeout","detail":"..."}` on stderr.

Exit `1` also covers auth/API errors, so to be certain it was a timeout, inspect the stderr `"error"` field rather than relying on the exit code alone. The poll interval (`--poll`, default 10s) means a reply is only noticed on a tick and the wait can overshoot `--timeout` by up to one interval; use `--poll 5` for tighter windows.

## Listening for events

`slackline listen` streams events as JSONL to **stdout**; connection status (`connected`, `reconnecting`, `disconnected`) goes to **stderr** as plain text. Keep them separate so status lines don't corrupt JSON parsing. Requires both bot and app tokens (Socket Mode).

Default event types (no flags): `mention`, `dm`, `reaction_added`, `reaction_removed`, and `thread_reply` for threads the bot started. Events from the bot itself are filtered unless `--include-bot-self`. `--all-messages` is a firehose of every message in every channel the bot is in.

Each event has `type`, `channel`, `user`, `ts`, often `thread_ts`, and a `files` array when attachments are present. Watch for mentions and react to each:

```bash
slackline listen | while IFS= read -r line; do
  [ "$(jq -r .type <<<"$line")" = "mention" ] || continue
  ch=$(jq -r .channel <<<"$line")
  ts=$(jq -r .ts <<<"$line")
  slackline react add --channel "$ch" --emoji eyes --ts "$ts"
done
```

`react` is idempotent (re-reacting is a safe no-op) and `--emoji` takes the name **without colons** (`white_check_mark`, not `:white_check_mark:`).

## Files

A file shows up as a `files` array on an event, carrying `id`/`name`/`mimetype`/`size`/`title` but **no URL**. Download it by ID with `slackline download --file <id> --out <path>`. Note: file uploads arrive as a `message`/`dm`/`thread_reply` event (Slack subtype `file_share`), **never as a `mention`** — to catch file-bearing messages in a channel you must run `listen --all-messages`.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Slack API error (includes `ask` timeout) |
| 2 | Auth error (invalid/revoked token) |
| 3 | Config error (missing file/token) |
| 4 | Usage error (bad flags) |

All errors are written to stderr as JSON: `{"error":"...","detail":"..."}`.

## Common mistakes

- **Taking the first line as newest.** `read` is oldest-first; the newest is the last line.
- **Reading a channel the bot hasn't joined.** Returns `not_in_channel`; the bot must be invited.
- **Parsing `listen` stdout as if it included status.** Status is on stderr; only stdout is JSONL.
- **Filtering `listen` for `mention` to catch file uploads.** Files come on `message`-family events with `--all-messages`, not on mentions.
- **Treating any non-zero `ask` exit as a timeout.** Check the stderr `"error"` field.
- **Passing emoji with colons.** Use the bare name.
