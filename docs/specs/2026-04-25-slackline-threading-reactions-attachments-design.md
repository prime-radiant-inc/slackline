# Slackline: threading, reactions, attachments, scriptable provisioning

**Status:** approved (design phase)
**Date:** 2026-04-25
**Author:** Jesse + Bot

## Summary

Slackline today supports send/read/listen for text-only messages, with mentions and DMs streamed via Socket Mode. This spec adds:

1. **Threading visibility in the listener** — opt-in flags for thread-reply and full-channel streaming.
2. **Reactions** — symmetric send (`react add` / `react remove`) and receive (`reaction_added` / `reaction_removed`).
3. **Attachments** — `--attach` repeatable flag on `send` for upload, dedicated `download` command for fetch, file metadata in receive-side events.
4. **Scriptable bot provisioning** — replace the monolithic interactive `slackline create` with discrete primitives (`slackline provision bootstrap`, `slackline provision <name>`, existing `slackline init`) so an agent with browser tools can deploy bots end-to-end without per-bot human interaction.

There is exactly one semantic break (the `reaction` event renames to `reaction_added`) and one interface break (`slackline create` is removed in favor of `provision`). Both are documented in the migration section.

## Motivation

The user is deploying ~12 bots in containers from this codebase. The first bot (`slackline-claude`) was provisioned today by an agent driving Chrome via use_browser; the existing CLI made the experience clunky because `slackline create` interleaves API calls and stdin prompts with human-only browser actions in a single non-resumable flow. Splitting into primitives makes the same flow scriptable.

Beyond provisioning, the messaging surface is too narrow for the bots' intended use. A conversational bot that can't see thread replies it didn't get @-mentioned in, can't react with an emoji, and can't send or receive files isn't usable as a real Slack participant.

## Non-goals

- Resumable downloads.
- Parallel multi-file download in one CLI invocation (call `slackline download` per file).
- Public-link generation, file search, file listing.
- Slack workflow steps, slash commands, interactivity, modals.
- Backwards compatibility for the removed `slackline create` command beyond a discoverable migration error.
- A retry / circuit-breaker layer for transient Slack API failures (caller's job).

## Architecture

No package restructuring. Touched files by package:

| Package | Files |
|---|---|
| `cmd/` | NEW: `provision.go`, `react.go`, `download.go`. MODIFIED: `send.go` (--attach), `listen.go` (mode flags), `read.go` (files in JSONL), `helpers_test.go`. REMOVED: `create.go`. |
| `listen/` | `events.go` Event struct grows `Files`, `ParentUserID`. `listener.go` handler grows: `message.channels`/`message.groups` subscriptions, `reaction_removed`, file-share synthesis, channel-message filtering modes. |
| `slack/` | `api.go` SlackAPI interface gains reaction, upload, file-info, file-download methods. NEW: `files.go` with multi-file-upload wrapper around `files.getUploadURLExternal` + `files.completeUploadExternal`. |
| `provision/` | `manifest.go` adds scopes (`reactions:write`, `files:read`, `files:write`) and bot_events (`reaction_removed`, `message.channels`, `message.groups`). Parameterized for `--description` / `--always-online`. `tokens.go` unchanged. |
| `config/` | unchanged. |
| `errs/` | unchanged. |
| repo root | NEW: `.claude-plugin/plugin.json`, `skills/slackline-provision-bot/SKILL.md`, `skills/slackline-provision-bot/copy-buttons.md`. MODIFIED: `CLAUDE.md`, `README.md`. |

The `slack.SlackAPI` interface is the testing seam: every new API method goes through it; every command test stubs against `cmd.fakeAPI`.

Considered and rejected: typed event sum types (Go interfaces / per-type structs) instead of growing the flat `Event` struct. Rejected because Go's lack of true sum types makes the pattern verbose, and the JSONL contract favors a single flat schema with `type` discrimination.

## Component: `slackline listen` changes

### Flags

```
slackline listen [--threads] [--all-messages] [--include-bot-self]
```

- **default**: `mention`, `dm`, `reaction_added`, `reaction_removed` events. Backwards-compatible with existing JSONL consumers (only `reaction_removed` is new and ignorable).
- **`--threads`**: also emit `thread_reply` events for messages in threads where the bot has participated. Stateless: emit only when the message has a `thread_ts` AND `parent_user_id == bot_user_id`.
- **`--all-messages`**: firehose. Emit every `message.channels`/`message.groups` event as `channel_message`. Implies `--threads`.
- **`--include-bot-self`**: turns off the default self-filter that drops events authored by `bot_user_id`. Applies to mentions, DMs, channel messages, reactions.

### Event schemas

```jsonc
{"type":"reaction_added",  "channel":"C…", "user":"U…", "emoji":"thumbsup", "item_ts":"…", "thread_ts":"…?"}
{"type":"reaction_removed","channel":"C…", "user":"U…", "emoji":"thumbsup", "item_ts":"…", "thread_ts":"…?"}
{"type":"thread_reply",    "channel":"C…", "user":"U…", "text":"…", "ts":"…", "thread_ts":"…", "parent_user_id":"U…", "files":[…?]}
{"type":"channel_message", "channel":"C…", "user":"U…", "text":"…", "ts":"…", "thread_ts":"…?", "files":[…?]}
{"type":"dm",              "channel":"D…", "user":"U…", "text":"…", "ts":"…", "thread_ts":"…?", "files":[…?]}
{"type":"mention",         "channel":"…",  "user":"U…", "text":"…", "ts":"…", "thread_ts":"…?", "files":[…?]}
```

`files` element schema:

```jsonc
{"id":"F123","name":"report.pdf","mimetype":"application/pdf","size":12345,"title":"Q4 Report"}
```

URLs and download tokens are deliberately omitted from receive-side events. Caller invokes `slackline download --file F123 --out PATH` to fetch bytes. Keeps tokens off stdout and gives slackline a single chokepoint for download policy.

### Files-without-text

Slack already presents a file share with no message body as a regular message event with `subtype: "file_share"`, empty `text`, and a populated `files` array — there is no separate `file_shared`-only event we need to subscribe to. The listener emits the appropriate type (`dm` / `mention` / `thread_reply` / `channel_message`) with `text:""` and the `files` array. Caller logic doesn't branch on "is this a file-only share."

### Renames (BREAKING)

`reaction` → `reaction_added`. The old name is ambiguous now that `reaction_removed` exists.

### Self-filter

Drops events whose `user == bot_user_id`. Applies uniformly to all message and reaction events. `--include-bot-self` defeats this.

## Component: `slackline send` with attachments

### CLI

```
slackline send --channel CHANNEL [--message TEXT | stdin]
               [--thread TS]
               [--attach PATH] ...     # repeatable
```

### Behavior

- No `--attach`: text-only path, unchanged. Calls `chat.postMessage`.
- One or more `--attach`: switches to `files.completeUploadExternal` batch path. All files uploaded then shared as **one message** with `initial_comment = TEXT`.
- `--message` becomes optional when at least one `--attach` is present.
- `--thread` works in both paths; the upload's `thread_ts` parameter takes the same value.

### Validation (before any API call)

1. Each `--attach` resolves to a regular, readable file.
2. Combined size ≤ `SLACKLINE_MAX_UPLOAD_BYTES` (default 100 MB).
3. No glob expansion in slackline; the shell does that.

### Output

```jsonc
{
  "ok": true,
  "channel": "C…",
  "ts": "1777153650.707559",
  "thread_ts": "1777153600.000000",   // omitted if not a thread reply
  "files": [                            // omitted if no attachments
    {"id":"F123","name":"report.pdf","permalink":"https://..."}
  ]
}
```

`permalink` is included on send-side responses so the caller can reference the file in a follow-up message.

### Error handling

- File-not-found / unreadable → `errs.Usage` (exit code 4).
- Upload failure → `errs.SlackAPI` (exit code 1).
- Partial-success on multi-file upload → fail the whole send. Detail names which files succeeded so the caller can debug. Considered alternative: per-file status + overall ok=true. Rejected — too easy for callers to miss attachment failure.

### slack-go specifics

`UploadFileV2` ships single files as their own messages. For batch (N files in one message), slack-go does not currently expose a one-call helper. We implement `slack/files.go::CompleteUploadExternal` over the raw HTTP endpoints `files.getUploadURLExternal` (per file) and `files.completeUploadExternal` (with `files: [{id, title}, …]`, single `channel_id`, `initial_comment`, `thread_ts`).

## Component: `slackline react`

### CLI

```
slackline react add    --channel CHANNEL --ts MESSAGE_TS --emoji NAME
slackline react remove --channel CHANNEL --ts MESSAGE_TS --emoji NAME
```

`--emoji` is the emoji name without colons (`thumbsup`, not `:thumbsup:`). The CLI strips colons defensively.

Subcommand pattern (`react add`, `react remove`) chosen over flag (`react --remove`) because it mirrors `slack-go`'s `AddReaction`/`RemoveReaction`, reads naturally in scripts, and leaves room for `react list` later.

### Output

```jsonc
{"ok": true, "channel": "C…", "ts": "1777…", "emoji": "thumbsup", "action": "added"}
{"ok": true, "channel": "C…", "ts": "1777…", "emoji": "thumbsup", "action": "removed"}
```

### Error handling

- `already_reacted` (add when present) → idempotent success: `{"ok": true, "no_op": true, …}`.
- `no_reaction` (remove when absent) → same idempotent success.
- `message_not_found` / `channel_not_found` / `not_authed` → standard `errs.SlackAPI` / `errs.Auth`.

The idempotent treatment is intentional and a deviation from slack-go's surface. Considered alternative: surface errors verbatim. Rejected — agentic callers retrying after partial failure shouldn't have to special-case "I already did this."

## Component: `slackline download`

### CLI

```
slackline download --file FILE_ID --out PATH
```

`--file` and `--out` both required. `--out -` writes raw bytes to stdout. No `--channel` argument — file IDs are workspace-global.

### Behavior

1. Call `files.info` for metadata. Fail with `errs.SlackAPI` on `file_not_found` / `not_in_channel` / `access_denied`.
2. Check `size` against `SLACKLINE_MAX_DOWNLOAD_BYTES` (default 100 MB) before fetching content. Exceeds → `errs.Usage` with size in detail.
3. GET `url_private` with `Authorization: Bearer <bot_token>`.
4. Stream to `--out`:
   - `--out PATH`: create with mode `0o600`, write atomically (`<path>.tmp` then rename).
   - `--out -`: stream raw to stdout.
5. On `--out PATH` success, print summary JSON on **stderr** (keeps stdout binary-clean for `--out -`):

```jsonc
{"ok":true,"file":"F123","name":"report.pdf","mimetype":"application/pdf","size":12345,"path":"./report.pdf"}
```

### Error handling

- Existing file at `--out PATH`: refuse (`errs.Usage`). `--force` to overwrite. Considered alternative: silent overwrite. Rejected — agents in loops shouldn't accidentally clobber.
- Out-path's parent dir missing: refuse (`errs.Usage`). Caller creates dirs.
- Network failure mid-stream: `<path>.tmp` is removed; no `<path>` produced. `errs.SlackAPI`.

## Component: `slackline provision`

Replaces the removed `slackline create`. No browser, no waiting, no stdin prompts (in the per-bot subcommand).

### CLI

```
slackline provision NAME [--description TEXT] [--always-online]
slackline provision bootstrap          # one-time per machine
```

`provision NAME` behavior:

1. Load `provision.json`. If missing → fail with a clear message pointing at `slackline provision bootstrap`.
2. Rotate config token via `tooling.tokens.rotate`. Persist new tokens to `provision.json`.
3. Call `apps.manifest.create` with the manifest from `GenerateManifest(NAME, description, alwaysOnline)`.
4. Emit single-line JSON to stdout:

```jsonc
{
  "ok": true,
  "app_id": "A0AV818HENA",
  "team_id": "T0B08TKUM89",
  "team_domain": "tooleraworkspace",
  "install_url": "https://api.slack.com/apps/A0AV818HENA/install-on-team",
  "oauth_authorize_url": "https://slack.com/oauth/v2/authorize?client_id=…&team=…&scope=…",
  "oauth_page_url": "https://api.slack.com/apps/A0AV818HENA/oauth",
  "general_page_url": "https://api.slack.com/apps/A0AV818HENA/general"
}
```

5. No progress messages on stderr in success path. Errors use the standard `{"error":"…","detail":"…"}` envelope.

`provision bootstrap` behavior:

- Reads `SLACKLINE_CONFIG_TOKEN` and `SLACKLINE_REFRESH_TOKEN` if **both** are set; if exactly one is set, fails with `errs.Usage` (matching `slackline init`'s pairing semantics). Otherwise prompts via stdin (matching today's `create --init` UX).
- Writes `provision.json` with `0o600` permissions.
- Single-purpose. Distinct from `provision NAME` so scripts don't accidentally trigger interactive prompts.

### `slackline create` removal

`cmd/create.go` deleted. Invoking `slackline create` (any args) returns:

```jsonc
{"error":"removed","detail":"slackline create has been split into 'slackline provision bootstrap' and 'slackline provision <name>'. See `slackline provision --help`."}
```

with exit code 4. Hard break, not silent removal, so existing scripts get a discoverable migration message.

### `slackline init` unchanged

Already supports non-interactive env-var mode (`SLACKLINE_BOT_TOKEN`, `SLACKLINE_APP_TOKEN`, `SLACKLINE_WORKSPACE_URL`). No changes here.

### End-to-end agentic recipe (becomes SKILL.md content)

```
1. slackline provision bootstrap         # one-time per machine
2. for each new bot:
   a. slackline provision NAME           → captures app_id + URLs
   b. agent drives browser:
      - install via oauth_authorize_url, click Allow
      - copy xoxb- from oauth_page_url
      - generate + copy xapp- (connections:write) from general_page_url
   c. SLACKLINE_BOT_TOKEN=… SLACKLINE_APP_TOKEN=… slackline init
```

## Component: `slack.SlackAPI` interface

```go
type SlackAPI interface {
    // existing
    AuthTest() (*goslack.AuthTestResponse, error)
    PostMessage(channelID string, options ...goslack.MsgOption) (string, string, error)
    GetConversationHistory(*goslack.GetConversationHistoryParameters) (*goslack.GetConversationHistoryResponse, error)
    GetConversationReplies(*goslack.GetConversationRepliesParameters) ([]goslack.Message, bool, string, error)
    GetConversations(*goslack.GetConversationsParameters) ([]goslack.Channel, string, error)

    // reactions
    AddReaction(name string, item goslack.ItemRef) error
    RemoveReaction(name string, item goslack.ItemRef) error

    // attachments
    UploadFileV2(params goslack.UploadFileV2Parameters) (*goslack.FileSummary, error)
    GetFileInfo(fileID string, count, page int) (*goslack.File, []goslack.Comment, *goslack.Paging, error)
    GetFile(downloadURL string, writer io.Writer) error
    CompleteUploadExternal(channelID, threadTS, initialComment string, files []FileUpload) ([]goslack.FileSummary, error)
}
```

`CompleteUploadExternal` is the multi-file batch wrapper implemented in `slack/files.go` over raw HTTP. `FileUpload` is a slackline-defined struct: `{Path, Title}`.

`cmd.fakeSlackAPI` (defined in `cmd/helpers_test.go`) grows methods for each new entry with simple call-recording behavior.

## Manifest changes (consolidated)

```go
// scopes added (existing kept):
"reactions:write",
"files:read",
"files:write",

// bot_events added (existing kept):
"reaction_removed",
"message.channels",
"message.groups",
```

`message.channels`/`message.groups` are needed for `--threads` and `--all-messages` flag modes. Slack pushes them only if the manifest opts in; the listener filters per flag at runtime. The default mode listener silently drops them. File shares without text body arrive via `message.channels`/`message.groups`/`message.im` already (as `subtype: "file_share"`) — no separate `file_shared` subscription needed.

`provision.GenerateManifest` becomes parameterized: `GenerateManifest(name, description string, alwaysOnline bool) *Manifest`. Existing call sites pass the existing defaults.

**Existing-bot impact:** `slackline-claude` (provisioned 2026-04-25) has the old manifest. We don't push retroactively. Each existing bot needs one manual reinstall via its OAuth URL after this ships. Documented in the new SKILL.md.

## Skills + docs

### To create

- `.claude-plugin/plugin.json` — slackline becomes a publishable Claude Code plugin from this repo.
- `skills/slackline-provision-bot/SKILL.md` — agentic provision recipe. Worked example: `provision <name>` → JSON output → browser-driving steps with the exact CSS selectors discovered during the `slackline-claude` provisioning today (`button[aria-label="Copy access token"]`, `button[aria-label="Copy refresh token"]`, `button[data-qa="oauth_submit_button"]`, `button.p-app_level_tokens_info__input_button`, etc.) → `init` with env vars. Calls out the "real CDP click vs. JS click" gotcha (slack's React handlers don't fire on JS-driven `.click()` on the copy buttons).
- `skills/slackline-provision-bot/copy-buttons.md` — progressive-disclosure file with the full selector reference for the OAuth, general, and config-token pages, since these may drift as Slack updates their admin UI. One file to update when selectors break.

### To update

- `~/prime-radiant/cc-plugin-primeradiant-ops/.../skills/slack-messaging/SKILL.md` — covers `send`, `read`, `listen`, `react`, `download`, `--attach`, new event shapes. Migration note for `reaction` → `reaction_added` event rename.
- `superpowers-marketplace/superpowers-lab` slack-messaging skill — locate source repo (cache is at `~/.claude/plugins/cache/superpowers-marketplace/superpowers-lab/0.3.0/skills/slack-messaging/SKILL.md`), apply same updates OR mark fully deprecated with explicit pointer to the primeradiant version.
- `slackline/CLAUDE.md` — refresh "Architecture" section for new packages, command list, event types.
- `slackline/README.md` — same.

The implementation plan resolves "where is the source for the lab skill" before editing — caches are just snapshots.

## Testing strategy

`cmd/helpers_test.go`, `listen/listener_test.go`, `listen/events_test.go`, `provision/manifest_test.go` remain the seam.

New test groups:

1. **Reactions** (`cmd/react_test.go`): success / `already_reacted`-as-success / `no_reaction`-as-success / channel_not_found / auth-failure.
2. **Send with attachments** (`cmd/send_test.go` extended): single file, multiple files, file + text, file in thread, size cap, missing file, unreadable file. Fake `SlackAPI` records upload params.
3. **Download** (`cmd/download_test.go`): `--out PATH` writes correct bytes via httptest, `--out -` writes to stdout, `--force` semantics, size cap, missing parent dir.
4. **Listener** (`listen/listener_test.go` extended): `reaction_removed` emission, `--threads` mode emits replies only when bot is parent, `--all-messages` emits every message, `--include-bot-self` defeats self-filter, file-bearing message emits files array, `file_shared` synthesizes a message event.
5. **Provision** (`cmd/provision_test.go`): bootstrap reads env, bootstrap reads stdin, provision rotates and creates app and emits expected JSON, removed-create returns the migration error. httptest server stands in for `slack.com/api/`.
6. **Manifest regression** (`provision/manifest_test.go` extended): golden-file assertion that `GenerateManifest` produces exactly the documented scopes and bot_events; accidental scope drift fails CI.

No real-network tests. The `apiBase` override pattern used today (e.g., `createAppViaManifest`) extends consistently to all new subcommands.

## Backwards compatibility / migration

| Surface | Change | Compat impact |
|---|---|---|
| `slackline create` | Removed | **Hard break.** Returns the migration error. |
| `slackline create --init` | Removed | Same. |
| `reaction` event in JSONL | Renamed to `reaction_added` | **Soft break.** Old consumers parsing `type=="reaction"` see no events; one-line update needed. |
| New events (`reaction_removed`, `thread_reply`, `channel_message`) | Added | No impact unless consumer is strict-mode parsing unknown types. |
| Files array on `dm`/`mention` | Added | No impact (additive field). |
| `slackline send --attach` | Added | No impact (additive flag). |
| `slackline react/download/provision` | Added | No impact (new commands). |
| Manifest scope additions | Existing bots need one-time reinstall via OAuth URL | Documented in SKILL.md and CLAUDE.md. |

Two breaks total: the `reaction` event rename and the `create` removal. Both worth it for the cleaner end state.

## Implementation ordering

1. Manifest expansion (`provision/manifest.go` + tests). No behavior change yet.
2. `SlackAPI` interface additions + `cmd.fakeAPI` updates. Unblocks all command work.
3. `slackline provision` + bootstrap + remove `create`. Migration error lands early so any scripts break early.
4. `slackline react add/remove` + `reaction_removed` listener. Small, self-contained; exercises new SlackAPI surface.
5. `slackline download`. Depends on `GetFileInfo`/`GetFile`.
6. `slackline send --attach`. Depends on `UploadFileV2` + `CompleteUploadExternal` wrapper.
7. Listener `--threads` / `--all-messages` / `--include-bot-self` flags + new `thread_reply` / `channel_message` events + files array attached to all message events. Touches the listener last so it can incorporate everything (file metadata schema is shared with download). The `reaction` → `reaction_added` rename happens earlier in step 4 alongside `reaction_removed`.
8. Plugin manifest + new SKILL.md + update of all slackline-related skills + CLAUDE.md / README. Last, because docs reference final command shapes.

Each step independently passes tests. PR boundaries align with these steps.
