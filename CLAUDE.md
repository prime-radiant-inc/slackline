# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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
- **`config/`** — defines `Config` and `ProvisionConfig` structs, `Load`/`Save`, `DefaultPath()`/`DefaultProvisionPath()`. Env vars `SLACKLINE_BOT_TOKEN` and `SLACKLINE_APP_TOKEN` are applied inside `Load()` after reading the file, overriding file values.
- **`errs/`** — `SlackError` type with `Code` field (`Success`/`SlackAPI`/`Auth`/`Config`/`Usage` = 0–4). `WriteError` writes `{"error":"...","detail":"..."}` to stderr. `Execute()` in root maps returned errors to exit codes.
- **`slack/`** — thin wrapper around `slack-go/slack`. `Client` handles `AuthTest`, `PostMessage`, `GetConversationHistory`, etc. `Resolver` provides channel name→ID resolution with in-process caching (no disk cache).
- **`listen/`** — Socket Mode event loop. `Listener` wraps `socketmode.Client`, filters self-events by bot user ID, and emits typed `Event` structs (`mention`, `dm`, `reaction`) as JSONL to stdout. Status messages go to stderr.
- **`provision/`** — admin-only: `GenerateManifest` builds the Slack app manifest JSON, `RotateConfigToken` calls `tooling.tokens.rotate`.

### Config resolution (all runtime commands)

```
--config flag  →  SLACKLINE_CONFIG env  →  ~/.config/slackline/config.json
```

Resolved in `loadConfig()` in `cmd/root.go`. `SLACKLINE_BOT_TOKEN` / `SLACKLINE_APP_TOKEN` override the file values inside `config.Load()`.

### Two config files

- `~/.config/slackline/config.json` — bot identity (tokens, workspace info). Used by all runtime commands.
- `~/.config/slackline/provision.json` — App Configuration Token + refresh token. Used **only** by `slackline create`. Never distribute to non-admin machines.

### Admin vs user flow

`slackline create` (admin): provisions a new Slack app via the manifest API, walks through interactive token collection, writes `config.json`. Uses `provision.json` to rotate the short-lived config token before each run.

`slackline init` (developer/agent): prompts for already-provisioned `xoxb-` and `xapp-` tokens, validates via `auth.test`, writes `config.json`. Does not touch `provision.json`.

### Testing approach

Commands use `cmd.OutOrStdout()` / `cmd.OutOrStderr()` throughout, making output capturable in tests. `cmd/helpers_test.go` has the bulk of command-level tests. `createAppViaManifest` and similar HTTP functions accept an `apiBase` override for httptest-based testing.
