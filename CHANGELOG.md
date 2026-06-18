# Changelog

All notable changes to this project are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/); versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `slackline users [--match <query>]` lists workspace users and resolves a handle, display name, real name, or ID to a `U...` user ID (the path needed to mention someone).

### Changed
- `slackline send` now linkifies `@handle` mentions to real Slack mentions (`<@U...>`) so the person is notified. Handles that don't match a unique user are left as literal text with a `warning:` on stderr; email addresses are never treated as mentions. Pass `--no-link-names` to opt out. (#1)
- `slackline read` now resolves user IDs to handles: the author column renders as `U...|handle`, in-text `<@U...>` mentions are enriched to the labeled `<@U...|handle>` form, and JSON output gains a `user_name` field. Pass `--no-resolve-names` to skip the lookup. (#1)

## [0.4.0] - 2026-06-18

### Added
- `slackline auth whoami` prints the authenticated bot identity.
- `slackline permalink` prints the bare Slack permalink for a message.
- `slackline send`, `ask`, `react`, `channels`, and `download` now accept `--format json` where structured output is useful.

### Changed
- **Breaking:** runtime commands now default to compact, line-oriented text for agent-facing output instead of JSON envelopes. `send` prints `<channel_id> <ts>`, `ask` replies print the same `<ts> <user> <text>` shape as `read`, and `react add/remove` are silent on success.
- **Breaking:** error output is now `error: <code>: <detail>` on stderr. Exit codes remain the machine contract.
- **Breaking:** `channels --json` is replaced by `channels --format json`.
- `slackline read` and `listen` now default to compact text, with JSONL still available through `--format json`.
- `slackline auth status` now validates the Socket Mode app token instead of only reporting that it is configured.

### Fixed
- `slackline listen` now returns a non-zero error with detail when Socket Mode disconnects before readiness.
- `slackline listen --all-messages --type ...` no longer emits Slack `message_replied` parent updates as ordinary `channel_message` events.

## [0.3.4] - 2026-06-09

### Fixed
- `slackline download` now enforces the configured size cap on the actual downloaded byte stream, not just Slack `files.info` metadata.
- Release builds now use Go 1.25.11, clearing reachable Go standard-library vulnerabilities reported against Go 1.25.10.

## [0.3.3] - 2026-06-02

### Fixed
- Installer verification now reports the newly installed binary instead of any older `slackline` already on `PATH`.

## [0.3.2] - 2026-06-02

### Changed
- The installer now verifies the downloaded binary against GitHub's release asset SHA-256 digest.

### Fixed
- Release publication now works for this private repo by avoiding GitHub Artifact Attestations, which require Enterprise Cloud for private repositories.

## [0.3.1] - 2026-06-02

### Added
- Release builds now generate GitHub artifact attestations, and the installer verifies the downloaded binary's attestation before installing it.

### Changed
- The installer now requires the GitHub CLI (`gh`) so release attestations can be verified.
- The Go toolchain baseline is now `1.25.10`.

### Fixed
- Runtime and provisioning config rewrites now repair existing permissive file modes back to `0600`.
- Interactive token prompts now require a terminal and read secrets without echoing pasted tokens.
- `slackline download --out PATH` now uses random same-directory temporary files and no-replace finalization when `--force=false`.
- Socket Mode self-event filtering now handles Slack events that identify the bot by `bot_id`.

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
