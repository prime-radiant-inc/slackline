# Slackline Implementation Plan

> **Executor:** superpowers:executing-waves

**Goal:** Build a cross-platform Go CLI that gives AI agents a Slack identity — handling app provisioning, messaging, channel discovery, and real-time event listening via Socket Mode.

**Architecture:** Single Go binary using Cobra for subcommands and `slack-go/slack` for all Slack API interaction. One config file per bot identity. All output is JSON/JSONL to stdout; errors are JSON to stderr with typed exit codes.

**Tech Stack:** Go 1.26, `github.com/slack-go/slack`, `github.com/spf13/cobra`, Go standard library only beyond that. No CGo.

**Spec:** `docs/specs/2026-03-16-slackline-design.md`

**Linear:** PRI-722

## Execution Summary

- **Waves:** 5
- **Total tasks:** 17
- **Max parallel tasks:** 9 (Wave 4)
- **Sequential time:** ~6h (17 tasks × ~20 min avg)
- **Parallel time:** ~2h
- **Time saved:** ~4h (67%)

| Wave | Tasks | Parallel | What |
|------|-------|----------|------|
| 1 | T1 | 1 | Project scaffold (go.mod, root cmd) |
| 2 | T2, T4, T11, T14 | 4 | Independent packages (errs, slack, listen, provision) |
| 3 | T3, T5 | 2 | Config + channel resolution |
| 4 | T6-T10, T12, T13, T15, T17 | 9 | All commands + polish |
| 5 | T16 | 1 | Build verification |

**Dependency analysis:** File overlap exists only on `cmd/root.go` (T1→T2→T3). All other dependencies are compile-time (import) ordering — each wave creates packages that later waves import. Wave boundaries ensure all imported packages exist before consumers run.

---

## File Structure

```
slackline/
├── main.go                      # Entry point — calls cmd.Execute()
├── go.mod
├── go.sum
├── cmd/
│   ├── root.go                  # Root Cobra command, global --config flag, config loading
│   ├── send.go                  # slackline send
│   ├── read.go                  # slackline read
│   ├── ask.go                   # slackline ask
│   ├── listen.go                # slackline listen
│   ├── channels.go              # slackline channels
│   ├── auth.go                  # slackline auth + auth status
│   ├── initcmd.go               # slackline init (not init.go — avoids Go init() confusion)
│   └── create.go                # slackline create
├── config/
│   ├── config.go                # Config + ProvisionConfig structs, Load/Save, env overrides
│   └── config_test.go           # Config round-trip, env override, validation tests
├── slack/
│   ├── api.go                   # SlackAPI interface (our abstraction over slack-go)
│   ├── client.go                # NewClient() factory — returns real slack-go client as SlackAPI
│   ├── resolve.go               # ResolveChannel(): #name / C... ID / URL → channel ID
│   └── resolve_test.go          # Resolution logic tests (URL parsing, ID passthrough, name lookup)
├── provision/
│   ├── manifest.go              # Embedded app manifest template + generation
│   ├── manifest_test.go         # Manifest scope/event correctness tests
│   ├── tokens.go                # Config token rotation (refresh flow)
│   └── tokens_test.go           # Token rotation logic tests
├── listen/
│   ├── events.go                # Event types (Mention, DM, Reaction) + JSONL marshal
│   ├── events_test.go           # Event marshaling tests
│   └── listener.go              # Socket Mode event loop, self-filter, JSONL writer
└── errs/
    ├── errors.go                # Exit codes (0-4), JSON error writer to stderr
    └── errors_test.go           # Error formatting tests
```

**Design notes:**
- `slack/api.go` defines a `SlackAPI` interface wrapping the subset of `slack-go` methods we use. This insulates us from slack-go breaking changes (it's pre-1.0) and allows unit testing command logic with test doubles.
- Test doubles are used to feed known data into our logic — tests assert on *our* behavior, not on mock call counts.
- `cmd/` files are thin: parse flags, load config, call into packages, handle errors. Business logic lives in the packages.
- `config/` handles both the bot config (`config.json`) and provision config (`provision.json`) in a single package since they share the same directory and loading patterns.
- **Spec deviation:** `errs/` is not in the spec's architecture diagram. It's added here because error handling (exit codes, JSON stderr formatting) is cross-cutting and deserves its own package rather than being inlined in every command. This is an intentional addition.

---

## Wave 1: Scaffold (1 task)

### Task 1: Project Scaffold

**Dependencies:** None

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `cmd/root.go`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/drewritter/prime-rad/slackline
go mod init github.com/prime-radiant/slackline
```

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/spf13/cobra@latest
go get github.com/slack-go/slack@latest
```

After `go get` resolves, note the exact slack-go version pinned in `go.mod`. The spec references v0.19.0 (March 2026). Since slack-go is pre-1.0, minor versions can contain breaking changes. If `@latest` resolves to a different version, use that but be aware of potential API differences — the interface in Task 4 should be verified against the pinned version.

- [ ] **Step 3: Create `main.go`**

```go
package main

import (
	"os"

	"github.com/prime-radiant/slackline/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
```

`Execute()` returns an int exit code. `main()` calls `os.Exit()` with it. This keeps exit code control at the top level and lets commands return typed exit codes.

- [ ] **Step 4: Create `cmd/root.go`**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "slackline",
	Short: "Give AI agents a Slack identity",
	Long:  "A CLI tool for AI agents to send messages, read channels, and listen for events in Slack.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/slackline/config.json)")
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
```

- [ ] **Step 5: Verify it builds and runs**

```bash
go build -o slackline .
./slackline --help
```

Expected: help text showing "Give AI agents a Slack identity" and `--config` flag.

- [ ] **Step 6: Commit**

```bash
git add main.go go.mod go.sum cmd/root.go
git commit -m "feat: project scaffold with cobra root command"
```

---

## Wave 2: Independent Packages (parallel — 4 tasks)

### Task 2: Error Handling Package

**Dependencies:** Task 1 (shares `cmd/root.go`)

**Files:**
- Create: `errs/errors.go`
- Create: `errs/errors_test.go`

The error package defines exit codes and a JSON error writer. Every command uses this for consistent error output.

Exit codes from spec:
- `0` — success
- `1` — Slack API error
- `2` — auth error
- `3` — config error
- `4` — usage error

- [ ] **Step 1: Write failing tests for error formatting**

```go
package errs

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteError_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, "channel_not_found", "Could not find channel #nonexistent")

	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nbody: %s", err, buf.String())
	}
	if got["error"] != "channel_not_found" {
		t.Errorf("error = %q, want %q", got["error"], "channel_not_found")
	}
	if got["detail"] != "Could not find channel #nonexistent" {
		t.Errorf("detail = %q, want %q", got["detail"], "Could not find channel #nonexistent")
	}
}

func TestWriteError_TrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, "test", "test detail")
	out := buf.String()
	if out[len(out)-1] != '\n' {
		t.Error("output should end with newline")
	}
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"Success", Success, 0},
		{"SlackAPI", SlackAPI, 1},
		{"Auth", Auth, 2},
		{"Config", Config, 3},
		{"Usage", Usage, 4},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestSlackError_ImplementsError(t *testing.T) {
	err := &SlackError{Code: SlackAPI, Err: "channel_not_found", Detail: "not found"}
	if err.Error() != "channel_not_found: not found" {
		t.Errorf("Error() = %q, want %q", err.Error(), "channel_not_found: not found")
	}
	if err.ExitCode() != SlackAPI {
		t.Errorf("ExitCode() = %d, want %d", err.ExitCode(), SlackAPI)
	}
}

func TestAuthError(t *testing.T) {
	err := AuthError("token_revoked")
	se, ok := err.(*SlackError)
	if !ok {
		t.Fatal("AuthError should return *SlackError")
	}
	if se.ExitCode() != Auth {
		t.Errorf("ExitCode() = %d, want %d", se.ExitCode(), Auth)
	}
	if se.Detail != "Token invalid or revoked. Run 'slackline init' to reconfigure." {
		t.Errorf("unexpected detail: %s", se.Detail)
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

```bash
cd /Users/drewritter/prime-rad/slackline
go test ./errs/...
```

Expected: compilation errors (package doesn't exist yet).

- [ ] **Step 3: Implement error package**

```go
package errs

import (
	"encoding/json"
	"fmt"
	"io"
)

// Exit codes per spec.
const (
	Success  = 0
	SlackAPI = 1
	Auth     = 2
	Config   = 3
	Usage    = 4
)

// SlackError is a structured error with an exit code.
type SlackError struct {
	Code   int
	Err    string
	Detail string
}

func (e *SlackError) Error() string {
	return fmt.Sprintf("%s: %s", e.Err, e.Detail)
}

func (e *SlackError) ExitCode() int {
	return e.Code
}

// WriteError writes a JSON error object to w.
func WriteError(w io.Writer, errCode string, detail string) {
	obj := struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}{
		Error:  errCode,
		Detail: detail,
	}
	data, _ := json.Marshal(obj)
	fmt.Fprintln(w, string(data))
}

// AuthError returns a SlackError for authentication failures.
// The detail message tells the user to re-run init.
func AuthError(slackErr string) error {
	return &SlackError{
		Code:   Auth,
		Err:    slackErr,
		Detail: "Token invalid or revoked. Run 'slackline init' to reconfigure.",
	}
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test ./errs/... -v
```

Expected: all pass.

- [ ] **Step 5: Wire exit codes into `cmd/root.go`**

Update `Execute()` to extract typed exit codes from `SlackError`:

```go
// Replace Execute() in cmd/root.go:

import (
	"errors"
	"os"

	"github.com/prime-radiant/slackline/errs"
	"github.com/spf13/cobra"
)

func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		var se *errs.SlackError
		if errors.As(err, &se) {
			errs.WriteError(os.Stderr, se.Err, se.Detail)
			return se.ExitCode()
		}
		errs.WriteError(os.Stderr, "unknown_error", err.Error())
		return 1
	}
	return 0
}
```

Remove the `fmt` import from root.go (no longer needed).

- [ ] **Step 6: Verify build**

```bash
go build -o slackline .
```

- [ ] **Step 7: Commit**

```bash
git add errs/ cmd/root.go
git commit -m "feat: error handling package with exit codes and JSON output"
```

---

## Wave 3: Config & Channel Resolution (parallel — 2 tasks)

### Task 3: Config Package

**Dependencies:** Task 2 (shares `cmd/root.go`)

**Files:**
- Create: `config/config.go`
- Create: `config/config_test.go`

Handles loading/saving the bot config file and provision config file. Supports env var overrides.

- [ ] **Step 1: Write failing tests**

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	want := &Config{
		Version: 1,
		Workspace: Workspace{
			Name:   "Test",
			TeamID: "T123",
			URL:    "https://test.slack.com",
		},
		Bot: Bot{
			Name:     "test-bot",
			AppID:    "A123",
			BotToken: "xoxb-test",
			AppToken: "xapp-test",
		},
	}

	if err := Save(want, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if got.Version != want.Version {
		t.Errorf("Version = %d, want %d", got.Version, want.Version)
	}
	if got.Workspace.Name != want.Workspace.Name {
		t.Errorf("Workspace.Name = %q, want %q", got.Workspace.Name, want.Workspace.Name)
	}
	if got.Bot.BotToken != want.Bot.BotToken {
		t.Errorf("Bot.BotToken = %q, want %q", got.Bot.BotToken, want.Bot.BotToken)
	}
	if got.Bot.AppToken != want.Bot.AppToken {
		t.Errorf("Bot.AppToken = %q, want %q", got.Bot.AppToken, want.Bot.AppToken)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{Version: 1, Bot: Bot{BotToken: "xoxb-x"}}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file perm = %o, want 0600", perm)
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "config.json")

	cfg := &Config{Version: 1}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("parent dir stat: %v", err)
	}
	if perm := parentInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("dir perm = %o, want 0700", perm)
	}
}

func TestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Version: 1,
		Bot: Bot{
			BotToken: "xoxb-from-file",
			AppToken: "xapp-from-file",
		},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxb-from-env")
	t.Setenv("SLACKLINE_APP_TOKEN", "xapp-from-env")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Bot.BotToken != "xoxb-from-env" {
		t.Errorf("BotToken = %q, want env override %q", got.Bot.BotToken, "xoxb-from-env")
	}
	if got.Bot.AppToken != "xapp-from-env" {
		t.Errorf("AppToken = %q, want env override %q", got.Bot.AppToken, "xapp-from-env")
	}
}

func TestEnvOnly(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxb-env-only")
	t.Setenv("SLACKLINE_APP_TOKEN", "xapp-env-only")

	got, err := Load("") // No file path
	if err != nil {
		t.Fatalf("Load with no file: %v", err)
	}

	if got.Bot.BotToken != "xoxb-env-only" {
		t.Errorf("BotToken = %q, want %q", got.Bot.BotToken, "xoxb-env-only")
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDefaultPath(t *testing.T) {
	p := DefaultPath()
	if filepath.Base(p) != "config.json" {
		t.Errorf("DefaultPath base = %q, want config.json", filepath.Base(p))
	}
	if filepath.Base(filepath.Dir(p)) != "slackline" {
		t.Errorf("DefaultPath parent dir = %q, want slackline", filepath.Base(filepath.Dir(p)))
	}
}

func TestProvisionConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provision.json")

	want := &ProvisionConfig{
		ConfigToken:  "xoxe-config",
		RefreshToken: "xoxe-refresh",
	}

	if err := SaveProvision(want, path); err != nil {
		t.Fatalf("SaveProvision: %v", err)
	}

	got, err := LoadProvision(path)
	if err != nil {
		t.Fatalf("LoadProvision: %v", err)
	}

	if got.ConfigToken != want.ConfigToken {
		t.Errorf("ConfigToken = %q, want %q", got.ConfigToken, want.ConfigToken)
	}
	if got.RefreshToken != want.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, want.RefreshToken)
	}
}

func TestSLACKLINE_CONFIG_EnvVar(t *testing.T) {
	// The SLACKLINE_CONFIG env var is handled in cmd/root.go's loadConfig(),
	// not in the config package itself. This test verifies that Load() works
	// correctly when given an explicit path (which is what loadConfig passes
	// after resolving the env var).
	dir := t.TempDir()
	path := filepath.Join(dir, "custom", "config.json")

	cfg := &Config{
		Version: 1,
		Bot:     Bot{BotToken: "xoxb-custom-path", AppToken: "xapp-x"},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load with explicit path: %v", err)
	}
	if got.Bot.BotToken != "xoxb-custom-path" {
		t.Errorf("BotToken = %q, want xoxb-custom-path", got.Bot.BotToken)
	}
}

func TestSave_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Version: 1,
		Bot:     Bot{BotToken: "xoxb-x", AppToken: "xapp-x"},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(path)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
	if _, ok := raw["version"]; !ok {
		t.Error("saved JSON missing 'version' field")
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

```bash
go test ./config/... -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement config package**

```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the bot identity config written to config.json.
type Config struct {
	Version   int       `json:"version"`
	Workspace Workspace `json:"workspace,omitempty"`
	Bot       Bot       `json:"bot"`
}

type Workspace struct {
	Name   string `json:"name,omitempty"`
	TeamID string `json:"team_id,omitempty"`
	URL    string `json:"url,omitempty"`
}

type Bot struct {
	Name     string `json:"name"`
	AppID    string `json:"app_id"`
	BotToken string `json:"bot_token"`
	AppToken string `json:"app_token"`
}

// ProvisionConfig holds admin-level credentials for app creation.
// Stored separately from bot config in provision.json.
type ProvisionConfig struct {
	ConfigToken  string `json:"config_token"`
	RefreshToken string `json:"refresh_token"`
}

// DefaultPath returns ~/.config/slackline/config.json.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "slackline", "config.json")
}

// DefaultProvisionPath returns ~/.config/slackline/provision.json.
func DefaultProvisionPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "slackline", "provision.json")
}

// Load loads config from the given path (or default) with env var overrides.
// If path is empty and no default file exists, returns a config populated
// only from env vars.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}

	cfg, err := LoadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if cfg == nil {
		cfg = &Config{Version: 1}
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

// LoadFile loads config from an exact file path. Returns error if file missing.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Save writes config to path with 0600 perms. Creates parent dirs with 0700.
func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// LoadProvision loads the provision config from the given path.
func LoadProvision(path string) (*ProvisionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read provision config: %w", err)
	}
	var cfg ProvisionConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse provision config: %w", err)
	}
	return &cfg, nil
}

// SaveProvision writes provision config with 0600 perms.
func SaveProvision(cfg *ProvisionConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create provision dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal provision config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write provision config: %w", err)
	}
	return nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SLACKLINE_BOT_TOKEN"); v != "" {
		cfg.Bot.BotToken = v
	}
	if v := os.Getenv("SLACKLINE_APP_TOKEN"); v != "" {
		cfg.Bot.AppToken = v
	}
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test ./config/... -v
```

Expected: all pass.

- [ ] **Step 5: Wire config loading into root command**

Update `cmd/root.go` to resolve config path from `--config` flag → `SLACKLINE_CONFIG` env var → default:

```go
// Add to cmd/root.go — helper used by subcommands to load config.

import "github.com/prime-radiant/slackline/config"

func loadConfig() (*config.Config, string, error) {
	path := cfgFile
	if path == "" {
		path = os.Getenv("SLACKLINE_CONFIG")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, "", err
	}
	// Resolve the effective path for display
	if path == "" {
		path = config.DefaultPath()
	}
	return cfg, path, nil
}
```

- [ ] **Step 6: Verify build**

```bash
go build -o slackline .
```

Expected: clean build.

- [ ] **Step 7: Commit**

```bash
git add config/ cmd/root.go
git commit -m "feat: config package with load/save and env var overrides"
```

---

### Task 4: Slack API Interface and Client Factory [Wave 2]

**Dependencies:** Task 1 (needs `go.mod`)

**Files:**
- Create: `slack/api.go`
- Create: `slack/client.go`

This defines the `SlackAPI` interface — our abstraction over `slack-go/slack`. All command code depends on this interface, never on `*slack.Client` directly. This insulates us from slack-go breaking changes (pre-1.0 library) and enables unit testing with test doubles.

**Important for implementer:** Verify all method signatures against the pinned `slack-go/slack` version in `go.mod`. The signatures below are based on slack-go v0.14+. If a signature differs, update the interface to match — the wrapping adapter in `client.go` must satisfy the interface without extra glue.

- [ ] **Step 1: Create `slack/api.go` — the interface**

```go
package slack

import (
	goslack "github.com/slack-go/slack"
)

// SlackAPI is the subset of slack-go methods used by slackline.
// All command logic depends on this interface, not on *slack.Client directly.
//
// Signatures match slack-go's *Client methods exactly so that
// goslack.New() satisfies this interface without an adapter.
type SlackAPI interface {
	// AuthTest validates the bot token and returns identity info.
	AuthTest() (response *goslack.AuthTestResponse, err error)

	// PostMessage sends a message to a channel. Returns (channel, timestamp, err).
	PostMessage(channelID string, options ...goslack.MsgOption) (string, string, error)

	// GetConversationHistory fetches messages from a channel.
	GetConversationHistory(params *goslack.GetConversationHistoryParameters) (*goslack.GetConversationHistoryResponse, error)

	// GetConversationReplies fetches thread replies. Returns (messages, paging, err).
	// Use paging.NextCursor to check for more pages.
	GetConversationReplies(params *goslack.GetConversationRepliesParameters) ([]goslack.Message, bool, string, error)

	// GetConversations lists channels visible to the bot.
	// Returns (channels, nextCursor, err).
	GetConversations(params *goslack.GetConversationsParameters) ([]goslack.Channel, string, error)
}

// IMPORTANT FOR IMPLEMENTER: The above signatures are based on slack-go v0.14-v0.19.
// Verify against your pinned version. Known variations across versions:
//
// GetConversationReplies may return:
//   - ([]Message, bool, string, error)  — older versions
//   - ([]Message, *Paging, error)       — newer versions
//
// GetConversations may return:
//   - ([]Channel, string, error)                    — older versions
//   - ([]Channel, *GetConversationsParameters, error) — newer versions (cursor in params)
//
// If the pinned version uses different return types, update this interface
// to match, then update all call sites. If *slack.Client doesn't directly
// satisfy the interface, write a thin adapter struct in client.go.
```

- [ ] **Step 2: Create `slack/client.go` — the real client factory**

```go
package slack

import (
	goslack "github.com/slack-go/slack"
)

// NewClient creates a real Slack API client from a bot token.
// The returned client implements SlackAPI via slack-go's *Client methods.
func NewClient(botToken string) SlackAPI {
	return goslack.New(botToken)
}
```

`goslack.New()` returns `*goslack.Client`, which already has all the methods in our interface — no adapter needed.

- [ ] **Step 3: Verify build**

```bash
go build ./slack/...
```

Expected: compiles. If `*goslack.Client` doesn't satisfy `SlackAPI`, the implementer needs to write a thin adapter struct.

- [ ] **Step 4: Commit**

```bash
git add slack/
git commit -m "feat: SlackAPI interface and client factory"
```

---

### Task 5: Channel Resolution [Wave 3]

**Dependencies:** Task 4 (imports `slack/api.go`)

**Files:**
- Create: `slack/resolve.go`
- Create: `slack/resolve_test.go`

Channel resolution accepts `#name`, `C...` ID, or Slack URL and returns a channel ID. Name resolution requires calling the Slack API (paginated `GetConversations`). ID and URL resolution are pure string parsing.

- [ ] **Step 1: Write failing tests for pure resolution logic**

```go
package slack

import (
	"testing"

	goslack "github.com/slack-go/slack"
)

func TestResolveChannel_RawID(t *testing.T) {
	// Raw channel IDs (C..., G..., D...) pass through without API calls.
	r := NewResolver(nil) // nil API — should not be called
	tests := []struct {
		input string
		want  string
	}{
		{"C0A8LJZQSAX", "C0A8LJZQSAX"},
		{"G0A2GP2FRRC", "G0A2GP2FRRC"},
		{"D0AA4MWTX45", "D0AA4MWTX45"},
	}
	for _, tt := range tests {
		got, err := r.Resolve(tt.input)
		if err != nil {
			t.Errorf("Resolve(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveChannel_URL(t *testing.T) {
	// Slack URLs contain the channel ID in the path.
	r := NewResolver(nil)
	tests := []struct {
		input string
		want  string
	}{
		{"https://prime-radiant-inc.slack.com/archives/C0A8LJZQSAX", "C0A8LJZQSAX"},
		{"https://app.slack.com/client/T0A2XMY5117/C0A8LJZQSAX", "C0A8LJZQSAX"},
		{"https://prime-radiant-inc.slack.com/archives/C0A8LJZQSAX/p1769756026624319", "C0A8LJZQSAX"},
	}
	for _, tt := range tests {
		got, err := r.Resolve(tt.input)
		if err != nil {
			t.Errorf("Resolve(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveChannel_InvalidURL(t *testing.T) {
	r := NewResolver(nil)
	_, err := r.Resolve("https://example.com/not-a-slack-url")
	if err == nil {
		t.Error("expected error for non-Slack URL")
	}
}

// fakeAPI implements SlackAPI for testing channel name resolution.
type fakeAPI struct {
	channels []goslack.Channel
}

func (f *fakeAPI) AuthTest() (*goslack.AuthTestResponse, error) { return nil, nil }
func (f *fakeAPI) PostMessage(string, ...goslack.MsgOption) (string, string, error) {
	return "", "", nil
}
func (f *fakeAPI) GetConversationHistory(*goslack.GetConversationHistoryParameters) (*goslack.GetConversationHistoryResponse, error) {
	return nil, nil
}
func (f *fakeAPI) GetConversationReplies(*goslack.GetConversationRepliesParameters) ([]goslack.Message, bool, string, error) {
	return nil, false, "", nil
}

// NOTE FOR IMPLEMENTER: If your pinned slack-go version uses different return
// types for GetConversationReplies or GetConversations, update these fakes to match.
func (f *fakeAPI) GetConversations(params *goslack.GetConversationsParameters) ([]goslack.Channel, string, error) {
	return f.channels, "", nil
}

func TestResolveChannel_Name(t *testing.T) {
	api := &fakeAPI{
		channels: []goslack.Channel{
			{GroupConversation: goslack.GroupConversation{
				Name: "ops", Conversation: goslack.Conversation{ID: "C0A8LJZQSAX"},
			}},
			{GroupConversation: goslack.GroupConversation{
				Name: "general", Conversation: goslack.Conversation{ID: "C0A2GP2FRRC"},
			}},
		},
	}
	r := NewResolver(api)

	got, err := r.Resolve("#ops")
	if err != nil {
		t.Fatalf("Resolve(#ops) error: %v", err)
	}
	if got != "C0A8LJZQSAX" {
		t.Errorf("Resolve(#ops) = %q, want C0A8LJZQSAX", got)
	}
}

func TestResolveChannel_NameNotFound(t *testing.T) {
	api := &fakeAPI{
		channels: []goslack.Channel{
			{GroupConversation: goslack.GroupConversation{
				Name: "ops", Conversation: goslack.Conversation{ID: "C123"},
			}},
		},
	}
	r := NewResolver(api)

	_, err := r.Resolve("#nonexistent")
	if err == nil {
		t.Error("expected error for channel not found")
	}
}

func TestResolveChannel_NameCached(t *testing.T) {
	callCount := 0
	api := &countingFakeAPI{
		channels: []goslack.Channel{
			{GroupConversation: goslack.GroupConversation{
				Name: "ops", Conversation: goslack.Conversation{ID: "C123"},
			}},
		},
		callCount: &callCount,
	}
	r := NewResolver(api)

	// First call fetches from API
	r.Resolve("#ops")
	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}

	// Second call should use cache
	r.Resolve("#ops")
	if callCount != 1 {
		t.Errorf("expected still 1 API call after cache hit, got %d", callCount)
	}
}

type countingFakeAPI struct {
	fakeAPI
	channels  []goslack.Channel
	callCount *int
}

func (f *countingFakeAPI) GetConversations(params *goslack.GetConversationsParameters) ([]goslack.Channel, string, error) {
	*f.callCount++
	return f.channels, "", nil
}

func TestResolveChannel_PrefersActiveOverArchived(t *testing.T) {
	api := &fakeAPI{
		channels: []goslack.Channel{
			{GroupConversation: goslack.GroupConversation{
				Name: "ops", Conversation: goslack.Conversation{ID: "C_ARCHIVED", IsArchived: true},
			}},
			{GroupConversation: goslack.GroupConversation{
				Name: "ops", Conversation: goslack.Conversation{ID: "C_ACTIVE"},
			}},
		},
	}
	r := NewResolver(api)

	got, err := r.Resolve("#ops")
	if err != nil {
		t.Fatalf("Resolve(#ops) error: %v", err)
	}
	if got != "C_ACTIVE" {
		t.Errorf("Resolve(#ops) = %q, want C_ACTIVE (active over archived)", got)
	}
}
```

Note: The `fakeAPI` is a test double that feeds known data into our resolution logic. Tests assert on *our* resolution behavior (caching, URL parsing, name matching, archived preference) — not on whether we called the mock correctly.

**Implementer note:** The `goslack.Channel` struct nesting may differ across slack-go versions. The key fields are accessed via `channel.ID`, `channel.Name`, `channel.IsArchived`. Verify the struct layout and adjust test construction if needed.

- [ ] **Step 2: Run tests — verify they fail**

```bash
go test ./slack/... -v
```

Expected: compilation errors (Resolver doesn't exist yet).

- [ ] **Step 3: Implement channel resolution**

```go
package slack

import (
	"fmt"
	"net/url"
	"strings"

	goslack "github.com/slack-go/slack"
)

// Resolver resolves channel references (#name, C... ID, URL) to channel IDs.
type Resolver struct {
	api            SlackAPI
	cache          map[string]string // name → ID, populated on first name lookup
	cachePopulated bool              // true after first full fetch — avoids re-fetching on misses
}

// NewResolver creates a channel resolver. api can be nil if only ID/URL
// resolution is needed (no name lookups).
func NewResolver(api SlackAPI) *Resolver {
	return &Resolver{api: api}
}

// Resolve takes a channel reference and returns a channel ID.
// Accepts: "#channel-name", "C0A8LJZQSAX", or a Slack URL.
func (r *Resolver) Resolve(ref string) (string, error) {
	// Raw channel ID — starts with C, G, or D followed by uppercase alphanumeric
	if isChannelID(ref) {
		return ref, nil
	}

	// Slack URL — extract channel ID from path
	if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
		return r.resolveURL(ref)
	}

	// Channel name — strip # prefix and look up via API
	if strings.HasPrefix(ref, "#") {
		name := strings.TrimPrefix(ref, "#")
		return r.resolveName(name)
	}

	return "", fmt.Errorf("unrecognized channel reference: %q (use #name, channel ID, or Slack URL)", ref)
}

func isChannelID(s string) bool {
	if len(s) < 2 {
		return false
	}
	prefix := s[0]
	if prefix != 'C' && prefix != 'G' && prefix != 'D' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

func (r *Resolver) resolveURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.Contains(u.Host, "slack.com") {
		return "", fmt.Errorf("not a Slack URL: %s", rawURL)
	}

	// URL patterns:
	// /archives/C0A8LJZQSAX
	// /archives/C0A8LJZQSAX/p1769756026624319
	// /client/T.../C0A8LJZQSAX
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, part := range parts {
		if isChannelID(part) {
			return part, nil
		}
	}
	return "", fmt.Errorf("no channel ID found in URL: %s", rawURL)
}

func (r *Resolver) resolveName(name string) (string, error) {
	if r.api == nil {
		return "", fmt.Errorf("cannot resolve channel name %q: no API client", name)
	}

	// Check cache first
	if r.cache != nil {
		if id, ok := r.cache[name]; ok {
			return id, nil
		}
		// Cache exists and was fully populated — name doesn't exist
		if r.cachePopulated {
			return "", fmt.Errorf("channel %q not found", name)
		}
	}

	// Fetch all channels and build cache
	if err := r.populateCache(); err != nil {
		return "", fmt.Errorf("channel lookup failed: %w", err)
	}

	id, ok := r.cache[name]
	if !ok {
		return "", fmt.Errorf("channel %q not found", name)
	}
	if strings.HasPrefix(id, "AMBIGUOUS:") {
		ids := strings.TrimPrefix(id, "AMBIGUOUS:")
		return "", fmt.Errorf("multiple active channels named %q (%s) — use a channel ID instead", name, ids)
	}
	return id, nil
}

func (r *Resolver) populateCache() error {
	r.cache = make(map[string]string)
	// Track archived channels and ambiguous active names separately
	archived := make(map[string]string)
	ambiguous := make(map[string][]string) // name → list of active channel IDs

	cursor := ""
	for {
		channels, nextCursor, err := r.api.GetConversations(&goslack.GetConversationsParameters{
			Types:           "public_channel,private_channel",
			ExcludeArchived: false,
			Limit:           200,
			Cursor:          cursor,
		})
		if err != nil {
			return err
		}

		for _, ch := range channels {
			if ch.IsArchived {
				if _, exists := archived[ch.Name]; !exists {
					archived[ch.Name] = ch.ID
				}
			} else {
				// Track all active channels per name for ambiguity detection
				ambiguous[ch.Name] = append(ambiguous[ch.Name], ch.ID)
				r.cache[ch.Name] = ch.ID
			}
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	// Mark ambiguous names — multiple active channels with same name
	// These will be caught at resolve time and return an error
	for name, ids := range ambiguous {
		if len(ids) > 1 {
			// Store a sentinel — resolve will detect and error
			r.cache[name] = "AMBIGUOUS:" + strings.Join(ids, ",")
		}
	}

	// Backfill archived channels only where no active channel has the same name
	for name, id := range archived {
		if _, exists := r.cache[name]; !exists {
			r.cache[name] = id
		}
	}

	r.cachePopulated = true
	return nil
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test ./slack/... -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add slack/
git commit -m "feat: channel resolution with name/ID/URL support and caching"
```

---

## Wave 4: All Commands (parallel — 9 tasks)

**TDD note for cmd layer:** The `cmd/` files are thin Cobra wrappers — most business logic lives in packages (`slack/`, `listen/`, `config/`, `errs/`) which are fully TDD'd. The command implementations below don't include separate test files because Cobra command handlers are hard to unit test without extracting every function. The implementer SHOULD extract testable helper functions where logic is non-trivial (e.g., `fetchHistory`, `fetchReplies`, `maskToken`, output formatting) and write tests for those. At minimum, test the output format for `send` (JSON) and `read` (JSONL) to ensure spec compliance.

**Shared utilities:** `isAuthError()` is defined in `cmd/send.go` but used by multiple commands. Since all cmd files are in the same package, this works. However, the implementer should move it to `cmd/root.go` to make the shared nature explicit. Alternatively, move it to the `errs` package if it grows.

### Task 6: `send` Command

**Dependencies:** Tasks 2, 3, 4, 5 (imports `errs`, `config`, `slack`)

**Files:**
- Create: `cmd/send.go`

The `send` command sends a message to a channel. Reads message from `--message` flag or stdin. Outputs JSON with `ts`, `channel`, and optionally `thread_ts`.

- [ ] **Step 1: Create `cmd/send.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prime-radiant/slackline/errs"
	slackpkg "github.com/prime-radiant/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var sendChannel string
var sendMessage string
var sendThread string

func init() {
	sendCmd.Flags().StringVar(&sendChannel, "channel", "", "channel name (#ops), ID (C...), or Slack URL (required)")
	sendCmd.Flags().StringVar(&sendMessage, "message", "", "message text (reads stdin if omitted)")
	sendCmd.Flags().StringVar(&sendThread, "thread", "", "thread timestamp to reply to")
	sendCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(sendCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to a Slack channel",
	Long:  "Send a message to a channel. Message can be passed via --message or piped via stdin.",
	RunE:  runSend,
}

func runSend(cmd *cobra.Command, args []string) error {
	// Get message content
	text := sendMessage
	if text == "" {
		// Read from stdin — but only if stdin is not a TTY
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return &errs.SlackError{
				Code:   errs.Usage,
				Err:    "no_message",
				Detail: "Provide --message or pipe message via stdin",
			}
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return &errs.SlackError{
				Code:   errs.Usage,
				Err:    "stdin_read_error",
				Detail: fmt.Sprintf("Failed to read stdin: %v", err),
			}
		}
		text = strings.TrimRight(string(data), "\n")
	}

	if text == "" {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "empty_message",
			Detail: "Message cannot be empty",
		}
	}

	// Load config
	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "No bot token configured. Run 'slackline init' to set up."}
	}

	// Create client and resolve channel
	api := slackpkg.NewClient(cfg.Bot.BotToken)
	resolver := slackpkg.NewResolver(api)
	channelID, err := resolver.Resolve(sendChannel)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "channel_not_found", Detail: err.Error()}
	}

	// Build message options
	opts := []goslack.MsgOption{goslack.MsgOptionText(text, false)}
	if sendThread != "" {
		opts = append(opts, goslack.MsgOptionTS(sendThread))
	}

	// Send
	respChannel, ts, err := api.PostMessage(channelID, opts...)
	if err != nil {
		// Check for auth errors
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "send_failed", Detail: err.Error()}
	}

	// Output
	out := struct {
		OK       bool   `json:"ok"`
		Channel  string `json:"channel"`
		TS       string `json:"ts"`
		ThreadTS string `json:"thread_ts,omitempty"`
	}{
		OK:       true,
		Channel:  respChannel,
		TS:       ts,
		ThreadTS: sendThread,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// isAuthError checks if a Slack API error is an authentication failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "token_revoked" || msg == "invalid_auth" || msg == "not_authed" || msg == "account_inactive"
}
```

- [ ] **Step 2: Build and verify help**

```bash
go build -o slackline .
./slackline send --help
```

Expected: shows `--channel`, `--message`, `--thread` flags.

- [ ] **Step 3: Commit**

```bash
git add cmd/send.go
git commit -m "feat: send command with stdin support and channel resolution"
```

---

### Task 7: `read` Command

**Dependencies:** Tasks 2, 3, 4, 5 (imports `errs`, `config`, `slack`)

**Files:**
- Create: `cmd/read.go`

Reads messages from a channel. Outputs JSONL (one message per line). Supports `--limit`, `--thread`, `--since`. Results in chronological order.

- [ ] **Step 1: Create `cmd/read.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/prime-radiant/slackline/errs"
	slackpkg "github.com/prime-radiant/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var readChannel string
var readLimit int
var readThread string
var readSince string

func init() {
	readCmd.Flags().StringVar(&readChannel, "channel", "", "channel name (#ops), ID (C...), or Slack URL (required)")
	readCmd.Flags().IntVar(&readLimit, "limit", 20, "max messages to return")
	readCmd.Flags().StringVar(&readThread, "thread", "", "thread timestamp to read replies from")
	readCmd.Flags().StringVar(&readSince, "since", "", "only messages after this ISO 8601 timestamp")
	readCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(readCmd)
}

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read messages from a Slack channel",
	Long:  "Read messages from a channel or thread. Output is JSONL (one message per line) in chronological order.",
	RunE:  runRead,
}

// messageOutput is the JSONL output for a single message.
// Uses omitempty to strip null/empty fields per spec.
type messageOutput struct {
	TS       string `json:"ts"`
	User     string `json:"user,omitempty"`
	Text     string `json:"text"`
	ThreadTS string `json:"thread_ts,omitempty"`
}

func runRead(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "No bot token configured. Run 'slackline init' to set up."}
	}

	api := slackpkg.NewClient(cfg.Bot.BotToken)
	resolver := slackpkg.NewResolver(api)
	channelID, err := resolver.Resolve(readChannel)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "channel_not_found", Detail: err.Error()}
	}

	// Convert --since to Unix timestamp string for Slack API
	var oldest string
	if readSince != "" {
		t, err := time.Parse(time.RFC3339, readSince)
		if err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "invalid_since", Detail: fmt.Sprintf("--since must be ISO 8601: %v", err)}
		}
		oldest = fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
	}

	var messages []goslack.Message

	if readThread != "" {
		// Read thread replies
		messages, err = fetchReplies(api, channelID, readThread, oldest, readLimit)
	} else {
		// Read channel history
		messages, err = fetchHistory(api, channelID, oldest, readLimit)
	}
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "read_failed", Detail: err.Error()}
	}

	// Output JSONL — messages are already in chronological order from fetch functions
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	for _, msg := range messages {
		out := messageOutput{
			TS:       msg.Timestamp,
			User:     msg.User,
			Text:     msg.Text,
			ThreadTS: msg.ThreadTimestamp,
		}
		// Strip thread_ts when it equals ts (not a threaded message)
		if out.ThreadTS == out.TS {
			out.ThreadTS = ""
		}
		enc.Encode(out)
	}
	return nil
}

// fetchHistory fetches channel messages, paginating as needed, returning
// up to limit messages in chronological order (oldest first).
func fetchHistory(api slackpkg.SlackAPI, channelID, oldest string, limit int) ([]goslack.Message, error) {
	var all []goslack.Message
	cursor := ""
	remaining := limit

	for remaining > 0 {
		pageSize := remaining
		if pageSize > 100 {
			pageSize = 100 // Slack max per request
		}

		resp, err := api.GetConversationHistory(&goslack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Oldest:    oldest,
			Limit:     pageSize,
			Cursor:    cursor,
		})
		if err != nil {
			return nil, err
		}

		all = append(all, resp.Messages...)
		remaining -= len(resp.Messages)

		if !resp.HasMore || resp.ResponseMetadata.NextCursor == "" {
			break
		}
		cursor = resp.ResponseMetadata.NextCursor
	}

	// Slack returns reverse-chronological; reverse to chronological
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}

	// Trim to limit (pagination may overshoot)
	if len(all) > limit {
		all = all[len(all)-limit:] // Keep the most recent `limit` messages
	}

	return all, nil
}

// fetchReplies fetches thread replies, paginating as needed, returning
// up to limit messages in chronological order.
func fetchReplies(api slackpkg.SlackAPI, channelID, threadTS, oldest string, limit int) ([]goslack.Message, error) {
	var all []goslack.Message
	cursor := ""
	remaining := limit

	for remaining > 0 {
		pageSize := remaining
		if pageSize > 100 {
			pageSize = 100
		}

		msgs, hasMore, nextCursor, err := api.GetConversationReplies(&goslack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Oldest:    oldest,
			Limit:     pageSize,
			Cursor:    cursor,
		})
		if err != nil {
			return nil, err
		}

		all = append(all, msgs...)
		remaining -= len(msgs)

		if !hasMore || nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	// Replies are already chronological from Slack API
	if len(all) > limit {
		all = all[:limit]
	}

	return all, nil
}

```

**Implementer note:** Verify `GetConversationHistoryResponse` field names. The response contains `Messages []Message`, `HasMore bool`, and `ResponseMetadata.NextCursor` — confirm the exact struct path for the cursor field in the pinned slack-go version.

- [ ] **Step 2: Build and verify help**

```bash
go build -o slackline .
./slackline read --help
```

Expected: shows `--channel`, `--limit`, `--thread`, `--since` flags.

- [ ] **Step 3: Commit**

```bash
git add cmd/read.go
git commit -m "feat: read command with JSONL output and pagination"
```

---

### Task 8: `channels` Command

**Dependencies:** Tasks 2, 3, 4 (imports `errs`, `config`, `slack`)

**Files:**
- Create: `cmd/channels.go`

Lists channels. Default: channels the bot is in. `--all`: all visible channels. `--json`: JSON output instead of table.

- [ ] **Step 1: Create `cmd/channels.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/prime-radiant/slackline/errs"
	slackpkg "github.com/prime-radiant/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var channelsAll bool
var channelsJSON bool

func init() {
	channelsCmd.Flags().BoolVar(&channelsAll, "all", false, "list all visible channels (not just joined)")
	channelsCmd.Flags().BoolVar(&channelsJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(channelsCmd)
}

var channelsCmd = &cobra.Command{
	Use:   "channels",
	Short: "List Slack channels",
	Long:  "List channels the bot is in, or all visible channels with --all.",
	RunE:  runChannels,
}

type channelOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Topic   string `json:"topic,omitempty"`
	Purpose string `json:"purpose,omitempty"`
}

func runChannels(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "No bot token configured. Run 'slackline init' to set up."}
	}

	api := slackpkg.NewClient(cfg.Bot.BotToken)

	// Fetch channels
	var allChannels []goslack.Channel
	cursor := ""
	for {
		channels, nextCursor, err := api.GetConversations(&goslack.GetConversationsParameters{
			Types:           "public_channel,private_channel",
			ExcludeArchived: true,
			Limit:           200,
			Cursor:          cursor,
		})
		if err != nil {
			if isAuthError(err) {
				return errs.AuthError(err.Error())
			}
			return &errs.SlackError{Code: errs.SlackAPI, Err: "channels_failed", Detail: err.Error()}
		}
		allChannels = append(allChannels, channels...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	// Filter to joined channels unless --all
	var filtered []goslack.Channel
	for _, ch := range allChannels {
		if channelsAll || ch.IsMember {
			filtered = append(filtered, ch)
		}
	}

	if channelsJSON {
		// JSON output
		var out []channelOutput
		for _, ch := range filtered {
			out = append(out, channelOutput{
				ID:      ch.ID,
				Name:    ch.Name,
				Topic:   ch.Topic.Value,
				Purpose: ch.Purpose.Value,
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Table output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, ch := range filtered {
		purpose := ch.Purpose.Value
		if len(purpose) > 60 {
			purpose = purpose[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t#%s\t%s\n", ch.ID, ch.Name, purpose)
	}
	return w.Flush()
}
```

**Implementer note:** Verify `ch.IsMember`, `ch.Topic.Value`, and `ch.Purpose.Value` field paths exist in the pinned slack-go version. These are typical for `slack.Channel` but struct nesting may vary.

- [ ] **Step 2: Build and verify help**

```bash
go build -o slackline .
./slackline channels --help
```

Expected: shows `--all` and `--json` flags.

- [ ] **Step 3: Commit**

```bash
git add cmd/channels.go
git commit -m "feat: channels command with table and JSON output"
```

---

### Task 9: `auth status` Command

**Dependencies:** Tasks 2, 3, 4 (imports `errs`, `config`, `slack`)

**Files:**
- Create: `cmd/auth.go`

Displays current config status and validates tokens via `auth.test`.

- [ ] **Step 1: Create `cmd/auth.go`**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/prime-radiant/slackline/errs"
	slackpkg "github.com/prime-radiant/slackline/slack"
	"github.com/spf13/cobra"
)

func init() {
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication commands",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check configuration and token validity",
	RunE:  runAuthStatus,
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	cfg, cfgPath, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}

	// Validate bot token
	botStatus := "not configured"
	botName := ""
	workspace := ""
	teamID := ""
	if cfg.Bot.BotToken != "" {
		api := slackpkg.NewClient(cfg.Bot.BotToken)
		resp, err := api.AuthTest()
		if err != nil {
			botStatus = fmt.Sprintf("invalid (%s)", err.Error())
		} else {
			botStatus = "valid"
			botName = resp.User
			workspace = resp.Team
			teamID = resp.TeamID
		}
	}

	// Validate app token — use auth.test (not apps.connections.open which opens a WebSocket)
	appStatus := "not configured"
	if cfg.Bot.AppToken != "" {
		// App tokens can be validated via auth.test too
		appAPI := slackpkg.NewClient(cfg.Bot.AppToken)
		_, err := appAPI.AuthTest()
		if err != nil {
			appStatus = fmt.Sprintf("invalid (%s)", err.Error())
		} else {
			appStatus = "valid"
		}
	}

	// Mask tokens for display
	botTokenDisplay := maskToken(cfg.Bot.BotToken)
	appTokenDisplay := maskToken(cfg.Bot.AppToken)

	fmt.Fprintf(os.Stdout, "Bot:       %s\n", botName)
	fmt.Fprintf(os.Stdout, "Workspace: %s (%s)\n", workspace, teamID)
	fmt.Fprintf(os.Stdout, "Bot Token: %s (%s)\n", botTokenDisplay, botStatus)
	fmt.Fprintf(os.Stdout, "App Token: %s (%s)\n", appTokenDisplay, appStatus)
	fmt.Fprintf(os.Stdout, "Config:    %s\n", cfgPath)

	return nil
}

// maskToken shows the prefix and last 4 chars: "xoxb-...XXXX"
func maskToken(token string) string {
	if token == "" {
		return "(none)"
	}
	if len(token) <= 8 {
		return token[:4] + "-..."
	}
	return token[:5] + "..." + token[len(token)-4:]
}
```

- [ ] **Step 2: Build and verify**

```bash
go build -o slackline .
./slackline auth status --help
```

Expected: shows the `auth status` subcommand.

- [ ] **Step 3: Commit**

```bash
git add cmd/auth.go
git commit -m "feat: auth status command with token validation"
```

---

### Task 10: `ask` Command [Wave 4]

**Dependencies:** Tasks 2, 3, 4, 5 (imports `errs`, `config`, `slack`)

**Files:**
- Create: `cmd/ask.go`

Sends a message, then polls the thread for replies from other users. Exits 0 when a reply arrives, exits 1 on timeout.

- [ ] **Step 1: Create `cmd/ask.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/prime-radiant/slackline/errs"
	slackpkg "github.com/prime-radiant/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var askChannel string
var askMessage string
var askTimeout int
var askPoll int

func init() {
	askCmd.Flags().StringVar(&askChannel, "channel", "", "channel name, ID, or Slack URL (required)")
	askCmd.Flags().StringVar(&askMessage, "message", "", "message text (reads stdin if omitted)")
	askCmd.Flags().IntVar(&askTimeout, "timeout", 300, "max seconds to wait for a reply")
	askCmd.Flags().IntVar(&askPoll, "poll", 10, "seconds between polls")
	askCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(askCmd)
}

var askCmd = &cobra.Command{
	Use:   "ask",
	Short: "Send a message and wait for a reply",
	Long:  "Send a message, then poll the thread for replies. Exits 0 on reply, 1 on timeout.",
	RunE:  runAsk,
}

func runAsk(cmd *cobra.Command, args []string) error {
	// Get message text (same logic as send)
	text := askMessage
	if text == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return &errs.SlackError{Code: errs.Usage, Err: "no_message", Detail: "Provide --message or pipe message via stdin"}
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "stdin_read_error", Detail: fmt.Sprintf("Failed to read stdin: %v", err)}
		}
		text = strings.TrimRight(string(data), "\n")
	}
	if text == "" {
		return &errs.SlackError{Code: errs.Usage, Err: "empty_message", Detail: "Message cannot be empty"}
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "No bot token configured. Run 'slackline init' to set up."}
	}

	api := slackpkg.NewClient(cfg.Bot.BotToken)
	resolver := slackpkg.NewResolver(api)
	channelID, err := resolver.Resolve(askChannel)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "channel_not_found", Detail: err.Error()}
	}

	// Get bot user ID for self-filtering
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: err.Error()}
	}
	botUserID := authResp.UserID

	// Send the message
	_, ts, err := api.PostMessage(channelID, goslack.MsgOptionText(text, false))
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "send_failed", Detail: err.Error()}
	}

	// Poll for replies
	deadline := time.Now().Add(time.Duration(askTimeout) * time.Second)
	pollInterval := time.Duration(askPoll) * time.Second

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		msgs, _, _, err := api.GetConversationReplies(&goslack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: ts,
		})
		if err != nil {
			if isAuthError(err) {
				return errs.AuthError(err.Error())
			}
			return &errs.SlackError{Code: errs.SlackAPI, Err: "poll_failed", Detail: err.Error()}
		}

		// Filter: replies from others (not the bot itself), excluding the parent message
		var replies []goslack.Message
		for _, msg := range msgs {
			if msg.Timestamp == ts {
				continue // Skip the parent message we sent
			}
			if msg.User == botUserID {
				continue // Skip our own replies
			}
			replies = append(replies, msg)
		}

		if len(replies) > 0 {
			// Print all new replies as JSONL
			enc := json.NewEncoder(os.Stdout)
			enc.SetEscapeHTML(false)
			for _, msg := range replies {
				out := messageOutput{
					TS:       msg.Timestamp,
					User:     msg.User,
					Text:     msg.Text,
					ThreadTS: msg.ThreadTimestamp,
				}
				enc.Encode(out)
			}
			return nil // Exit 0
		}
	}

	// Timeout
	return &errs.SlackError{Code: errs.SlackAPI, Err: "timeout", Detail: fmt.Sprintf("No reply received within %d seconds", askTimeout)}
}
```

- [ ] **Step 2: Build and verify help**

```bash
go build -o slackline .
./slackline ask --help
```

Expected: shows `--channel`, `--message`, `--timeout`, `--poll` flags.

- [ ] **Step 3: Commit**

```bash
git add cmd/ask.go
git commit -m "feat: ask command with poll-based reply detection"
```

---

### Task 11: Listen Events Package [Wave 2]

**Dependencies:** Task 1 (needs `go.mod`)

**Files:**
- Create: `listen/events.go`
- Create: `listen/events_test.go`

Defines event types (`mention`, `dm`, `reaction`) and their JSONL marshaling. This is pure data + serialization — no Socket Mode yet.

- [ ] **Step 1: Write failing tests**

```go
package listen

import (
	"encoding/json"
	"testing"
)

func TestMentionEvent_JSON(t *testing.T) {
	e := Event{
		Type:     "mention",
		Channel:  "C0A8LJZQSAX",
		User:     "U0123",
		Text:     "hey @drew-claude check the logs",
		TS:       "1769756026.624319",
		ThreadTS: "1769756026.624319",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]interface{}
	json.Unmarshal(data, &got)

	if got["type"] != "mention" {
		t.Errorf("type = %v, want mention", got["type"])
	}
	if got["channel"] != "C0A8LJZQSAX" {
		t.Errorf("channel = %v, want C0A8LJZQSAX", got["channel"])
	}
	if got["text"] != "hey @drew-claude check the logs" {
		t.Errorf("text = %v", got["text"])
	}
}

func TestDMEvent_JSON(t *testing.T) {
	e := Event{
		Type:    "dm",
		Channel: "D0AA4MWTX45",
		User:    "U0456",
		Text:    "can you review this PR?",
		TS:      "1769756030.111111",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]interface{}
	json.Unmarshal(data, &got)

	if got["type"] != "dm" {
		t.Errorf("type = %v, want dm", got["type"])
	}
	// thread_ts should be omitted (empty)
	if _, ok := got["thread_ts"]; ok {
		t.Error("thread_ts should be omitted when empty")
	}
}

func TestReactionEvent_JSON(t *testing.T) {
	e := Event{
		Type:    "reaction",
		Channel: "C0A8LJZQSAX",
		User:    "U0123",
		Emoji:   "eyes",
		ItemTS:  "1769756026.624319",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]interface{}
	json.Unmarshal(data, &got)

	if got["type"] != "reaction" {
		t.Errorf("type = %v, want reaction", got["type"])
	}
	if got["emoji"] != "eyes" {
		t.Errorf("emoji = %v, want eyes", got["emoji"])
	}
	if got["item_ts"] != "1769756026.624319" {
		t.Errorf("item_ts = %v", got["item_ts"])
	}
	// Reaction events should not have text, ts fields
	if _, ok := got["text"]; ok {
		t.Error("reaction event should not have text")
	}
}

func TestEvent_OmitsEmptyFields(t *testing.T) {
	e := Event{
		Type:    "mention",
		Channel: "C123",
		User:    "U123",
		Text:    "hello",
		TS:      "123.456",
	}

	data, _ := json.Marshal(e)
	var got map[string]interface{}
	json.Unmarshal(data, &got)

	// These should be omitted
	for _, key := range []string{"thread_ts", "emoji", "item_ts"} {
		if _, ok := got[key]; ok {
			t.Errorf("field %q should be omitted when empty", key)
		}
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

```bash
go test ./listen/... -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement events package**

```go
package listen

// Event is a unified event emitted by the listener as JSONL.
// Fields are omitted when empty to keep output compact.
type Event struct {
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	User     string `json:"user,omitempty"`
	Text     string `json:"text,omitempty"`
	TS       string `json:"ts,omitempty"`
	ThreadTS string `json:"thread_ts,omitempty"`
	Emoji    string `json:"emoji,omitempty"`
	ItemTS   string `json:"item_ts,omitempty"`
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test ./listen/... -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add listen/
git commit -m "feat: listen event types with JSONL marshaling"
```

---

### Task 12: `listen` Command (Socket Mode) [Wave 4]

**Dependencies:** Tasks 2, 3, 4, 11 (imports `errs`, `config`, `slack`, `listen`)

**Files:**
- Create: `listen/listener.go`
- Create: `cmd/listen.go`

Connects to Slack via Socket Mode, emits JSONL events to stdout, status to stderr. Filters out self-messages. Runs until SIGTERM/SIGINT or stdin closes.

- [ ] **Step 1: Create `listen/listener.go`**

```go
package listen

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

// Listener connects to Slack via Socket Mode and emits events as JSONL.
type Listener struct {
	api       *goslack.Client
	sm        *socketmode.Client
	botUserID string
	out       io.Writer
	status    io.Writer
}

// NewListener creates a Socket Mode listener.
// botToken is the xoxb- token; appToken is the xapp- token.
// botUserID is used to filter self-messages.
func NewListener(botToken, appToken, botUserID string, out, status io.Writer) *Listener {
	api := goslack.New(botToken, goslack.OptionAppLevelToken(appToken))
	sm := socketmode.New(api)
	return &Listener{
		api:       api,
		sm:        sm,
		botUserID: botUserID,
		out:       out,
		status:    status,
	}
}

// Run starts the Socket Mode connection and blocks until interrupted.
// Events are written as JSONL to l.out. Status messages go to l.status.
// Shuts down on SIGTERM, SIGINT, or when stdin is closed (spec requirement).
func (l *Listener) Run() error {
	stop := make(chan struct{}, 1)

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		close(stop)
	}()

	// Monitor stdin for EOF — when parent process exits, stdin closes
	go func() {
		buf := make([]byte, 1)
		for {
			_, err := os.Stdin.Read(buf)
			if err != nil { // EOF or error
				close(stop)
				return
			}
		}
	}()

	go func() {
		for evt := range l.sm.Events {
			l.handleEvent(evt)
		}
	}()

	// Start Socket Mode in background goroutine
	// Note: "connected" status is emitted by handleEvent on EventTypeConnected,
	// not here — sm.Run() hasn't connected yet at this point.
	go l.sm.Run()

	// Block until shutdown signal
	<-stop
	fmt.Fprintln(l.status, "disconnected")
	return nil
}

func (l *Listener) handleEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPIEvent, ok := evt.Data.(goslack.EventsAPIEvent)
		if !ok {
			return
		}
		l.sm.Ack(*evt.Request)
		l.handleEventsAPI(eventsAPIEvent)

	case socketmode.EventTypeConnectionError:
		fmt.Fprintln(l.status, "reconnecting")

	case socketmode.EventTypeConnected:
		fmt.Fprintln(l.status, "connected")
	}
}

func (l *Listener) handleEventsAPI(evt goslack.EventsAPIEvent) {
	switch ev := evt.InnerEvent.Data.(type) {
	case *goslack.AppMentionEvent:
		if ev.User == l.botUserID {
			return // Self-filter
		}
		l.emit(Event{
			Type:     "mention",
			Channel:  ev.Channel,
			User:     ev.User,
			Text:     ev.Text,
			TS:       ev.TimeStamp,
			ThreadTS: ev.ThreadTimeStamp,
		})

	case *goslack.MessageEvent:
		// Only handle DMs (im channel type starts with D)
		if len(ev.Channel) == 0 || ev.Channel[0] != 'D' {
			return
		}
		if ev.User == l.botUserID {
			return // Self-filter: drop our own messages
		}
		// Skip message subtypes (edits, deletes, etc.) — only new messages in v1
		if ev.SubType != "" {
			return
		}
		l.emit(Event{
			Type:     "dm",
			Channel:  ev.Channel,
			User:     ev.User,
			Text:     ev.Text,
			TS:       ev.TimeStamp,
			ThreadTS: ev.ThreadTimeStamp,
		})

	case *goslack.ReactionAddedEvent:
		if ev.User == l.botUserID {
			return // Self-filter
		}
		l.emit(Event{
			Type:    "reaction",
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})
	}
}

func (l *Listener) emit(e Event) {
	// Strip empty thread_ts to match event table in spec
	if e.ThreadTS == "" || e.ThreadTS == e.TS {
		e.ThreadTS = ""
	}
	data, err := json.Marshal(e)
	if err != nil {
		return // Should never happen with simple structs
	}
	fmt.Fprintln(l.out, string(data))
}
```

**Implementer note:** Socket Mode event type names and inner event struct types may vary by slack-go version. Verify:
- `socketmode.EventTypeEventsAPI` exists and is the correct constant
- `goslack.EventsAPIEvent` and its `InnerEvent.Data` type assertion patterns
- `goslack.AppMentionEvent`, `goslack.MessageEvent`, `goslack.ReactionAddedEvent` field names
- `socketmode.New()` signature and whether it needs options
- How `Ack()` works — confirm `*evt.Request` is the right argument

- [ ] **Step 2: Create `cmd/listen.go`**

```go
package cmd

import (
	"os"

	"github.com/prime-radiant/slackline/errs"
	"github.com/prime-radiant/slackline/listen"
	slackpkg "github.com/prime-radiant/slackline/slack"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listenCmd)
}

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Listen for real-time Slack events",
	Long:  "Connect via Socket Mode and stream @mentions, DMs, and reactions as JSONL to stdout.",
	RunE:  runListen,
}

func runListen(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "No bot token configured. Run 'slackline init' to set up."}
	}
	if cfg.Bot.AppToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_app_token", Detail: "No app token configured. Socket Mode requires an app token (xapp-)."}
	}

	// Get bot user ID for self-filtering
	api := slackpkg.NewClient(cfg.Bot.BotToken)
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: err.Error()}
	}

	listener := listen.NewListener(cfg.Bot.BotToken, cfg.Bot.AppToken, authResp.UserID, os.Stdout, os.Stderr)
	return listener.Run()
}
```

- [ ] **Step 3: Build and verify help**

```bash
go build -o slackline .
./slackline listen --help
```

Expected: shows listen command help.

- [ ] **Step 4: Commit**

```bash
git add listen/listener.go cmd/listen.go
git commit -m "feat: listen command with Socket Mode event streaming"
```

---

### Task 13: `init` Command [Wave 4]

**Dependencies:** Tasks 2, 3, 4 (imports `errs`, `config`, `slack`)

**Files:**
- Create: `cmd/initcmd.go`

Interactive token collection for developers who already have tokens. Validates token prefixes, calls `auth.test` to populate workspace/bot metadata, writes config.

- [ ] **Step 1: Create `cmd/initcmd.go`**

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/prime-radiant/slackline/config"
	"github.com/prime-radiant/slackline/errs"
	slackpkg "github.com/prime-radiant/slackline/slack"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Configure slackline with existing tokens",
	Long:  "Set up slackline on a new machine using tokens provisioned by an admin.",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	// Prompt for workspace URL (optional, for display)
	fmt.Print("Workspace URL (e.g. https://myteam.slack.com): ")
	workspaceURL, _ := reader.ReadString('\n')
	workspaceURL = strings.TrimSpace(workspaceURL)

	// Prompt for bot token
	fmt.Print("Bot Token (xoxb-): ")
	botToken, _ := reader.ReadString('\n')
	botToken = strings.TrimSpace(botToken)
	if !strings.HasPrefix(botToken, "xoxb-") {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "invalid_token",
			Detail: "Bot token must start with 'xoxb-'. You may have pasted the wrong token type.",
		}
	}

	// Prompt for app token
	fmt.Print("App Token (xapp-): ")
	appToken, _ := reader.ReadString('\n')
	appToken = strings.TrimSpace(appToken)
	if !strings.HasPrefix(appToken, "xapp-") {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "invalid_token",
			Detail: "App token must start with 'xapp-'. You may have pasted the wrong token type.",
		}
	}

	// Validate bot token via auth.test
	api := slackpkg.NewClient(botToken)
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: fmt.Sprintf("Bot token validation failed: %v", err)}
	}

	// Build config
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
			URL:    workspaceURL,
		},
		Bot: config.Bot{
			Name:     authResp.User,
			BotToken: botToken,
			AppToken: appToken,
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
```

- [ ] **Step 2: Build and verify help**

```bash
go build -o slackline .
./slackline init --help
```

- [ ] **Step 3: Commit**

```bash
git add cmd/initcmd.go
git commit -m "feat: init command for interactive token configuration"
```

---

### Task 14: Manifest Template and Provision Tokens [Wave 2]

**Dependencies:** Task 1 (needs `go.mod`)

**Files:**
- Create: `provision/manifest.go`
- Create: `provision/manifest_test.go`
- Create: `provision/tokens.go`
- Create: `provision/tokens_test.go`

The manifest template is embedded in the binary with all required scopes and event subscriptions. The tokens module handles config token rotation.

- [ ] **Step 1: Write failing tests for manifest**

```go
package provision

import (
	"encoding/json"
	"testing"
)

func TestGenerateManifest_ContainsRequiredScopes(t *testing.T) {
	m := GenerateManifest("test-bot")

	requiredScopes := []string{
		"chat:write",
		"channels:read",
		"groups:read",
		"channels:history",
		"groups:history",
		"app_mentions:read",
		"im:history",
		"im:read",
		"reactions:read",
		"users:read",
	}

	for _, scope := range requiredScopes {
		found := false
		for _, s := range m.OAuthConfig.Scopes.Bot {
			if s == scope {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("manifest missing required bot scope: %s", scope)
		}
	}
}

func TestGenerateManifest_SocketModeEnabled(t *testing.T) {
	m := GenerateManifest("test-bot")
	if !m.Settings.SocketModeEnabled {
		t.Error("socket_mode_enabled should be true")
	}
}

func TestGenerateManifest_EventSubscriptions(t *testing.T) {
	m := GenerateManifest("test-bot")

	requiredEvents := []string{"app_mention", "message.im", "reaction_added"}
	for _, event := range requiredEvents {
		found := false
		for _, e := range m.Settings.EventSubscriptions.BotEvents {
			if e == event {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("manifest missing event subscription: %s", event)
		}
	}
}

func TestGenerateManifest_AppName(t *testing.T) {
	m := GenerateManifest("drew-claude")
	if m.DisplayInfo.Name != "drew-claude" {
		t.Errorf("app name = %q, want drew-claude", m.DisplayInfo.Name)
	}
}

func TestGenerateManifest_ValidJSON(t *testing.T) {
	m := GenerateManifest("test")
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("marshaled manifest is empty")
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

```bash
go test ./provision/... -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement manifest**

```go
package provision

// Manifest represents a Slack app manifest for the manifest API.
// We define our own struct rather than using slack-go's manifest types
// to insulate from pre-1.0 breaking changes.
type Manifest struct {
	DisplayInfo  DisplayInfo  `json:"display_information"`
	Settings     Settings     `json:"settings"`
	OAuthConfig  OAuthConfig  `json:"oauth_config"`
}

type DisplayInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Settings struct {
	SocketModeEnabled  bool               `json:"socket_mode_enabled"`
	EventSubscriptions EventSubscriptions `json:"event_subscriptions"`
}

type EventSubscriptions struct {
	BotEvents []string `json:"bot_events"`
}

type OAuthConfig struct {
	Scopes Scopes `json:"scopes"`
}

type Scopes struct {
	Bot []string `json:"bot"`
}

// GenerateManifest creates a Slack app manifest with the required scopes
// and event subscriptions for slackline.
func GenerateManifest(appName string) *Manifest {
	return &Manifest{
		DisplayInfo: DisplayInfo{
			Name:        appName,
			Description: "Slackline bot identity for AI agents",
		},
		Settings: Settings{
			SocketModeEnabled: true,
			EventSubscriptions: EventSubscriptions{
				BotEvents: []string{
					"app_mention",
					"message.im",
					"reaction_added",
				},
			},
		},
		OAuthConfig: OAuthConfig{
			Scopes: Scopes{
				Bot: []string{
					"chat:write",
					"channels:read",
					"groups:read",
					"channels:history",
					"groups:history",
					"app_mentions:read",
					"im:history",
					"im:read",
					"reactions:read",
					"users:read",
				},
			},
		},
	}
}
```

- [ ] **Step 4: Run manifest tests — verify they pass**

```bash
go test ./provision/... -v
```

- [ ] **Step 5: Write failing tests for token rotation**

```go
package provision

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRotateConfigToken_Success(t *testing.T) {
	// Mock Slack's tooling.tokens.rotate endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tooling.tokens.rotate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"ok":            true,
			"token":         "xoxe-new-config-token",
			"refresh_token": "xoxe-new-refresh-token",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	newConfig, newRefresh, err := RotateConfigToken(server.URL, "xoxe-old-refresh")
	if err != nil {
		t.Fatalf("RotateConfigToken: %v", err)
	}
	if newConfig != "xoxe-new-config-token" {
		t.Errorf("config token = %q, want xoxe-new-config-token", newConfig)
	}
	if newRefresh != "xoxe-new-refresh-token" {
		t.Errorf("refresh token = %q, want xoxe-new-refresh-token", newRefresh)
	}
}

func TestRotateConfigToken_Expired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"ok":    false,
			"error": "token_expired",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	_, _, err := RotateConfigToken(server.URL, "xoxe-expired")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}
```

- [ ] **Step 6: Implement token rotation**

```go
package provision

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const defaultSlackAPIBase = "https://slack.com"

// RotateConfigToken refreshes the App Configuration Token using the refresh token.
// Returns (newConfigToken, newRefreshToken, error).
// apiBase can be overridden for testing; use "" for the default Slack API.
func RotateConfigToken(apiBase, refreshToken string) (string, string, error) {
	if apiBase == "" {
		apiBase = defaultSlackAPIBase
	}

	resp, err := http.PostForm(apiBase+"/api/tooling.tokens.rotate", url.Values{
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return "", "", fmt.Errorf("token rotation request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK           bool   `json:"ok"`
		Error        string `json:"error,omitempty"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("parse rotation response: %w", err)
	}

	if !result.OK {
		return "", "", fmt.Errorf("token rotation failed: %s", result.Error)
	}

	return result.Token, result.RefreshToken, nil
}
```

- [ ] **Step 7: Run all provision tests**

```bash
go test ./provision/... -v
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add provision/
git commit -m "feat: app manifest template and config token rotation"
```

---

### Task 15: `create` Command [Wave 4]

**Dependencies:** Tasks 2, 3, 4, 14 (imports `errs`, `config`, `slack`, `provision`)

**Files:**
- Create: `cmd/create.go`

Admin-only command that creates a Slack app via the manifest API and walks through the token collection flow. Uses the provision package for manifest generation and token rotation.

- [ ] **Step 1: Create `cmd/create.go`**

```go
package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/prime-radiant/slackline/config"
	"github.com/prime-radiant/slackline/errs"
	"github.com/prime-radiant/slackline/provision"
	slackpkg "github.com/prime-radiant/slackline/slack"
	"github.com/spf13/cobra"
)

var createName string
var createInit bool

func init() {
	createCmd.Flags().StringVar(&createName, "name", "", "bot name (required for app creation, not needed with --init alone)")
	createCmd.Flags().BoolVar(&createInit, "init", false, "first-time bootstrap — prompt for config token")
	// NOTE: --name is NOT marked required because --init can be used standalone
	// to bootstrap config tokens without creating an app. Validation is in runCreate.
	rootCmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Slack bot app (admin)",
	Long:  "Create a Slack app via the manifest API. Requires an App Configuration Token.",
	RunE:  runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)
	provPath := config.DefaultProvisionPath()

	// Load or bootstrap provision config
	provCfg, err := config.LoadProvision(provPath)
	if err != nil || createInit {
		// First-time bootstrap
		fmt.Fprintln(os.Stderr, "No config token found. You need to generate one:")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  1. Go to https://api.slack.com/apps")
		fmt.Fprintln(os.Stderr, "  2. Scroll to \"Your App Configuration Tokens\"")
		fmt.Fprintln(os.Stderr, "  3. Click \"Generate Token\" for your workspace")
		fmt.Fprintln(os.Stderr, "")

		fmt.Print("Paste your config token (starts with xoxe-): ")
		configToken, _ := reader.ReadString('\n')
		configToken = strings.TrimSpace(configToken)
		if !strings.HasPrefix(configToken, "xoxe-") {
			return &errs.SlackError{Code: errs.Usage, Err: "invalid_token", Detail: "Config token must start with 'xoxe-'"}
		}

		fmt.Print("Paste your refresh token (starts with xoxe-): ")
		refreshToken, _ := reader.ReadString('\n')
		refreshToken = strings.TrimSpace(refreshToken)
		if !strings.HasPrefix(refreshToken, "xoxe-") {
			return &errs.SlackError{Code: errs.Usage, Err: "invalid_token", Detail: "Refresh token must start with 'xoxe-'"}
		}

		provCfg = &config.ProvisionConfig{
			ConfigToken:  configToken,
			RefreshToken: refreshToken,
		}
		if err := config.SaveProvision(provCfg, provPath); err != nil {
			return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
		}
		fmt.Fprintln(os.Stderr, "\n✓ Config token stored.")
	}

	// If --init was the only goal (no --name), we're done after storing tokens
	if createName == "" {
		return nil
	}

	// Rotate config token (they expire after 12 hours)
	fmt.Fprint(os.Stderr, "Refreshing config token... ")
	newConfig, newRefresh, err := provision.RotateConfigToken("", provCfg.RefreshToken)
	if err != nil {
		if strings.Contains(err.Error(), "token_expired") {
			fmt.Fprintln(os.Stderr, "✗")
			return &errs.SlackError{
				Code:   errs.Auth,
				Err:    "config_token_expired",
				Detail: "Refresh token expired. Re-generate at https://api.slack.com/apps and run 'slackline create --init'.",
			}
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "token_rotation_failed", Detail: err.Error()}
	}
	provCfg.ConfigToken = newConfig
	provCfg.RefreshToken = newRefresh
	if err := config.SaveProvision(provCfg, provPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: fmt.Sprintf("Failed to save rotated tokens: %v", err)}
	}
	fmt.Fprintln(os.Stderr, "✓")

	// Create app via manifest API
	manifest := provision.GenerateManifest(createName)
	manifestJSON, _ := json.Marshal(manifest)

	fmt.Fprintf(os.Stderr, "Creating Slack app %q... ", createName)
	appID, err := createAppViaManifest("", provCfg.ConfigToken, string(manifestJSON))
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗")
		return &errs.SlackError{Code: errs.SlackAPI, Err: "create_app_failed", Detail: err.Error()}
	}
	fmt.Fprintf(os.Stderr, "✓ (app_id: %s)\n", appID)

	// Guide the admin through installation
	fmt.Fprintf(os.Stderr, "\nStep 1: Install the app\n")
	fmt.Fprintf(os.Stderr, "  → https://api.slack.com/apps/%s/install-on-team\n", appID)
	fmt.Fprintf(os.Stderr, "  Click \"Allow\", then press Enter.\n")
	reader.ReadString('\n')

	fmt.Fprintf(os.Stderr, "\nStep 2: Paste Bot Token (xoxb-)\n")
	fmt.Fprintf(os.Stderr, "  → https://api.slack.com/apps/%s/oauth\n", appID)
	fmt.Print("  Token: ")
	botToken, _ := reader.ReadString('\n')
	botToken = strings.TrimSpace(botToken)
	if !strings.HasPrefix(botToken, "xoxb-") {
		return &errs.SlackError{Code: errs.Usage, Err: "invalid_token", Detail: "Bot token must start with 'xoxb-'"}
	}

	fmt.Fprintf(os.Stderr, "\nStep 3: Paste App Token (xapp-)\n")
	fmt.Fprintf(os.Stderr, "  → https://api.slack.com/apps/%s/general\n", appID)
	fmt.Fprintln(os.Stderr, "  Click \"Generate Token\", add connections:write scope.")
	fmt.Print("  Token: ")
	appToken, _ := reader.ReadString('\n')
	appToken = strings.TrimSpace(appToken)
	if !strings.HasPrefix(appToken, "xapp-") {
		return &errs.SlackError{Code: errs.Usage, Err: "invalid_token", Detail: "App token must start with 'xapp-'"}
	}

	// Validate bot token
	api := slackpkg.NewClient(botToken)
	authResp, err := api.AuthTest()
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: fmt.Sprintf("Bot token validation failed: %v", err)}
	}

	// Write config
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
		},
		Bot: config.Bot{
			Name:     createName,
			AppID:    appID,
			BotToken: botToken,
			AppToken: appToken,
		},
	}

	if err := config.Save(cfg, cfgPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
	}

	fmt.Fprintf(os.Stderr, "\n✓ %s ready. Config written to %s\n", createName, cfgPath)
	return nil
}

// createAppViaManifest calls the Slack apps.manifest.create API.
// apiBase can be overridden for testing (e.g., httptest server URL); use "" for default.
func createAppViaManifest(apiBase, configToken, manifestJSON string) (string, error) {
	if apiBase == "" {
		apiBase = "https://slack.com"
	}
	resp, err := http.PostForm(apiBase+"/api/apps.manifest.create", url.Values{
		"token":    {configToken},
		"manifest": {manifestJSON},
	})
	if err != nil {
		return "", fmt.Errorf("manifest create request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
		AppID string `json:"app_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse manifest response: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.AppID, nil
}
```

**Implementer notes:**
- The `apps.manifest.create` endpoint is a direct HTTP call, not via slack-go, because slack-go's manifest API may not match our struct. This keeps us in control.
- The `createAppViaManifest` function should probably be moved to the `provision` package for testability. The implementer may refactor this. If so, also write a test using `httptest.NewServer` similar to the token rotation test.

- [ ] **Step 2: Build and verify help**

```bash
go build -o slackline .
./slackline create --help
```

Expected: shows `--name` and `--init` flags.

- [ ] **Step 3: Commit**

```bash
git add cmd/create.go
git commit -m "feat: create command for admin app provisioning"
```

---

## Wave 5: Verification (1 task — barrier)

### Task 16: End-to-End Build Verification

**Dependencies:** All prior tasks (verification barrier)

This task verifies the full binary builds correctly and all help text renders properly.

**Files:**
- None new — verification only

- [ ] **Step 1: Build**

```bash
cd /Users/drewritter/prime-rad/slackline
go build -o slackline .
```

- [ ] **Step 2: Verify all commands exist**

```bash
./slackline --help
./slackline send --help
./slackline read --help
./slackline ask --help
./slackline channels --help
./slackline listen --help
./slackline auth status --help
./slackline init --help
./slackline create --help
```

Expected: all commands show help with correct flags.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 4: Run go vet**

```bash
go vet ./...
```

Expected: no issues.

- [ ] **Step 5: Commit (if any fixes needed)**

```bash
git status
# Review output — only add files you intentionally changed
git add <specific-files>
git commit -m "fix: resolve build issues from integration"
```

---

### Task 17: Add `.gitignore` and `Makefile` [Wave 4]

**Dependencies:** None (independent files)

**Files:**
- Create: `.gitignore`
- Create: `Makefile`

- [ ] **Step 1: Create `.gitignore`**

```
# Binary
slackline

# Go
*.exe
*.test
*.out
```

- [ ] **Step 2: Create `Makefile`**

```makefile
.PHONY: build test vet clean

build:
	go build -o slackline .

test:
	go test ./... -v

vet:
	go vet ./...

clean:
	rm -f slackline
```

- [ ] **Step 3: Verify**

```bash
make clean build test vet
```

Expected: all targets pass.

- [ ] **Step 4: Commit**

```bash
git add .gitignore Makefile
git commit -m "chore: add .gitignore and Makefile"
```
