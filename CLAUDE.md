# Repository guide for AI coding agents

This file guides AI coding agents (such as Claude Code and Codex) working with code in this repository. It is also exposed as `AGENTS.md` through a symlink, so both filenames resolve to the same guidance.

## Commands

```bash
make build                  # build ./slackline binary
make test                   # go test ./... -v
make vet                    # go vet ./...
make clean                  # remove built binary
make release VERSION=1.2.3  # tag + push (requires clean working tree)

go test ./cmd/... -run TestFunctionName -v   # run a single test
go build -ldflags "-X main.version=dev" -o slackline .  # build with version
```

**Linter:** `golangci-lint run ./...` — uses `gofumpt` (stricter than `gofmt`) and `gci` for import ordering. Run before committing; the pre-commit hook also runs it.

**Hooks (lefthook):** pre-commit runs `golangci-lint` + `go vet`; pre-push runs `go test`. Do not skip hooks.

## Architecture

### Package roles

- **`main.go`** — entry point only. Declares `var version = "dev"`, calls `cmd.SetVersion(version)`, then `cmd.Execute()`. Version is injected at release via `-ldflags "-X main.version=..."`.
- **`cmd/`** — one file per Cobra subcommand. `root.go` wires the `--config` persistent flag and `loadConfig()` helper used by all runtime commands. Shared helpers (`isAuthError`, `writeMessage`) live in `ask.go`.
  - `cmd/provision.go` — `provision NAME` (per-bot, no interaction, machine-readable JSON output) and `provision bootstrap` (one-time per machine; env-var-or-stdin to seed `provision.json`). Calls `tooling.tokens.rotate` and `apps.manifest.create`.
  - `cmd/react.go` — `react add` / `react remove` subcommands. Idempotent: `already_reacted` and `no_reaction` Slack errors are silently treated as success (`no_op: true` in output).
  - `cmd/download.go` — `download --file ID --out PATH|'-'`. Fetches file metadata via `files.info` then downloads via authenticated `url_private` GET. Atomic write via `.tmp` + rename. `--out -` streams to stdout.
  - `cmd/create.go` — migration stub. Returns `errs.Usage` error with a pointer to `slackline provision`. The command is still registered (not hidden) so users get a helpful error instead of "unknown command".
  - `cmd/users.go` — `users [--match <query>]` lists/searches workspace users (`{id, handle, display_name, real_name}`, text + json), mirroring `channels.go`. `filterUsers` drops deactivated users and substring-matches ID/handle/display/real name. This is the agent's name→ID path for mentions.
  - `cmd/send.go` — `linkifyMessage` rewrites `@handle` to `<@ID>` (via `slack.UserDirectory`) before posting so mentions notify; default on, `--no-link-names` opts out. Unresolved tokens stay literal with a stderr `warning:`; an outright resolution failure (e.g. missing `users:read`) is a hard error, not a silent degrade. Resolution only contacts Slack when the body contains an `@token`.
  - `cmd/read.go` — when name resolution is on (default; `--no-resolve-names` opts out), `resolveNames` enriches in-text `<@ID>` to `<@ID|handle>`, sets `messageOutput.UserName`, and renders the author as `ID|handle` in text. Costs one `users.list` per read; a lookup failure warns and falls back to raw IDs.
- **`config/`** — defines `Config` and `ProvisionConfig` structs, `Load`/`Save`, `DefaultPath()`/`DefaultProvisionPath()`. Env vars `SLACKLINE_BOT_TOKEN` and `SLACKLINE_APP_TOKEN` are applied inside `Load()` after reading the file, overriding file values.
- **`errs/`** — `SlackError` type with `Code` field (`Success`/`SlackAPI`/`Auth`/`Config`/`Usage`/`Timeout` = 0–5). `WriteError` writes `{"error":"...","detail":"..."}` to stderr. `Execute()` in root maps returned errors to exit codes; cobra's own usage failures (unknown/missing flags, wrong argument counts, unknown commands) are mapped to `Usage` (4) via `configureUsageErrors`/`isCobraUsageError` in `root.go`.
- **`slack/`** — thin wrapper around `slack-go/slack`. `Client` handles `AuthTest`, `PostMessage`, `GetConversationHistory`, etc. `Resolver` provides channel name→ID resolution with in-process caching (no disk cache). `slack/users.go` adds `UserDirectory`: one cached `users.list` fetch backing forward resolution (`ResolveHandle`: `@handle`→ID, handle-first then unique display/real name), reverse lookup (`Name`: ID→handle), `LinkifyMentions` (`@handle`→`<@ID>`, regex-anchored to skip emails, trailing-dot aware), and `EnrichMentions` (`<@ID>`→`<@ID|handle>`). `slack/files.go` adds `UploadFiles` for multi-file batched upload via raw HTTP to `files.getUploadURLExternal` + `files.completeUploadExternal` (the `UploadFileV2` method does not exist in v0.19.0).
- **`listen/`** — Socket Mode event loop. `Listener` wraps `socketmode.Client`, filters self-events by bot user ID, and emits compact text event lines to stdout by default (`--format json` preserves JSONL output). Status messages go to stderr. Event types: `mention`, `dm`, `reaction` (with an `action` field, `added`/`removed`), `thread_reply` (emitted by default for replies in threads the bot started; `--all-messages` widens to all thread replies), `channel_message` (emitted only with `--all-messages`). `--threads` is accepted for backward compatibility but is a no-op since v0.2.1 — bot-parent thread replies are always emitted.
- **`provision/`** — admin-only: `GenerateManifest` builds the Slack app manifest JSON, `RotateConfigToken` calls `tooling.tokens.rotate`.

### Config resolution (all runtime commands)

```
--config flag  →  SLACKLINE_CONFIG env  →  ~/.config/slackline/config.json
```

Resolved in `loadConfig()` in `cmd/root.go`. `SLACKLINE_BOT_TOKEN` / `SLACKLINE_APP_TOKEN` override the file values inside `config.Load()`.

### Two config files

- `~/.config/slackline/config.json` — bot identity (tokens, workspace info). Used by all runtime commands.
- `~/.config/slackline/provision.json` — App Configuration Token + refresh token. Used **only** by `slackline provision`. Never distribute to non-admin machines.

### Admin vs user flow

`slackline provision bootstrap` (one-time per machine): seeds `provision.json` from `SLACKLINE_CONFIG_TOKEN` / `SLACKLINE_REFRESH_TOKEN` env vars, or interactively via stdin. Does not contact Slack.

`slackline provision NAME` (per-bot, admin): rotates the config token, calls `apps.manifest.create`, and writes machine-readable JSON to stdout with `app_id`, `install_url`, `oauth_authorize_url`, `oauth_page_url`, `general_page_url`. No interactive prompts. The agentic recipe for driving the browser install + token-collection steps lives in the `using-slack` skill's `provisioning.md` (`.claude-plugin/` + `skills/using-slack/provisioning.md`).

`slackline init` (developer/agent): prompts for already-provisioned `xoxb-` and `xapp-` tokens (or reads them from `SLACKLINE_BOT_TOKEN` / `SLACKLINE_APP_TOKEN`), validates via `auth.test`, writes `config.json`. Does not touch `provision.json`.

### Testing approach

Commands use `cmd.OutOrStdout()` / `cmd.OutOrStderr()` throughout, making output capturable in tests. `cmd/helpers_test.go` has the bulk of command-level tests. `postProvisionManifestCreate` and similar HTTP functions accept an `apiBase` override for httptest-based testing.

`cmd.fakeSlackAPI` (defined in test helpers) now stubs reactions (`AddReaction`, `RemoveReaction`), file ops (`GetFileInfo`, `GetFile`), `UploadFiles` batched upload, and `GetUsers` (via `users`/`usersErr` fields) in addition to the original message/channel methods. Shared user fixtures (`cmdMakeUser`, `drewUser`) live in `cmd/helpers_test.go`.

`provision/manifest_test.go` has a golden-file regression test (`TestGenerateManifest_Golden`) that pins the exact scope set (`reactions:write`, `files:read`, `files:write`, plus the original set) and event set (`reaction_removed`, `message.channels`, `message.groups`, plus the original set).

### slack-go API quirks

The `slack-go/slack` library is pre-1.0 and has some non-obvious signatures:

- `GetConversationReplies` returns `([]Message, bool, string, error)` — the bool is `hasMore`, the string is a cursor.
- `GetConversations` returns `([]Channel, string, error)` — the string is the next cursor.
- `GetConversationsParameters.Types` is a `[]string` (the library comma-joins it internally); `slack/resolve.go` passes `[]string{"public_channel", "private_channel"}`.
- Pagination metadata field is `ResponseMetadata` (not `ResponseMetaData`).
- `UploadFileV2` does not exist in v0.19.0. Multi-file batched shares require the raw `files.getUploadURLExternal` + `files.completeUploadExternal` endpoints, implemented in `slack/files.go::UploadFiles`.
- `MessageEvent.Files` and `MessageEvent.ParentUserId` actually live on `ev.Message` (`*goslack.Msg`), not on the `MessageEvent` itself. Access via `ev.Message.Files` / `ev.Message.ParentUserId` after nil-checking `ev.Message`.
- `AppMentionEvent` has no `Files` field in v0.19.0. Mention events cannot carry attached files via this library; file-upload events arrive as `MessageEvent` with `SubType == "file_share"` instead.
- `GetUsers(options ...GetUsersOption)` returns `([]User, error)` and paginates `users.list` internally. The user's display name is `User.Profile.DisplayName`; the unique workspace handle is `User.Name`.
- `MsgOptionLinkNames` (the `link_names` param) is documented in slack-go as "finds and links user **groups**. Does not support linking individual users" — so it does **not** reliably linkify `@handle` user mentions. slackline pre-resolves `@handle`→`<@ID>` in `slack.UserDirectory.LinkifyMentions` instead, which is also Slack's recommended mention syntax.

---
<!-- doc-audit:last-reviewed -->
_Last reviewed: 2026-06-11 · commit `e4f4b21` · verified against code (1 claim deferred to review)._
