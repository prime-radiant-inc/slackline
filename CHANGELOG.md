# Changelog

All notable changes to this project are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/); versioning follows [Semantic Versioning](https://semver.org/).

## [0.2.1] - 2026-05-18

### Fixed
- `slackline listen` now emits `thread_reply` events by default whenever someone replies in a thread the bot started, with no flag required. Previously, an agent that posted a message and watched for replies would silently miss any reply posted via Slack's "Reply in thread" UI unless `--threads` or `--all-messages` was set. Bot-parent replies are the highest-signal slice of channel traffic and never warranted being gated.

### Changed
- `--threads` flag is now a no-op (kept for backward compatibility). The bot-parent reply case it previously gated is on unconditionally; the broader "any thread the bot has touched" case it documented was never implemented.
- Plugin manifest (`.claude-plugin/plugin.json`) bumped to 0.2.1 to match git tag history.
