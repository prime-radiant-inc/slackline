# slackline

> A single-binary Go CLI that gives AI agents a Slack identity to send messages, read channels, and stream real-time events, with admin tooling to provision new bots.

**Family:** agent-libs · **Type:** tool · **Lifecycle:** production · **Owner:** obra

## What it does
slackline is a single Go binary with one config file that gives an AI agent a Slack bot identity. It can send messages, read channels, and stream real-time events over Slack's Socket Mode. It also includes admin tooling (`slackline provision`) to create and bootstrap new Slack bots non-interactively.

## How it fits
- Depends on: — (no internal prime-radiant-inc deps in go.mod)
- Used by: —
- External: Slack API (bot token `xoxb-`, app token `xapp-`, Socket Mode); GitHub releases for install/digest verification

## Runtime & data
- Runs: CLI (`slackline`), installed to `~/.local/bin`
- Data in: Slack tokens (env or `~/.config/slackline/config.json`), Slack channel events
- Data out: Slack messages, real-time event stream

<!-- Maintained by the maintaining-project-map skill. Do not hand-edit; regenerated. -->
