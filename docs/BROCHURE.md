# slackline

slackline gives an AI agent its own Slack identity in a single binary, so it can send messages, read channels, and stream real-time events.

## The problem

Your AI agent can do the work. It has no way to say so in Slack. Your team lives in channels, yet an agent that wants to post an update, answer a question in a thread, or notice when someone mentions it has to reach Slack through a webhook server, an OAuth flow, and a set of bot tokens you manage by hand. The connection becomes a project of its own, and it has to be built before the agent says a word.

## What you get

slackline gives the agent a Slack account it owns. Messages arrive from the agent under its own name, and the agent acts on its own:

- It posts to any channel or thread under its own name, and it can attach files. (`slackline send`)
- It reads recent channel and thread history when it needs context. (`slackline read`)
- It asks a question and waits for a person to reply, then returns the answer or reports a timeout. (`slackline ask`)
- It streams mentions, direct messages, reactions, and thread replies as they happen, one compact event line at a time. (`slackline listen`)
- It adds and removes emoji reactions. (`slackline react`)
- It downloads files people share with it. (`slackline download`)

Runtime output is short and agent-readable. `read` and `listen` default to compact text, while JSON remains available with `--format json` where a script needs exact fields.

## Using it

Once an administrator has provisioned a bot (see Running it), a developer points slackline at that bot's tokens and the agent is live. The whole catalog is one binary:

```
Available Commands:
  ask         Send a message and wait for a reply
  auth        Authentication commands
  channels    List Slack channels visible to the bot
  download    Download a file from Slack by file ID
  init        Configure slackline with existing tokens
  listen      Listen for real-time Slack events
  provision   Create a Slack app via the manifest API (machine-readable JSON output)
  react       Add or remove emoji reactions on a message
  read        Read messages from a Slack channel
  send        Send a message (and optionally one or more files) to a Slack channel
```

A first session takes three commands:

```bash
slackline init                                              # store the bot's tokens
slackline auth status                                       # confirm the identity
slackline send --channel '#ops' --message 'Deploy finished'
```

`send` returns the channel and timestamp of the posted message as JSON, so the agent can thread its next reply. To watch for work, the agent streams events and filters to the types it cares about:

```bash
slackline listen --type mention
```

Each event is one line on stdout. `listen` describes its own output:

```
Default text examples:
  mention C123 U123 100.001 <@UBOT> hello
  dm D123 U123 100.002 hello
  thread_reply C123 U123 100.003 thread=100.001 parent=UBOT reply
  reaction added C123 U123 item=100.001 thumbsup
  file F123 report.pdf 12345 application/pdf Q4 Report
```

The exact output each command returns is documented in the README. The command examples above are real; a live message response carries values from your own workspace, so producing one needs a configured bot.

## Running it

An owner deploys and operates slackline. The model is small enough to hold in your head.

Each bot is one binary plus one config file. A bot's identity is two Slack tokens: a bot token for everyday REST calls and an app token for the Socket Mode event stream. The config file lives at `~/.config/slackline/config.json` with `0600` permissions. You run several bots on one machine by pointing `SLACKLINE_CONFIG` at separate files.

You provision a new bot with `slackline provision`. Provisioning uses an administrator credential, the App Configuration Token, which stays in `provision.json` on the admin host and goes to no one else. The `using-slack` provisioning recipe walks an agent through the Slack install and token-collection screens step by step.

You stay in control of where the agent can act. The bot reads, asks, and listens only in channels a person has invited it to, and slackline filters out the bot's own events by default so a listening agent does not loop on itself. Running cost is the Slack app you register per bot and the routine care of its tokens.

## Who it's for, and who it isn't

slackline fits teams that run AI agents and want those agents working inside Slack: posting build and incident updates, watching channels for mentions, answering in threads, and reacting to events. It fits developers who want a scriptable Slack identity they can drive from a shell or a cron job.

It is built for agents and automation. People who want a chat client for themselves are served better by Slack's own apps. It is messaging and events, so a project that needs a full Slack app experience, with Block Kit interfaces and slash-command backends, needs an app framework instead. It runs on `darwin/arm64` and `linux/amd64`; other platforms are out of scope today.

## Limitations

- The agent reads, asks, and listens only in channels it has joined. A person adds it with `/invite @bot-name`. (user)
- Live event streaming holds a Socket Mode connection open and runs for as long as the process runs. (user)
- Provisioning a brand-new bot drives Slack's admin web pages through browser automation for the install and token steps. (operator)
- An app token's validity cannot be checked through Slack's REST API, so `auth status` reports it as configured rather than valid. (operator)
- Supported platforms today are `darwin/arm64` and `linux/amd64`. (operator)

## Getting started

- **Developer or agent:** get your bot's `xoxb-` and `xapp-` tokens from your administrator, run `slackline init`, then `slackline auth status`. The README "Setup" section has the details.
- **Owner or administrator:** provision a bot with `slackline provision`, then follow the `using-slack` provisioning recipe to install it and collect its tokens.
- **Contributor:** the architecture and package map live in `CLAUDE.md` (and `AGENTS.md`).

---

<!--
Where these claims come from (every capability cashes to a verified surface):
- send / read / ask / listen / react / download output: README.md "Commands" + "Event reference"; cmd/{send,read,ask,listen,react,download}.go
- command catalog block: `slackline --help`, with cobra boilerplate (`completion`, `help`) and the deprecated `create` stub trimmed
- listen event-fields block: real output of `slackline listen --help`; listen/events.go, listen/listener.go
- one binary + one config.json, 0600, SLACKLINE_CONFIG for multiple identities: README.md "Configuration"; config/config.go, cmd/root.go
- two tokens (bot xoxb- REST, app xapp- Socket Mode): README.md; docs/DICTIONARY.md
- provision + App Configuration Token in provision.json on the admin host: README.md "Provisioning a new bot"; skills/using-slack/provisioning.md; cmd/provision.go
- invite-to-channel requirement, self-event filtering: skills/using-slack/SKILL.md "membership gotcha"; listen/listener.go
- app token not REST-validatable / auth status "(configured)": README.md; cmd/auth.go
- supported platforms darwin/arm64, linux/amd64: README.md "Install"; install.sh

This file is the canonical positioning document and the render contract for the
brochure page. The prime-radiant website repo (prime-radiant-inc.github.io)
renders the brochure page from this file plus its stamp at build time; this repo
intentionally ships no docs/index.html (human-authorized deviation, 2026-06-11).
-->

---
<!-- doc-audit:last-reviewed -->
_Last reviewed: 2026-06-11 · commit `82458d4` · verified against code._
