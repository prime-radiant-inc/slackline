# Help & Usability Improvements Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the `unknown_error` bug for missing required flags, reorder exit codes to Unix convention, enrich all command help with output format and exit code docs, and add non-interactive `init` via env vars for agent use.

**Architecture:** Exit code constants in `errs/errors.go` are reordered (Usage=2, Auth=4). `Execute()` in `cmd/root.go` is refactored to extract a testable `execute(stderr io.Writer) int` inner function that gains a missing-flag detection branch. Non-interactive `init` reads env vars via an extracted `readEnvInputs()` helper before touching stdin. All command `Long` descriptions and `Example` fields are updated in-place.

**Spec:** `docs/superpowers/specs/2026-03-17-help-improvements-design.md`

**Tech Stack:** Go 1.21+, cobra, standard library

---

## Chunk 1: Exit Codes + Execute Bug Fix

### Task 1: Reorder exit code constants

**Files:**
- Modify: `errs/errors_test.go:33-50`
- Modify: `errs/errors.go:9-15`

- [ ] **Step 1: Update TestExitCodes to assert new values**

In `errs/errors_test.go`, update the table so `Usage=2` and `Auth=4`:

```go
func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"Success", Success, 0},
		{"SlackAPI", SlackAPI, 1},
		{"Usage", Usage, 2},
		{"Config", Config, 3},
		{"Auth", Auth, 4},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./errs/ -run TestExitCodes -v
```

Expected: FAIL — `Auth=2 want 4`, `Usage=4 want 2`

- [ ] **Step 3: Update constants in errs/errors.go**

```go
const (
	Success  = 0
	SlackAPI = 1
	Usage    = 2
	Config   = 3
	Auth     = 4
)
```

- [ ] **Step 3.5: Verify no hard-coded exit code integers in tests**

```
grep -rn "== 2\b\|== 4\b" --include="*_test.go" .
```

Expected: any matches should be for non-exit-code comparisons. The `errs/errors_test.go` `TestExitCodes` function is the only test that asserts exit code integers, and it uses named constants for the `got` side — the `want` integers are what we just updated. Confirm no other test file hard-codes `2` or `4` as an expected exit code.

- [ ] **Step 4: Run full test suite**

```
go test ./...
```

Expected: PASS — all callers use named constants (`errs.Auth`, `errs.Usage`) so no other code changes are needed. The integer values changed but the names did not.

- [ ] **Step 5: Commit**

```bash
git add errs/errors.go errs/errors_test.go
git commit -m "fix: reorder exit codes to Unix convention (Usage=2, Auth=4)"
```

---

### Task 2: Make Execute() testable and fix missing-flag error

**Files:**
- Modify: `cmd/root.go`
- Create: `cmd/root_test.go`

**Background:** `Execute()` currently writes errors to `os.Stderr` directly, making it untestable. Refactor it to extract `execute(stderr io.Writer) int` which the public `Execute()` calls with `os.Stderr`. Tests call `execute(&buf)` directly. cobra's missing-required-flag error is a plain `error` (not `*errs.SlackError`) that currently falls through to the `unknown_error` catch-all — we detect it with `strings.Contains` and return `usage_error` with exit code 2.

**Note on shared state:** `rootCmd` is a package-level variable shared across all tests in the `cmd` package. The `t.Cleanup` below resets `SetArgs` after each test. cobra re-parses flags on each `Execute()` call, so flag values do not persist between test runs. If you observe unexpected test interference, add `rootCmd.ResetCommands()` or restructure into a subtest.

- [ ] **Step 1: Write the failing test**

Create `cmd/root_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
)

func TestExecute_MissingRequiredFlag(t *testing.T) {
	// "ask" requires --channel; invoking without it should produce usage_error.
	rootCmd.SetArgs([]string{"ask"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	var buf bytes.Buffer
	code := execute(&buf)

	if code != errs.Usage {
		t.Errorf("exit code = %d, want %d (errs.Usage)", code, errs.Usage)
	}

	line := bytes.TrimRight(buf.Bytes(), "\n")
	var got map[string]string
	if err := json.Unmarshal(line, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nbody: %s", err, buf.String())
	}
	if got["error"] != "usage_error" {
		t.Errorf("error = %q, want %q", got["error"], "usage_error")
	}
	if got["detail"] == "" {
		t.Error("detail should not be empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./cmd/ -run TestExecute_MissingRequiredFlag -v
```

Expected: FAIL — compile error (`execute` function not yet defined)

- [ ] **Step 3: Refactor Execute() and add execute() in root.go**

In `cmd/root.go`, add `"io"` and `"strings"` to imports, then split `Execute` into a public wrapper and a testable inner function:

```go
// Execute runs the root command and returns an exit code.
func Execute() int {
	return execute(os.Stderr)
}

func execute(stderr io.Writer) int {
	if err := rootCmd.Execute(); err != nil {
		var se *errs.SlackError
		if errors.As(err, &se) {
			errs.WriteError(stderr, se.Err, se.Detail)
			return se.ExitCode()
		}
		if strings.Contains(err.Error(), "required flag(s)") {
			errs.WriteError(stderr, "usage_error", err.Error()+" — run 'slackline --help' for usage")
			return errs.Usage
		}
		errs.WriteError(stderr, "unknown_error", err.Error())
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./cmd/ -run TestExecute_MissingRequiredFlag -v
```

Expected: PASS

- [ ] **Step 5: Run full test suite**

```
go test ./...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/root.go cmd/root_test.go
git commit -m "fix: return usage_error (exit 2) for missing required flags instead of unknown_error"
```

---

## Chunk 2: Non-Interactive Init

### Task 3: Add env var support to init

**Files:**
- Modify: `cmd/initcmd.go`
- Create: `cmd/initcmd_test.go`

**Background:** `runInit` currently reads from `bufio.NewReader(os.Stdin)` at the top of the function. We must check env vars *before* touching stdin. Extract a `readEnvInputs()` helper that handles the env var detection, prefix validation, and one-of-two error. `runInit` checks it first; if it returns non-nil inputs it takes the non-interactive path without ever reading stdin.

- [ ] **Step 1: Write failing tests for readEnvInputs**

Create `cmd/initcmd_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
)

func TestReadEnvInputs_NeitherSet(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "")
	t.Setenv("SLACKLINE_APP_TOKEN", "")
	t.Setenv("SLACKLINE_WORKSPACE_URL", "")

	inputs, err := readEnvInputs()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if inputs != nil {
		t.Fatalf("expected nil inputs (interactive mode), got %+v", inputs)
	}
}

func TestReadEnvInputs_BothSet(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxb-valid-token")
	t.Setenv("SLACKLINE_APP_TOKEN", "xapp-valid-token")
	t.Setenv("SLACKLINE_WORKSPACE_URL", "https://myteam.slack.com")

	inputs, err := readEnvInputs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inputs == nil {
		t.Fatal("expected inputs, got nil")
	}
	if inputs.botToken != "xoxb-valid-token" {
		t.Errorf("botToken = %q, want %q", inputs.botToken, "xoxb-valid-token")
	}
	if inputs.appToken != "xapp-valid-token" {
		t.Errorf("appToken = %q, want %q", inputs.appToken, "xapp-valid-token")
	}
	if inputs.workspaceURL != "https://myteam.slack.com" {
		t.Errorf("workspaceURL = %q, want %q", inputs.workspaceURL, "https://myteam.slack.com")
	}
}

func TestReadEnvInputs_OnlyBotSet(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxb-valid-token")
	t.Setenv("SLACKLINE_APP_TOKEN", "")

	_, err := readEnvInputs()
	if err == nil {
		t.Fatal("expected error when only bot token is set")
	}
	se, ok := err.(*errs.SlackError)
	if !ok {
		t.Fatalf("expected *errs.SlackError, got %T", err)
	}
	if se.Code != errs.Usage {
		t.Errorf("exit code = %d, want %d (Usage)", se.Code, errs.Usage)
	}
}

func TestReadEnvInputs_OnlyAppSet(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "")
	t.Setenv("SLACKLINE_APP_TOKEN", "xapp-valid-token")

	_, err := readEnvInputs()
	if err == nil {
		t.Fatal("expected error when only app token is set")
	}
	se, ok := err.(*errs.SlackError)
	if !ok {
		t.Fatalf("expected *errs.SlackError, got %T", err)
	}
	if se.Code != errs.Usage {
		t.Errorf("exit code = %d, want %d (Usage)", se.Code, errs.Usage)
	}
}

func TestReadEnvInputs_BadBotPrefix(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxp-wrong-type")
	t.Setenv("SLACKLINE_APP_TOKEN", "xapp-valid-token")

	_, err := readEnvInputs()
	if err == nil {
		t.Fatal("expected error for wrong bot token prefix")
	}
	se, ok := err.(*errs.SlackError)
	if !ok {
		t.Fatalf("expected *errs.SlackError, got %T", err)
	}
	if se.Code != errs.Usage {
		t.Errorf("exit code = %d, want %d (Usage)", se.Code, errs.Usage)
	}
}

func TestReadEnvInputs_BadAppPrefix(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxb-valid-token")
	t.Setenv("SLACKLINE_APP_TOKEN", "xoxb-wrong-type")

	_, err := readEnvInputs()
	if err == nil {
		t.Fatal("expected error for wrong app token prefix")
	}
	se, ok := err.(*errs.SlackError)
	if !ok {
		t.Fatalf("expected *errs.SlackError, got %T", err)
	}
	if se.Code != errs.Usage {
		t.Errorf("exit code = %d, want %d (Usage)", se.Code, errs.Usage)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/ -run TestReadEnvInputs -v
```

Expected: FAIL — compile error (`readEnvInputs` not defined)

- [ ] **Step 3: Add readEnvInputs helper and initEnvInputs struct to initcmd.go**

Add before `runInit` (after the imports):

```go
type initEnvInputs struct {
	botToken     string
	appToken     string
	workspaceURL string
}

// readEnvInputs checks environment variables for non-interactive init.
// Returns (nil, nil) if neither token var is set — interactive mode.
// Returns (nil, error) if exactly one token is set or a prefix is wrong.
// Returns (*initEnvInputs, nil) if both tokens are set and valid.
func readEnvInputs() (*initEnvInputs, error) {
	bot := os.Getenv("SLACKLINE_BOT_TOKEN")
	app := os.Getenv("SLACKLINE_APP_TOKEN")
	url := os.Getenv("SLACKLINE_WORKSPACE_URL")

	if bot == "" && app == "" {
		return nil, nil
	}
	if bot == "" || app == "" {
		return nil, &errs.SlackError{
			Code:   errs.Usage,
			Err:    "missing_token",
			Detail: "set both SLACKLINE_BOT_TOKEN and SLACKLINE_APP_TOKEN for non-interactive mode",
		}
	}
	if !strings.HasPrefix(bot, "xoxb-") {
		return nil, &errs.SlackError{
			Code:   errs.Usage,
			Err:    "invalid_token",
			Detail: "SLACKLINE_BOT_TOKEN must start with 'xoxb-'",
		}
	}
	if !strings.HasPrefix(app, "xapp-") {
		return nil, &errs.SlackError{
			Code:   errs.Usage,
			Err:    "invalid_token",
			Detail: "SLACKLINE_APP_TOKEN must start with 'xapp-'",
		}
	}
	return &initEnvInputs{botToken: bot, appToken: app, workspaceURL: url}, nil
}
```

`strings` is already imported in `initcmd.go`. Verify `os` is imported too (it is).

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/ -run TestReadEnvInputs -v
```

Expected: PASS (all 5 tests)

- [ ] **Step 4.5: Acknowledge integration test scope**

The `runInit` non-interactive path calls `slackpkg.NewClient()` directly (no injected dependency), making it impossible to unit test the auth.test call or config.Save() without a real Slack token. This is consistent with existing `cmd` package patterns — `runAsk`, `runSend`, and `runRead` are also not unit tested at the cobra command level. The `readEnvInputs` unit tests (Steps 1–4) cover the input validation logic exhaustively. The non-interactive `runInit` integration is accepted as requiring a manual smoke test with real tokens.

Manual smoke test (run after implementing Step 5, with real tokens):
```bash
SLACKLINE_BOT_TOKEN=xoxb-... SLACKLINE_APP_TOKEN=xapp-... ./slackline init
# Expected: ✓ Config written to ~/.config/slackline/config.json
```

- [ ] **Step 5: Update runInit to use readEnvInputs**

Replace the opening of `runInit` with:

```go
func runInit(cmd *cobra.Command, args []string) error {
	// Check for non-interactive mode via env vars before touching stdin.
	envInputs, err := readEnvInputs()
	if err != nil {
		return err
	}
	if envInputs != nil {
		api := slackpkg.NewClient(envInputs.botToken)
		authResp, authErr := api.AuthTest()
		if authErr != nil {
			if isAuthError(authErr) {
				return errs.AuthError(authErr.Error())
			}
			return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: fmt.Sprintf("Bot token validation failed: %v", authErr)}
		}

		cfgPath := cfgFile
		if cfgPath == "" {
			cfgPath = os.Getenv("SLACKLINE_CONFIG")
		}
		if cfgPath == "" {
			cfgPath = config.DefaultPath()
		}

		cfg := &config.Config{
			Version: 1,
			Workspace: config.Workspace{
				Name:   authResp.Team,
				TeamID: authResp.TeamID,
				URL:    envInputs.workspaceURL,
			},
			Bot: config.Bot{
				Name:     authResp.User,
				BotToken: envInputs.botToken,
				AppToken: envInputs.appToken,
			},
		}
		if err := config.Save(cfg, cfgPath); err != nil {
			return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
		}

		fmt.Fprintf(os.Stderr, "\n✓ Config written to %s\n", cfgPath)
		fmt.Fprintf(os.Stderr, "  Bot: %s (via auth.test)\n", authResp.User)
		fmt.Fprintf(os.Stderr, "  Workspace: %s\n", authResp.Team)
		return nil
	}

	// Interactive path — existing code follows unchanged.
	reader := bufio.NewReader(os.Stdin)
	// ... (rest of existing runInit body)
```

Do not change anything after the `reader := bufio.NewReader(os.Stdin)` line.

- [ ] **Step 6: Run full test suite**

```
go test ./...
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/initcmd.go cmd/initcmd_test.go
git commit -m "feat: add non-interactive init via SLACKLINE_BOT_TOKEN/SLACKLINE_APP_TOKEN env vars"
```

---

## Chunk 3: Help Text Enrichment

### Task 4: Update root command Long

**Files:**
- Modify: `cmd/root.go`

- [ ] **Step 1: Extend Long in rootCmd**

The current `Long` is a single sentence. Extend it by replacing the `Long` field:

```go
Long: `A CLI tool for AI agents to send messages, read channels, and listen for events in Slack.

Getting started:
  slackline init       Configure with existing bot and app tokens
  slackline create     Provision a new Slack app (requires admin token)

All errors are written to stderr as JSON: {"error":"<code>","detail":"<message>"}

Exit codes: 0 success, 1 Slack API error, 2 usage error, 3 config error, 4 auth failure`,
```

- [ ] **Step 2: Run tests and smoke test**

```
go test ./...
go run . --help
```

Expected: tests PASS; help shows the getting-started block and exit codes line.

- [ ] **Step 3: Commit**

```bash
git add cmd/root.go
git commit -m "docs: extend root command help with getting-started guidance and exit codes"
```

---

### Task 5: Update send, ask, read, listen help

**Files:**
- Modify: `cmd/send.go`
- Modify: `cmd/ask.go`
- Modify: `cmd/read.go`
- Modify: `cmd/listen.go`

- [ ] **Step 1: Update sendCmd Long and Example**

In `cmd/send.go`, replace the `Long` field and add `Example`:

```go
var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to a Slack channel",
	Long: `Send a message to a channel. Message can be passed via --message or piped via stdin.
Note: --message is not marked required because stdin is a valid alternative.
Missing message is caught at runtime with exit code 2.

Output: {"ok":true,"channel":"C...","ts":"..."}
Exit codes: 0 success, 1 Slack API error, 2 usage, 3 config, 4 auth`,
	Example: `  slackline send --channel '#ops' --message 'deploying now'
  echo 'deploying now' | slackline send --channel C1234567890
  slackline send --channel '#ops' --message 'reply' --thread 1234567890.123456`,
	RunE: runSend,
}
```

- [ ] **Step 2: Update askCmd Long and Example**

In `cmd/ask.go`, replace the `Long` field and add `Example`:

```go
var askCmd = &cobra.Command{
	Use:   "ask",
	Short: "Send a message and wait for a reply",
	Long: `Sends a message to a channel and polls the thread for replies from other users.
Exits 0 when a reply is received. Exits 1 on timeout or Slack API error; distinguish
via the JSON error field ({"error":"timeout",...} vs {"error":"poll_failed",...}).
Note: --message is not marked required because stdin is a valid alternative.

Output: JSONL — one {"ts":"...","user":"...","text":"..."} per reply line
Exit codes: 0 reply received, 1 timeout or Slack API error, 2 usage, 3 config, 4 auth`,
	Example: `  slackline ask --channel '#ops' --message 'ready?' --timeout 60
  echo 'ready?' | slackline ask --channel '#ops'`,
	RunE: runAsk,
}
```

- [ ] **Step 3: Update readCmd Long and Example; fix RFC 3339 label**

In `cmd/read.go`:

1. Replace the `Long` field and add `Example` to `readCmd`:

```go
var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read messages from a Slack channel",
	Long: `Read messages from a channel or thread. Output is JSONL (one message per line).
--since accepts RFC 3339 timestamps (e.g. 2024-01-01T00:00:00Z).

Output: JSONL — one {"ts":"...","user":"...","text":"..."} per message line
Exit codes: 0 success, 1 Slack API error, 2 usage, 3 config, 4 auth`,
	Example: `  slackline read --channel '#ops' --limit 50
  slackline read --channel '#ops' --thread 1234567890.123456
  slackline read --channel '#ops' --since 2024-01-01T00:00:00Z`,
	RunE: runRead,
}
```

2. In `init()`, update the `--since` flag description from `"only return messages after this ISO 8601 timestamp"` to `"only return messages after this RFC 3339 timestamp"`.

3. In `runRead`, update the error detail from `"Failed to parse --since as ISO 8601: %v"` to `"Failed to parse --since as RFC 3339: %v"`.

- [ ] **Step 4: Update listenCmd Long and Example**

In `cmd/listen.go`, replace the `Long` field and add `Example`:

```go
var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Listen for real-time Slack events",
	Long: `Connect via Socket Mode and stream @mentions, DMs, and reactions as JSONL to stdout.
Requires app token (xapp-) for Socket Mode. Streams until interrupted.
No usage errors (no required flags).

Output: JSONL to stdout — {"type":"...","user":"...","text":"...","channel":"...","ts":"..."}
Exit codes: 0 clean exit, 1 connection error, 3 config, 4 auth`,
	Example: `  slackline listen
  slackline listen 2>/dev/null | jq .`,
	RunE: runListen,
}
```

- [ ] **Step 5: Run tests and smoke test**

```
go test ./...
go run . send --help
go run . ask --help
go run . read --help
go run . listen --help
```

Expected: tests PASS; each command shows updated Long and an Examples section.

- [ ] **Step 6: Commit**

```bash
git add cmd/send.go cmd/ask.go cmd/read.go cmd/listen.go
git commit -m "docs: add output format, exit codes, and examples to send/ask/read/listen help"
```

---

### Task 6: Update channels, auth, init, create help

**Files:**
- Modify: `cmd/channels.go`
- Modify: `cmd/auth.go`
- Modify: `cmd/initcmd.go`
- Modify: `cmd/create.go`

- [ ] **Step 1: Update channelsCmd Long and Example**

In `cmd/channels.go`, add `Long` and `Example` fields to `channelsCmd` (currently only has `Short`):

```go
var channelsCmd = &cobra.Command{
	Use:   "channels",
	Short: "List Slack channels visible to the bot",
	Long: `List Slack channels visible to the bot. Defaults to channels the bot has joined.
JSON output includes topic; the table omits topic for display width.

Output: table (ID, NAME, PURPOSE) by default; with --json: array of
        {"id":"C...","name":"...","topic":"...","purpose":"..."}
Exit codes: 0 success, 1 Slack API error, 3 config, 4 auth`,
	Example: `  slackline channels
  slackline channels --json
  slackline channels --all --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// existing body — do not change
	},
}
```

- [ ] **Step 2: Update authStatusCmd Long and Example**

In `cmd/auth.go`, add `Long` and `Example` to `authStatusCmd`:

```go
var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	Long: `Check whether configured tokens are valid and display bot identity.
Token validation errors are reported in the output text — this command always exits 0
unless the config file is missing or unreadable (exit 3).
Exit codes 1, 2, and 4 are never returned by this command.

Output: plain text — bot name, workspace, token status, config path
Exit codes: 0 success (including invalid tokens), 3 config error
Note: exit codes 1, 2, and 4 are never returned by this command.`,
	Example: `  slackline auth status`,
	RunE:    runAuthStatus,
}
```

- [ ] **Step 3: Update initCmd Long and Example**

In `cmd/initcmd.go`, replace the `Long` field and add `Example` to `initCmd`:

```go
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Configure slackline with existing tokens",
	Long: `Set up slackline on a new machine using tokens provisioned by an admin.
Supports non-interactive mode via environment variables for agent use.

Environment variables (non-interactive mode):
  SLACKLINE_BOT_TOKEN     Bot token (xoxb-)
  SLACKLINE_APP_TOKEN     App token (xapp-)
  SLACKLINE_WORKSPACE_URL Workspace URL, e.g. https://myteam.slack.com (optional)

If both SLACKLINE_BOT_TOKEN and SLACKLINE_APP_TOKEN are set, all stdin prompts are skipped.
If exactly one is set, init exits with a usage error (exit 2).
Token validation via auth.test is performed in both interactive and non-interactive modes.

Exit codes: 0 success, 2 usage, 3 config, 4 auth`,
	Example: `  slackline init
  SLACKLINE_BOT_TOKEN=xoxb-... SLACKLINE_APP_TOKEN=xapp-... slackline init`,
	RunE: runInit,
}
```

- [ ] **Step 4: Update createCmd Long and Example**

In `cmd/create.go`, replace the `Long` field and add `Example` to `createCmd`:

```go
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Slack bot app (admin)",
	Long: `Create a Slack app via the manifest API. Requires an App Configuration Token.
Interactive — not suitable for agent use.

Exit codes: 0 success, 1 Slack API error, 2 usage, 3 config, 4 auth`,
	Example: `  slackline create --name mybot
  slackline create --init`,
	RunE: runCreate,
}
```

- [ ] **Step 5: Run tests and smoke test**

```
go test ./...
go run . channels --help
go run . auth status --help
go run . init --help
go run . create --help
```

Expected: tests PASS; each shows updated Long with output format, exit codes, and Examples.

- [ ] **Step 6: Commit**

```bash
git add cmd/channels.go cmd/auth.go cmd/initcmd.go cmd/create.go
git commit -m "docs: add output format, exit codes, and examples to channels/auth/init/create help"
```
