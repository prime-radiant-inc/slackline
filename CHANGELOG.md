# Changelog

All notable changes to this project are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/); versioning follows [Semantic Versioning](https://semver.org/).

## [0.3.0] - 2026-06-01

### Added
- `slackline listen --type <types>` — emit only the named event types (`mention`, `dm`, `thread_reply`, `channel_message`, `reaction`), removing the need to `jq`-filter the stream. Unknown types error; `channel_message` requires `--all-messages`. It's an emit-time filter, not a subscription widener.
- `slackline listen` now emits a `ready` status to stderr once the Socket Mode session is subscribed and events will flow (distinct from `connected`, which is only the websocket open). Wait for `ready` before expecting events.
- `slackline listen --help` now documents the event JSON schema (field names per event type, including `reaction`'s `action` and the `file_share` caveat).

### Changed
- **Breaking:** the two listen reaction events `reaction_added` / `reaction_removed` are unified into a single `reaction` event with an `action` field (`"added"` | `"removed"`).
- **Breaking:** `slackline ask` now exits `5` on timeout (was `1`), so callers can distinguish a timeout from an API/auth/config error without parsing stderr.

## [0.2.3] - 2026-06-01

### Added
- `using-slack` skill (`skills/using-slack/`) documenting the CLI for agents, and `.claude-plugin/marketplace.json` so the plugin is installable via `/plugin marketplace add prime-radiant-inc/slackline`.

### Fixed
- `slackline read --thread` now returns the newest replies instead of the oldest. `conversations.replies` pages oldest-first, and the fetch loop stopped after collecting `--limit` messages — so on any thread with more replies than `--limit`, the newest reply (the true tail) was silently dropped at every limit. This broke thread polling: a watcher comparing the newest `ts` never saw new replies land. Thread reads now page to the end and keep the newest `--limit` messages, matching channel reads.

## [0.2.2] - 2026-05-18

### Changed
- Hoisted repeated string literals to named constants across the codebase. Wire-format strings (event types in `listen`, manifest scopes and bot events in `provision`, error codes in `errs`) now have a single source of truth, and test fixtures are file-local constants. No behavior change — emitted strings are byte-for-byte identical.

## [0.2.1] - 2026-05-18

### Fixed
- `slackline listen` now emits `thread_reply` events by default whenever someone replies in a thread the bot started, with no flag required. Previously, an agent that posted a message and watched for replies would silently miss any reply posted via Slack's "Reply in thread" UI unless `--threads` or `--all-messages` was set. Bot-parent replies are the highest-signal slice of channel traffic and never warranted being gated.

### Changed
- `--threads` flag is now a no-op (kept for backward compatibility). The bot-parent reply case it previously gated is on unconditionally; the broader "any thread the bot has touched" case it documented was never implemented.
- Plugin manifest (`.claude-plugin/plugin.json`) bumped to 0.2.1 to match git tag history.
