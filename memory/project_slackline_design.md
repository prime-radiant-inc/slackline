---
name: Slackline project context
description: Design decisions, research context, and key findings for slackline (Go CLI for AI agent Slack identity)
type: project
---

Slackline is a Go CLI binary that gives AI agents a Slack identity. Linear ticket: REDACTED.

**Why:** Replaces shaharia-lab/slackcli + shell scripts with a single self-contained tool. Adds provisioning automation, real-time Socket Mode listening, dynamic channel resolution.

**How to apply:** When working on slackline, check the design spec at `docs/specs/2026-03-16-slackline-design.md` and implementation plan at `docs/superpowers/plans/2026-03-16-slackline.md`.

## Key decisions
- slack-go/slack (pre-1.0) — pin version, wrap behind SlackAPI interface to insulate from breaking changes
- One config per bot identity — no multi-bot management
- JSONL output for all message/event data, JSON for single responses
- Config at ~/.config/slackline/config.json (0600 perms), provision tokens in separate provision.json
- Socket Mode for real-time events (at-most-once delivery)
- `errs/` package added (not in original spec) for cross-cutting error handling

## API signature notes (slack-go)
- GetConversationReplies may return `([]Message, bool, string, error)` or `([]Message, *Paging, error)` depending on version
- GetConversations may return `([]Channel, string, error)` or `([]Channel, *GetConversationsParameters, error)` depending on version
- GetConversationsParameters.Types is `string` not `[]string` (comma-separated)
- ResponseMetadata (not ResponseMetaData)
