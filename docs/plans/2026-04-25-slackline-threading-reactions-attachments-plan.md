# Slackline: Threading, Reactions, Attachments, Scriptable Provisioning — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend slackline with full thread visibility (configurable), symmetric reactions, attachment send/receive, and scriptable bot provisioning so an agent with browser tools can deploy bots end-to-end without per-bot human interaction.

**Architecture:** No package restructuring. New commands (`provision`, `react`, `download`) and new flags on existing commands (`send --attach`, `listen --threads`/`--all-messages`/`--include-bot-self`). Listener gains new event types (`reaction_removed`, `thread_reply`, `channel_message`) and a `files` array on all message events. Manifest gains `reactions:write`, `files:read`, `files:write` scopes and `reaction_removed`/`message.channels`/`message.groups` bot_events. The monolithic `slackline create` is split into `slackline provision bootstrap` (one-time per machine) + `slackline provision <name>` (API-only, machine-readable JSON output) + the existing `slackline init` (env-var-driven). The slackline repo also becomes a Claude Code plugin shipping a new `slackline-provision-bot` skill that documents the agentic recipe.

**Tech Stack:** Go 1.22+, [`spf13/cobra`](https://github.com/spf13/cobra) for the CLI, [`slack-go/slack`](https://github.com/slack-go/slack) for the Slack API client, standard library `net/http`/`net/url` for the raw `files.completeUploadExternal` calls and provisioning endpoints. Tests use `testing` + `httptest` (no real network).

**Spec:** `docs/specs/2026-04-25-slackline-threading-reactions-attachments-design.md` — read before starting.

**Workflow rules (from `CLAUDE.md`):**
- TDD throughout. No implementation without a failing test first.
- Commit after each task (often more frequently — after each step that produces a coherent change).
- Never skip pre-commit hooks (`golangci-lint`, `go vet`) or pre-push hooks (`go test`).
- Match surrounding code style. Use `gofumpt`-compatible formatting.

---

## File Structure

### New files

| Path | Responsibility |
|---|---|
| `cmd/provision.go` | `slackline provision <name>` and `slackline provision bootstrap` subcommands. Calls `tooling.tokens.rotate`, `apps.manifest.create`. |
| `cmd/provision_test.go` | Unit tests for both subcommands using `httptest`. |
| `cmd/react.go` | `slackline react add` and `slackline react remove` subcommands. Idempotent. |
| `cmd/react_test.go` | Unit tests using `fakeSlackAPI`. |
| `cmd/download.go` | `slackline download --file FID --out PATH`. Streams to file or stdout. |
| `cmd/download_test.go` | Unit tests using `httptest` for `url_private` GET. |
| `slack/files.go` | `CompleteUploadExternal` — raw HTTP wrapper around `files.getUploadURLExternal` + `files.completeUploadExternal` for batched multi-file uploads. |
| `slack/files_test.go` | Unit tests using `httptest`. |
| `.claude-plugin/plugin.json` | Slackline plugin manifest. |
| `skills/slackline-provision-bot/SKILL.md` | Agentic recipe for deploying a new bot. |
| `skills/slackline-provision-bot/copy-buttons.md` | CSS selectors reference for the Slack admin UI (progressive disclosure). |

### Modified files

| Path | What changes |
|---|---|
| `cmd/create.go` | Rewritten to a migration-error stub. Cobra still registers the command so users get a discoverable error rather than "unknown command". |
| `cmd/send.go` | Add repeatable `--attach` flag, multi-file upload path, `--message` becomes optional when `--attach` is present. |
| `cmd/send_test.go` (new tests appended to existing file if it exists, else create) | Coverage for `--attach`. |
| `cmd/listen.go` | Wire up new flags (`--threads`, `--all-messages`, `--include-bot-self`) and pass them to `listen.NewListener`. |
| `cmd/read.go` | Include `files` array in `messageOutput` JSONL when present. |
| `cmd/helpers_test.go` | `fakeSlackAPI` gains stub methods for new `SlackAPI` entries. |
| `cmd/initcmd_test.go` | No changes (keep verifying env-var pairing semantics). |
| `listen/events.go` | `Event` struct grows optional fields: `Files []FileMeta`, `ParentUserID string`. New `FileMeta` type. |
| `listen/listener.go` | (a) rename `reaction` event type to `reaction_added`. (b) add `*slackevents.ReactionRemovedEvent` handler. (c) populate `Files` on existing `mention`/`dm` event emissions when present on the source event. (d) handle `*slackevents.MessageEvent` for non-DM channels under `--threads` and `--all-messages` modes. (e) honor `--include-bot-self`. |
| `listen/listener_test.go` | New tests for renamed `reaction_added`, new `reaction_removed`, channel/thread modes, `--include-bot-self`, files-on-events. |
| `slack/api.go` | `SlackAPI` interface gains `AddReaction`, `RemoveReaction`, `UploadFileV2`, `GetFileInfo`, `GetFile`, `CompleteUploadExternal`. |
| `slack/client.go` | Inherits new methods automatically (the underlying `*goslack.Client` implements them) except `CompleteUploadExternal`. Add a tiny shim type that embeds `*goslack.Client` and adds the wrapper. |
| `provision/manifest.go` | `GenerateManifest` signature becomes `GenerateManifest(name, description string, alwaysOnline bool)`. New scopes (`reactions:write`, `files:read`, `files:write`) and bot_events (`reaction_removed`, `message.channels`, `message.groups`). |
| `provision/manifest_test.go` | Add golden-file regression test asserting exact scope and event set; update name/description tests. |
| `CLAUDE.md` | Refresh "Architecture" section: list new packages/files, command surface, event types. Document the `reaction` → `reaction_added` rename and `create` → `provision` migration. |
| `README.md` | Document the new commands and the agentic provisioning recipe (link to the new skill). |

### Out-of-repo updates

| Path | What changes |
|---|---|
| Source repo for `primeradiant-ops:slackline` skill | Update `SKILL.md` to cover `provision`, `react`, `download`, `--attach`, new event shapes; note `reaction` → `reaction_added` migration. (Implementation plan locates the source repo before editing — the cache at `~/.claude/plugins/cache/primeradiant/primeradiant-ops/.../skills/slack-messaging/SKILL.md` is just a snapshot.) |
| Source repo for `superpowers-lab:slack-messaging` skill | Either apply the same updates or mark it deprecated with a clear pointer to the slackline plugin's new skill. |

---

## Task 1: Parameterize and expand `provision/manifest.go`

**Why first:** No behavior change yet — just future-proofs the manifest for the listener and reaction work. Lets us add scopes/events that subsequent tasks rely on, with golden-file coverage that catches accidental drift.

**Files:**
- Modify: `provision/manifest.go`
- Modify: `provision/manifest_test.go`

### Steps

- [ ] **Step 1: Write the failing golden-file test**

Append to `provision/manifest_test.go`:

```go
func TestGenerateManifest_Golden(t *testing.T) {
	m := GenerateManifest("my-bot", "", false)

	wantScopes := map[string]bool{
		"chat:write":         true,
		"channels:read":      true,
		"groups:read":        true,
		"channels:history":   true,
		"groups:history":     true,
		"app_mentions:read":  true,
		"im:history":         true,
		"im:read":            true,
		"reactions:read":     true,
		"reactions:write":    true,
		"users:read":         true,
		"files:read":         true,
		"files:write":        true,
	}
	gotScopes := map[string]bool{}
	for _, s := range m.OAuthConfig.Scopes.Bot {
		gotScopes[s] = true
	}
	if len(gotScopes) != len(wantScopes) {
		t.Errorf("scope count = %d, want %d (got=%v)", len(gotScopes), len(wantScopes), m.OAuthConfig.Scopes.Bot)
	}
	for s := range wantScopes {
		if !gotScopes[s] {
			t.Errorf("missing scope: %s", s)
		}
	}
	for s := range gotScopes {
		if !wantScopes[s] {
			t.Errorf("unexpected scope: %s", s)
		}
	}

	wantEvents := map[string]bool{
		"app_mention":      true,
		"message.im":       true,
		"reaction_added":   true,
		"reaction_removed": true,
		"message.channels": true,
		"message.groups":   true,
	}
	gotEvents := map[string]bool{}
	for _, e := range m.Settings.EventSubscriptions.BotEvents {
		gotEvents[e] = true
	}
	if len(gotEvents) != len(wantEvents) {
		t.Errorf("event count = %d, want %d (got=%v)", len(gotEvents), len(wantEvents), m.Settings.EventSubscriptions.BotEvents)
	}
	for e := range wantEvents {
		if !gotEvents[e] {
			t.Errorf("missing event subscription: %s", e)
		}
	}
}

func TestGenerateManifest_DescriptionDefault(t *testing.T) {
	m := GenerateManifest("my-bot", "", false)
	if m.DisplayInfo.Description != "Slackline bot identity for AI agents" {
		t.Errorf("default description = %q", m.DisplayInfo.Description)
	}
}

func TestGenerateManifest_DescriptionOverride(t *testing.T) {
	m := GenerateManifest("my-bot", "Custom desc", false)
	if m.DisplayInfo.Description != "Custom desc" {
		t.Errorf("description = %q, want 'Custom desc'", m.DisplayInfo.Description)
	}
}

func TestGenerateManifest_AlwaysOnlineDefault(t *testing.T) {
	m := GenerateManifest("my-bot", "", false)
	if m.Features.BotUser.AlwaysOnline {
		t.Error("default always_online should be false")
	}
}

func TestGenerateManifest_AlwaysOnlineOverride(t *testing.T) {
	m := GenerateManifest("my-bot", "", true)
	if !m.Features.BotUser.AlwaysOnline {
		t.Error("always_online should be true when override is true")
	}
}
```

Also update the call sites in the existing tests in this file from `GenerateManifest("test-bot")` → `GenerateManifest("test-bot", "", false)`. Same for `GenerateManifest("test", ...)` and `GenerateManifest("my-bot", ...)`.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./provision/ -v -run TestGenerateManifest
```

Expected: build failure ("too many arguments in call to GenerateManifest" or similar) on the existing tests, plus the new tests can't run.

- [ ] **Step 3: Update `provision/manifest.go` to the new signature and add scopes/events**

Replace the body of `GenerateManifest` in `provision/manifest.go`:

```go
// GenerateManifest creates a Slack app manifest with the required scopes and
// event subscriptions for a slackline bot identity.
//
// description is optional — when empty, a default is applied.
// alwaysOnline controls Bot User's always_online setting.
func GenerateManifest(appName, description string, alwaysOnline bool) *Manifest {
	if description == "" {
		description = "Slackline bot identity for AI agents"
	}
	return &Manifest{
		DisplayInfo: DisplayInfo{
			Name:        appName,
			Description: description,
		},
		Features: Features{
			BotUser: BotUser{
				DisplayName:  appName,
				AlwaysOnline: alwaysOnline,
			},
		},
		Settings: Settings{
			SocketModeEnabled: true,
			EventSubscriptions: EventSubscriptions{
				BotEvents: []string{
					"app_mention",
					"message.im",
					"message.channels",
					"message.groups",
					"reaction_added",
					"reaction_removed",
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
					"reactions:write",
					"users:read",
					"files:read",
					"files:write",
				},
			},
		},
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./provision/ -v
```

Expected: all PASS, including the new golden test and parameter tests.

- [ ] **Step 5: Build to confirm nothing else broke (callers of GenerateManifest)**

```bash
go build ./...
```

Expected: build error in `cmd/create.go:105` because the existing call `provision.GenerateManifest(createName)` no longer compiles. **That is fine for now** — Task 5 will rewrite `cmd/create.go`. Temporarily fix the build by editing `cmd/create.go:105` to pass the new args:

```go
	manifest := provision.GenerateManifest(createName, "", false)
```

- [ ] **Step 6: Build + full test run**

```bash
go build ./... && go test ./...
```

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add provision/manifest.go provision/manifest_test.go cmd/create.go
git commit -m "feat(provision): parameterize manifest, add reactions:write/files:* scopes and reaction_removed/message.channels/message.groups events

Adds golden-file regression test that pins the exact scope and event set."
```

---

## Task 2: Extend `slack.SlackAPI` interface + `fakeSlackAPI`

**Why next:** Adding methods to the interface unlocks all command-level work in subsequent tasks. The fake updates keep the existing test suite green.

The real-client side gets these "for free" since `*goslack.Client` already implements them — except `CompleteUploadExternal` (handled in Task 3).

**Files:**
- Modify: `slack/api.go`
- Modify: `slack/client.go` (add a wrapper type so we can hang `CompleteUploadExternal` on it later)
- Modify: `cmd/helpers_test.go` (extend `fakeSlackAPI`)

### Steps

- [ ] **Step 1: Write failing compile-time interface check**

Edit `cmd/helpers_test.go`. The existing line at the bottom of the `fakeSlackAPI` definition reads:

```go
var _ slackpkg.SlackAPI = (*fakeSlackAPI)(nil)
```

This will become a compile-time failure once we extend the interface. That IS the test for this task — Go's type system enforces it. We don't need a runtime test for the bare interface.

- [ ] **Step 2: Extend the interface**

Replace the contents of `slack/api.go`:

```go
package slack

import (
	"io"

	goslack "github.com/slack-go/slack"
)

// SlackAPI is the subset of slack-go methods used by slackline.
// All command code depends on this interface, never on *slack.Client directly.
type SlackAPI interface {
	AuthTest() (response *goslack.AuthTestResponse, err error)
	PostMessage(channelID string, options ...goslack.MsgOption) (string, string, error)
	GetConversationHistory(params *goslack.GetConversationHistoryParameters) (*goslack.GetConversationHistoryResponse, error)
	GetConversationReplies(params *goslack.GetConversationRepliesParameters) ([]goslack.Message, bool, string, error)
	GetConversations(params *goslack.GetConversationsParameters) ([]goslack.Channel, string, error)

	// Reactions (Task 6).
	AddReaction(name string, item goslack.ItemRef) error
	RemoveReaction(name string, item goslack.ItemRef) error

	// Files (Tasks 8 + 9).
	UploadFileV2(params goslack.UploadFileV2Parameters) (*goslack.FileSummary, error)
	GetFileInfo(fileID string, count, page int) (*goslack.File, []goslack.Comment, *goslack.Paging, error)
	GetFile(downloadURL string, writer io.Writer) error

	// CompleteUploadExternal batches N files into a single Slack message.
	// Implemented in slack/files.go (Task 3).
	CompleteUploadExternal(channelID, threadTS, initialComment string, files []FileUpload) ([]goslack.FileSummary, error)
}

// FileUpload describes a single local file destined for a batched multi-file upload.
type FileUpload struct {
	Path  string
	Title string // optional; defaults to filename
}
```

- [ ] **Step 3: Run `go build ./...` to see what breaks**

```bash
go build ./...
```

Expected:
- `cmd/helpers_test.go`: the `var _ slackpkg.SlackAPI = (*fakeSlackAPI)(nil)` assertion fails — fakeSlackAPI doesn't implement the new methods.
- `slack/client.go`: returns `*goslack.Client` directly; that already implements everything except `CompleteUploadExternal`.

- [ ] **Step 4: Add a wrapper type in `slack/client.go`**

Replace the contents of `slack/client.go`:

```go
package slack

import (
	goslack "github.com/slack-go/slack"
)

// realClient embeds *goslack.Client so that all goslack methods promoted
// to it are inherited automatically. CompleteUploadExternal is added in
// slack/files.go on this same type.
type realClient struct {
	*goslack.Client
}

// NewClient returns a SlackAPI backed by a real slack-go client.
func NewClient(botToken string) SlackAPI {
	return &realClient{Client: goslack.New(botToken)}
}
```

(The `CompleteUploadExternal` method on `realClient` will be added by Task 3 in `slack/files.go`. For now, this file alone won't compile — that's expected.)

- [ ] **Step 5: Add stub methods to `fakeSlackAPI`**

Append to `cmd/helpers_test.go` (just before the `var _ slackpkg.SlackAPI = (*fakeSlackAPI)(nil)` line):

```go
// --- New interface methods (stubs by default, individual tests override). ---

// addReactionErr/removeReactionErr returned by the fake's reaction methods.
// capturedReactions records each call.
type capturedReaction struct {
	Name string
	Item goslack.ItemRef
}

func (f *fakeSlackAPI) AddReaction(name string, item goslack.ItemRef) error {
	f.reactionsAdded = append(f.reactionsAdded, capturedReaction{Name: name, Item: item})
	return f.addReactionErr
}

func (f *fakeSlackAPI) RemoveReaction(name string, item goslack.ItemRef) error {
	f.reactionsRemoved = append(f.reactionsRemoved, capturedReaction{Name: name, Item: item})
	return f.removeReactionErr
}

func (f *fakeSlackAPI) UploadFileV2(params goslack.UploadFileV2Parameters) (*goslack.FileSummary, error) {
	f.uploadV2Calls = append(f.uploadV2Calls, params)
	if f.uploadV2Err != nil {
		return nil, f.uploadV2Err
	}
	return f.uploadV2Resp, nil
}

func (f *fakeSlackAPI) GetFileInfo(fileID string, count, page int) (*goslack.File, []goslack.Comment, *goslack.Paging, error) {
	f.lastFileInfoID = fileID
	if f.fileInfoErr != nil {
		return nil, nil, nil, f.fileInfoErr
	}
	return f.fileInfo, nil, nil, nil
}

func (f *fakeSlackAPI) GetFile(downloadURL string, writer io.Writer) error {
	f.lastDownloadURL = downloadURL
	if f.getFileErr != nil {
		return f.getFileErr
	}
	if f.getFileBytes != nil {
		_, _ = writer.Write(f.getFileBytes)
	}
	return nil
}

func (f *fakeSlackAPI) CompleteUploadExternal(channelID, threadTS, initialComment string, files []slackpkg.FileUpload) ([]goslack.FileSummary, error) {
	f.lastCompleteUploadCall = completeUploadCall{
		ChannelID:      channelID,
		ThreadTS:       threadTS,
		InitialComment: initialComment,
		Files:          files,
	}
	if f.completeUploadErr != nil {
		return nil, f.completeUploadErr
	}
	return f.completeUploadResp, nil
}

type completeUploadCall struct {
	ChannelID      string
	ThreadTS       string
	InitialComment string
	Files          []slackpkg.FileUpload
}
```

Add the corresponding fields to the `fakeSlackAPI` struct definition near the top of the file (insert before the `// capturedHistoryParams records...` comment block, or at the end of the struct):

```go
	// reactions
	reactionsAdded     []capturedReaction
	reactionsRemoved   []capturedReaction
	addReactionErr     error
	removeReactionErr  error

	// files (UploadFileV2 single-file)
	uploadV2Calls []goslack.UploadFileV2Parameters
	uploadV2Resp  *goslack.FileSummary
	uploadV2Err   error

	// files (download)
	fileInfo        *goslack.File
	fileInfoErr     error
	lastFileInfoID  string
	getFileBytes    []byte
	getFileErr      error
	lastDownloadURL string

	// files (multi-file batch)
	lastCompleteUploadCall completeUploadCall
	completeUploadResp     []goslack.FileSummary
	completeUploadErr      error
```

Add `"io"` to the imports of `cmd/helpers_test.go`.

- [ ] **Step 6: Add a placeholder `CompleteUploadExternal` to make the build pass until Task 3**

Create a tiny stub in a new file `slack/files.go`:

```go
package slack

import (
	"errors"

	goslack "github.com/slack-go/slack"
)

// CompleteUploadExternal is the multi-file batched upload wrapper.
// Implementation lives in this file; this is the placeholder body that
// Task 3 fills in.
func (c *realClient) CompleteUploadExternal(channelID, threadTS, initialComment string, files []FileUpload) ([]goslack.FileSummary, error) {
	return nil, errors.New("not implemented")
}
```

- [ ] **Step 7: Build + run tests to confirm everything compiles and passes**

```bash
go build ./... && go test ./...
```

Expected: all green. The placeholder `CompleteUploadExternal` returns an error but no test actually calls it yet, so the suite stays green.

- [ ] **Step 8: Commit**

```bash
git add slack/api.go slack/client.go slack/files.go cmd/helpers_test.go
git commit -m "feat(slack): extend SlackAPI interface with reactions, files, and multi-file upload

Adds AddReaction/RemoveReaction, UploadFileV2/GetFileInfo/GetFile, and
CompleteUploadExternal (stubbed; implemented in next commit). fakeSlackAPI
extended with stub methods and call-recording fields."
```

---

## Task 3: Implement `slack.CompleteUploadExternal`

**Why next:** Required by Task 9 (`slackline send --attach`). Self-contained: pure HTTP wrapper around two Slack endpoints.

The Slack flow for batched multi-file shares:
1. `files.getUploadURLExternal` (POST) per file — returns `{upload_url, file_id}`.
2. PUT the file bytes to that `upload_url` (no auth header — the URL is signed).
3. `files.completeUploadExternal` (POST) once with `files: [{id, title}, …]`, `channel_id`, `initial_comment`, `thread_ts` — atomically shares them as one message.

`slack-go`'s `UploadFileV2` does this for ONE file at a time. We need the multi-file batch version, which `slack-go` doesn't expose, so we implement it directly.

**Files:**
- Modify: `slack/files.go`
- Create: `slack/files_test.go`

### Steps

- [ ] **Step 1: Write the failing test (`slack/files_test.go`)**

```go
package slack

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
)

func TestCompleteUploadExternal_TwoFiles(t *testing.T) {
	// Spy server records every call.
	var (
		getURLCalls    int
		putCalls       int
		completeCalls  int
		completeParams url.Values
		uploadedBytes  = map[string]string{}
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/files.getUploadURLExternal", func(w http.ResponseWriter, r *http.Request) {
		getURLCalls++
		_ = r.ParseForm()
		filename := r.Form.Get("filename")
		fileID := "F_" + strings.ReplaceAll(filename, ".", "_")
		uploadURL := "http://" + r.Host + "/upload?id=" + fileID
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"upload_url":"`+uploadURL+`","file_id":"`+fileID+`"}`)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		putCalls++
		body, _ := io.ReadAll(r.Body)
		uploadedBytes[r.URL.Query().Get("id")] = string(body)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/files.completeUploadExternal", func(w http.ResponseWriter, r *http.Request) {
		completeCalls++
		_ = r.ParseForm()
		completeParams = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"files":[{"id":"F_a_txt","title":"a.txt"},{"id":"F_b_txt","title":"b.txt"}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Two real local files.
	tmp := t.TempDir()
	pathA := filepath.Join(tmp, "a.txt")
	pathB := filepath.Join(tmp, "b.txt")
	if err := os.WriteFile(pathA, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte("bravo"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := &realClient{Client: goslack.New("xoxb-test", goslack.OptionAPIURL(srv.URL+"/api/"))}

	files := []FileUpload{{Path: pathA, Title: "a.txt"}, {Path: pathB, Title: "b.txt"}}
	got, err := c.CompleteUploadExternal("C123", "100.000", "hello", files)
	if err != nil {
		t.Fatalf("CompleteUploadExternal returned error: %v", err)
	}
	if getURLCalls != 2 {
		t.Errorf("getUploadURLExternal calls = %d, want 2", getURLCalls)
	}
	if putCalls != 2 {
		t.Errorf("PUT upload calls = %d, want 2", putCalls)
	}
	if completeCalls != 1 {
		t.Errorf("completeUploadExternal calls = %d, want 1", completeCalls)
	}
	if completeParams.Get("channel_id") != "C123" {
		t.Errorf("channel_id = %q", completeParams.Get("channel_id"))
	}
	if completeParams.Get("thread_ts") != "100.000" {
		t.Errorf("thread_ts = %q", completeParams.Get("thread_ts"))
	}
	if completeParams.Get("initial_comment") != "hello" {
		t.Errorf("initial_comment = %q", completeParams.Get("initial_comment"))
	}
	// files param is JSON-encoded.
	var filesArg []map[string]string
	if err := json.Unmarshal([]byte(completeParams.Get("files")), &filesArg); err != nil {
		t.Fatalf("files param not valid JSON: %v", err)
	}
	if len(filesArg) != 2 {
		t.Fatalf("files param length = %d, want 2", len(filesArg))
	}
	if uploadedBytes["F_a_txt"] != "alpha" {
		t.Errorf("uploaded bytes for a.txt = %q", uploadedBytes["F_a_txt"])
	}
	if uploadedBytes["F_b_txt"] != "bravo" {
		t.Errorf("uploaded bytes for b.txt = %q", uploadedBytes["F_b_txt"])
	}
	if len(got) != 2 {
		t.Errorf("returned summaries = %d, want 2", len(got))
	}
}

func TestCompleteUploadExternal_NoThread(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/files.getUploadURLExternal", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"upload_url":"http://`+r.Host+`/u","file_id":"F1"}`)
	})
	mux.HandleFunc("/u", func(w http.ResponseWriter, r *http.Request) {})
	var thread string
	mux.HandleFunc("/api/files.completeUploadExternal", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		thread = r.Form.Get("thread_ts")
		_, _ = io.WriteString(w, `{"ok":true,"files":[{"id":"F1"}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "x")
	_ = os.WriteFile(p, []byte("x"), 0o600)

	c := &realClient{Client: goslack.New("xoxb-test", goslack.OptionAPIURL(srv.URL+"/api/"))}
	_, err := c.CompleteUploadExternal("C1", "", "", []FileUpload{{Path: p}})
	if err != nil {
		t.Fatal(err)
	}
	// When thread_ts is empty it should be omitted from the form (or empty string).
	if thread != "" {
		t.Errorf("thread_ts = %q, want empty", thread)
	}
}

func TestCompleteUploadExternal_GetURLError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/files.getUploadURLExternal", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":false,"error":"file_too_big"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "x")
	_ = os.WriteFile(p, []byte("x"), 0o600)

	c := &realClient{Client: goslack.New("xoxb-test", goslack.OptionAPIURL(srv.URL+"/api/"))}
	_, err := c.CompleteUploadExternal("C1", "", "", []FileUpload{{Path: p}})
	if err == nil {
		t.Fatal("expected error from getUploadURLExternal failure")
	}
	if !strings.Contains(err.Error(), "file_too_big") {
		t.Errorf("error should mention file_too_big, got: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./slack/ -v -run TestCompleteUploadExternal
```

Expected: tests fail with the placeholder's `"not implemented"` error.

- [ ] **Step 3: Implement `CompleteUploadExternal`**

Replace `slack/files.go`:

```go
package slack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	goslack "github.com/slack-go/slack"
)

// CompleteUploadExternal uploads N local files via Slack's external-upload
// flow and shares them as a single message in channelID. If threadTS is
// non-empty, the message is posted as a thread reply. initialComment, when
// non-empty, becomes the message body.
func (c *realClient) CompleteUploadExternal(channelID, threadTS, initialComment string, files []FileUpload) ([]goslack.FileSummary, error) {
	apiBase, token := slackAPIInfo(c.Client)

	type uploadResult struct {
		fileID string
		title  string
	}
	results := make([]uploadResult, 0, len(files))

	for _, f := range files {
		size, err := fileSize(f.Path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", f.Path, err)
		}
		urlInfo, err := getUploadURLExternal(apiBase, token, filepath.Base(f.Path), size)
		if err != nil {
			return nil, err
		}
		if err := putFileBytes(urlInfo.UploadURL, f.Path); err != nil {
			return nil, err
		}
		title := f.Title
		if title == "" {
			title = filepath.Base(f.Path)
		}
		results = append(results, uploadResult{fileID: urlInfo.FileID, title: title})
	}

	type fileItem struct {
		ID    string `json:"id"`
		Title string `json:"title,omitempty"`
	}
	items := make([]fileItem, len(results))
	for i, r := range results {
		items[i] = fileItem{ID: r.fileID, Title: r.title}
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal files list: %w", err)
	}

	form := url.Values{
		"token":      {token},
		"files":      {string(itemsJSON)},
		"channel_id": {channelID},
	}
	if threadTS != "" {
		form.Set("thread_ts", threadTS)
	}
	if initialComment != "" {
		form.Set("initial_comment", initialComment)
	}

	resp, err := http.PostForm(apiBase+"files.completeUploadExternal", form)
	if err != nil {
		return nil, fmt.Errorf("completeUploadExternal POST failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var completeResp struct {
		OK    bool                  `json:"ok"`
		Error string                `json:"error,omitempty"`
		Files []goslack.FileSummary `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completeResp); err != nil {
		return nil, fmt.Errorf("decode completeUploadExternal: %w", err)
	}
	if !completeResp.OK {
		return nil, fmt.Errorf("completeUploadExternal: %s", completeResp.Error)
	}
	return completeResp.Files, nil
}

type uploadURLInfo struct {
	UploadURL string `json:"upload_url"`
	FileID    string `json:"file_id"`
}

func getUploadURLExternal(apiBase, token, filename string, size int64) (*uploadURLInfo, error) {
	resp, err := http.PostForm(apiBase+"files.getUploadURLExternal", url.Values{
		"token":    {token},
		"filename": {filename},
		"length":   {strconv.FormatInt(size, 10)},
	})
	if err != nil {
		return nil, fmt.Errorf("getUploadURLExternal POST failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
		uploadURLInfo
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode getUploadURLExternal: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("getUploadURLExternal: %s", out.Error)
	}
	return &out.uploadURLInfo, nil
}

func putFileBytes(uploadURL, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	req, err := http.NewRequest(http.MethodPost, uploadURL, f)
	if err != nil {
		return fmt.Errorf("build upload request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload PUT failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("upload returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func fileSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}
```

We also need access to the `*goslack.Client`'s API base URL and token. slack-go doesn't expose them directly. The cleanest workaround is a small accessor we add to the embed:

Append to `slack/client.go`:

```go
// slackAPIInfo extracts the API base URL and bot token from a *goslack.Client.
// We need them for the raw HTTP calls in CompleteUploadExternal because
// slack-go doesn't expose a reusable internal HTTP helper for these endpoints.
func slackAPIInfo(c *goslack.Client) (apiBase, token string) {
	return c.Endpoint(), c.Token()
}
```

Verify the methods `Endpoint()` and `Token()` exist on `*goslack.Client` for the version pinned in `go.mod`. If they're absent in your version, fall back to wrapping with our own URL/token storage:

If `Endpoint()`/`Token()` don't exist, change `realClient` in `slack/client.go` to:

```go
type realClient struct {
	*goslack.Client
	apiBase string
	token   string
}

func NewClient(botToken string) SlackAPI {
	return &realClient{
		Client:  goslack.New(botToken),
		apiBase: "https://slack.com/api/",
		token:   botToken,
	}
}

func slackAPIInfo(c *goslack.Client) (string, string) {
	// Caller passes c.Client; the embedding type already has apiBase/token.
	// (This stub is kept only to satisfy the existing call site;
	//  CompleteUploadExternal in slack/files.go uses c.apiBase/c.token directly.)
	return "https://slack.com/api/", "" // unused
}
```

…and update the first line of `CompleteUploadExternal` to:

```go
apiBase, token := c.apiBase, c.token
```

Update the test in `slack/files_test.go` to use `&realClient{Client: ..., apiBase: srv.URL+"/api/", token: "xoxb-test"}`.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./slack/ -v
```

Expected: all PASS.

- [ ] **Step 5: Run the full test suite**

```bash
go build ./... && go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add slack/files.go slack/files_test.go slack/client.go
git commit -m "feat(slack): implement CompleteUploadExternal for batched multi-file shares

Wraps files.getUploadURLExternal + per-file PUT + files.completeUploadExternal
into a single call that shares N files as one Slack message with optional
initial_comment and thread_ts."
```

---

## Task 4: New `slackline provision` command

**Why next:** Splits the monolithic `slackline create` into a scriptable primitive. Implements both `provision <name>` (per-bot, JSON output, no interaction) and `provision bootstrap` (one-time per machine, env-var-or-stdin to seed `provision.json`).

**Files:**
- Create: `cmd/provision.go`
- Create: `cmd/provision_test.go`

### Steps

- [ ] **Step 1: Write failing tests (`cmd/provision_test.go`)**

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/config"
)

func writeProvisionFile(t *testing.T, dir string, cfg *config.ProvisionConfig) string {
	t.Helper()
	path := filepath.Join(dir, "provision.json")
	if err := config.SaveProvision(cfg, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProvision_NameSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tooling.tokens.rotate", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"token":"xoxe.new","refresh_token":"xoxe-newref"}`)
	})
	mux.HandleFunc("/api/apps.manifest.create", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"ok":true,
			"app_id":"A123",
			"team_id":"T456",
			"team_domain":"acme",
			"oauth_authorize_url":"https://slack.com/oauth/v2/authorize?client_id=x"
		}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	provPath := writeProvisionFile(t, tmp, &config.ProvisionConfig{
		ConfigToken:  "xoxe.old",
		RefreshToken: "xoxe-oldref",
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runProvisionWithDeps("my-bot", "", false, provPath, srv.URL+"/api/", stdout, stderr)
	if err != nil {
		t.Fatalf("runProvisionWithDeps returned error: %v\nstderr: %s", err, stderr.String())
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout.String())
	}

	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
	if got["app_id"] != "A123" {
		t.Errorf("app_id = %v", got["app_id"])
	}
	if got["team_id"] != "T456" {
		t.Errorf("team_id = %v", got["team_id"])
	}
	if got["team_domain"] != "acme" {
		t.Errorf("team_domain = %v", got["team_domain"])
	}
	if !strings.Contains(got["install_url"].(string), "A123/install-on-team") {
		t.Errorf("install_url = %v", got["install_url"])
	}
	if !strings.Contains(got["oauth_page_url"].(string), "A123/oauth") {
		t.Errorf("oauth_page_url = %v", got["oauth_page_url"])
	}
	if !strings.Contains(got["general_page_url"].(string), "A123/general") {
		t.Errorf("general_page_url = %v", got["general_page_url"])
	}
	if got["oauth_authorize_url"] != "https://slack.com/oauth/v2/authorize?client_id=x" {
		t.Errorf("oauth_authorize_url = %v", got["oauth_authorize_url"])
	}

	// provision.json should now contain the rotated tokens.
	rotated, err := config.LoadProvision(provPath)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ConfigToken != "xoxe.new" {
		t.Errorf("rotated ConfigToken = %q, want xoxe.new", rotated.ConfigToken)
	}
	if rotated.RefreshToken != "xoxe-newref" {
		t.Errorf("rotated RefreshToken = %q, want xoxe-newref", rotated.RefreshToken)
	}
}

func TestProvision_MissingProvisionFile(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "does_not_exist.json")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runProvisionWithDeps("my-bot", "", false, missing, "https://example.invalid/api/", stdout, stderr)
	if err == nil {
		t.Fatal("expected error when provision.json missing")
	}
	if !strings.Contains(err.Error(), "bootstrap") {
		t.Errorf("error should mention 'bootstrap', got: %v", err)
	}
}

func TestProvisionBootstrap_FromEnv(t *testing.T) {
	tmp := t.TempDir()
	provPath := filepath.Join(tmp, "provision.json")

	t.Setenv("SLACKLINE_CONFIG_TOKEN", "xoxe.cfg")
	t.Setenv("SLACKLINE_REFRESH_TOKEN", "xoxe-ref")

	stderr := &bytes.Buffer{}
	err := runProvisionBootstrapWithDeps(provPath, &bytes.Buffer{}, stderr)
	if err != nil {
		t.Fatalf("bootstrap failed: %v\nstderr: %s", err, stderr.String())
	}

	got, err := config.LoadProvision(provPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigToken != "xoxe.cfg" || got.RefreshToken != "xoxe-ref" {
		t.Errorf("provision.json = %+v", got)
	}
}

func TestProvisionBootstrap_OnlyOneEnvSet(t *testing.T) {
	tmp := t.TempDir()
	provPath := filepath.Join(tmp, "provision.json")

	t.Setenv("SLACKLINE_CONFIG_TOKEN", "xoxe.cfg")
	t.Setenv("SLACKLINE_REFRESH_TOKEN", "")

	stderr := &bytes.Buffer{}
	err := runProvisionBootstrapWithDeps(provPath, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("expected error when only one env var set")
	}
	if _, statErr := os.Stat(provPath); !os.IsNotExist(statErr) {
		t.Error("provision.json should not be written on validation failure")
	}
}

func TestProvisionBootstrap_FromStdin(t *testing.T) {
	tmp := t.TempDir()
	provPath := filepath.Join(tmp, "provision.json")

	t.Setenv("SLACKLINE_CONFIG_TOKEN", "")
	t.Setenv("SLACKLINE_REFRESH_TOKEN", "")

	stdin := bytes.NewBufferString("xoxe.cfg-stdin\nxoxe-ref-stdin\n")
	stderr := &bytes.Buffer{}
	err := runProvisionBootstrapWithDeps(provPath, stdin, stderr)
	if err != nil {
		t.Fatalf("bootstrap failed: %v\nstderr: %s", err, stderr.String())
	}

	got, err := config.LoadProvision(provPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigToken != "xoxe.cfg-stdin" || got.RefreshToken != "xoxe-ref-stdin" {
		t.Errorf("provision.json = %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/ -v -run TestProvision
```

Expected: build failure ("undefined: runProvisionWithDeps", "undefined: runProvisionBootstrapWithDeps").

- [ ] **Step 3: Implement `cmd/provision.go`**

```go
package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/prime-radiant-inc/slackline/config"
	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/prime-radiant-inc/slackline/provision"
	"github.com/spf13/cobra"
)

var (
	provisionDescription string
	provisionAlwaysOn    bool
)

var provisionCmd = &cobra.Command{
	Use:   "provision NAME",
	Short: "Create a Slack app via the manifest API (machine-readable JSON output)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvisionWithDeps(args[0], provisionDescription, provisionAlwaysOn,
			config.DefaultProvisionPath(), "https://slack.com/api/",
			cmd.OutOrStdout(), cmd.OutOrStderr())
	},
}

var provisionBootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Seed provision.json from env vars or stdin (one-time per machine)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvisionBootstrapWithDeps(config.DefaultProvisionPath(), os.Stdin, cmd.OutOrStderr())
	},
}

func init() {
	provisionCmd.Flags().StringVar(&provisionDescription, "description", "", "override the default app description")
	provisionCmd.Flags().BoolVar(&provisionAlwaysOn, "always-online", false, "set bot_user.always_online on the manifest")
	provisionCmd.AddCommand(provisionBootstrapCmd)
	rootCmd.AddCommand(provisionCmd)
}

// runProvisionWithDeps is the testable core of `slackline provision NAME`.
func runProvisionWithDeps(name, description string, alwaysOnline bool, provPath, apiBase string, stdout, stderr io.Writer) error {
	prov, err := config.LoadProvision(provPath)
	if err != nil {
		return &errs.SlackError{
			Code:   errs.Config,
			Err:    "no_provision_config",
			Detail: "No provision.json found. Run `slackline provision bootstrap` first to seed config and refresh tokens.",
		}
	}

	newToken, newRefresh, err := rotateConfigToken(apiBase, prov.RefreshToken)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "token_rotation_failed", Detail: err.Error()}
	}
	prov.ConfigToken = newToken
	prov.RefreshToken = newRefresh
	if err := config.SaveProvision(prov, provPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
	}

	manifest := provision.GenerateManifest(name, description, alwaysOnline)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return &errs.SlackError{Code: errs.Usage, Err: "marshal_manifest", Detail: err.Error()}
	}

	createResp, err := postManifestCreate(apiBase, prov.ConfigToken, string(manifestJSON))
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "create_app_failed", Detail: err.Error()}
	}

	out := struct {
		OK                bool   `json:"ok"`
		AppID             string `json:"app_id"`
		TeamID            string `json:"team_id"`
		TeamDomain        string `json:"team_domain"`
		InstallURL        string `json:"install_url"`
		OAuthAuthorizeURL string `json:"oauth_authorize_url"`
		OAuthPageURL      string `json:"oauth_page_url"`
		GeneralPageURL    string `json:"general_page_url"`
	}{
		OK:                true,
		AppID:             createResp.AppID,
		TeamID:            createResp.TeamID,
		TeamDomain:        createResp.TeamDomain,
		InstallURL:        fmt.Sprintf("https://api.slack.com/apps/%s/install-on-team", createResp.AppID),
		OAuthAuthorizeURL: createResp.OAuthAuthorizeURL,
		OAuthPageURL:      fmt.Sprintf("https://api.slack.com/apps/%s/oauth", createResp.AppID),
		GeneralPageURL:    fmt.Sprintf("https://api.slack.com/apps/%s/general", createResp.AppID),
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// runProvisionBootstrapWithDeps is the testable core of `slackline provision bootstrap`.
func runProvisionBootstrapWithDeps(provPath string, stdin io.Reader, stderr io.Writer) error {
	cfgTok := os.Getenv("SLACKLINE_CONFIG_TOKEN")
	refTok := os.Getenv("SLACKLINE_REFRESH_TOKEN")

	switch {
	case cfgTok != "" && refTok != "":
		// Both env vars set — non-interactive path.
	case cfgTok != "" || refTok != "":
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "missing_token",
			Detail: "Set both SLACKLINE_CONFIG_TOKEN and SLACKLINE_REFRESH_TOKEN, or unset both to be prompted via stdin.",
		}
	default:
		// Interactive path — prompt via stdin.
		_, _ = fmt.Fprintln(stderr, "No config token found. Generate one at https://api.slack.com/apps")
		_, _ = fmt.Fprintln(stderr, "  → scroll to \"Your App Configuration Tokens\" → Generate Token.")
		_, _ = fmt.Fprintln(stderr, "")
		reader := bufio.NewReader(stdin)
		_, _ = fmt.Fprint(stderr, "Paste your config token: ")
		line, _ := reader.ReadString('\n')
		cfgTok = strings.TrimSpace(line)
		if cfgTok == "" {
			return &errs.SlackError{Code: errs.Usage, Err: "empty_config_token", Detail: "config token cannot be empty"}
		}
		_, _ = fmt.Fprint(stderr, "Paste your refresh token: ")
		line, _ = reader.ReadString('\n')
		refTok = strings.TrimSpace(line)
		if refTok == "" {
			return &errs.SlackError{Code: errs.Usage, Err: "empty_refresh_token", Detail: "refresh token cannot be empty"}
		}
	}

	prov := &config.ProvisionConfig{ConfigToken: cfgTok, RefreshToken: refTok}
	if err := config.SaveProvision(prov, provPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
	}
	_, _ = fmt.Fprintln(stderr, "✓ provision.json written.")
	return nil
}

// rotateConfigToken calls tooling.tokens.rotate. Mirrors provision.RotateConfigToken
// but with the apiBase override needed for tests.
func rotateConfigToken(apiBase, refreshToken string) (string, string, error) {
	resp, err := http.PostForm(apiBase+"tooling.tokens.rotate", url.Values{
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return "", "", fmt.Errorf("rotate request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		OK           bool   `json:"ok"`
		Error        string `json:"error,omitempty"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode rotate: %w", err)
	}
	if !out.OK {
		return "", "", fmt.Errorf("rotate: %s", out.Error)
	}
	return out.Token, out.RefreshToken, nil
}

type manifestCreateResponse struct {
	OK                bool   `json:"ok"`
	Error             string `json:"error,omitempty"`
	AppID             string `json:"app_id"`
	TeamID            string `json:"team_id"`
	TeamDomain        string `json:"team_domain"`
	OAuthAuthorizeURL string `json:"oauth_authorize_url"`
}

func postManifestCreate(apiBase, configToken, manifestJSON string) (*manifestCreateResponse, error) {
	resp, err := http.PostForm(apiBase+"apps.manifest.create", url.Values{
		"token":    {configToken},
		"manifest": {manifestJSON},
	})
	if err != nil {
		return nil, fmt.Errorf("manifest create POST failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out manifestCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode manifest.create: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("apps.manifest.create: %s", out.Error)
	}
	return &out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/ -v -run TestProvision
```

Expected: all PASS.

- [ ] **Step 5: Run full suite**

```bash
go build ./... && go test ./...
```

Expected: all green. Note: `cmd/create.go` still exists and compiles (we patched its `GenerateManifest` call site in Task 1). It will be replaced in Task 5.

- [ ] **Step 6: Commit**

```bash
git add cmd/provision.go cmd/provision_test.go
git commit -m "feat(cmd): add slackline provision command (split from create)

Adds two subcommands:
- provision <name>: API-only, machine-readable JSON output
- provision bootstrap: env-var or stdin to seed provision.json

This is the scriptable primitive that lets agents with browser tools deploy
bots end-to-end without per-bot interactive prompts."
```

---

## Task 5: Replace `slackline create` with a migration stub

**Why next:** Now that `provision` works, `create` should fail loudly with a clear migration message rather than silently doing the right thing under the wrong name.

**Files:**
- Modify: `cmd/create.go` (rewrite)
- Modify: `cmd/helpers_test.go` (no change expected, but worth checking for create-specific test helpers)

### Steps

- [ ] **Step 1: Write failing test (append to `cmd/provision_test.go`)**

```go
func TestCreate_RemovedReturnsMigrationError(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runCreateRemoved(stdout, stderr)
	if err == nil {
		t.Fatal("expected error from removed `slackline create`")
	}
	if !strings.Contains(err.Error(), "provision") {
		t.Errorf("error should mention 'provision' to guide migration, got: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./cmd/ -v -run TestCreate_RemovedReturnsMigrationError
```

Expected: build failure ("undefined: runCreateRemoved").

- [ ] **Step 3: Replace `cmd/create.go` with a stub**

Overwrite the entire file:

```go
package cmd

import (
	"io"

	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:    "create",
	Short:  "(removed — use `slackline provision`)",
	Long:   "The create command has been replaced by `slackline provision bootstrap` and `slackline provision <name>`.",
	Hidden: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCreateRemoved(cmd.OutOrStdout(), cmd.OutOrStderr())
	},
}

func runCreateRemoved(stdout, stderr io.Writer) error {
	return &errs.SlackError{
		Code:   errs.Usage,
		Err:    "removed",
		Detail: "slackline create has been split into 'slackline provision bootstrap' (one-time per machine) and 'slackline provision <name>' (per bot). See `slackline provision --help`.",
	}
}
```

- [ ] **Step 4: Run the test**

```bash
go test ./cmd/ -v -run TestCreate_RemovedReturnsMigrationError
```

Expected: PASS.

- [ ] **Step 5: Build + full test suite**

```bash
go build ./... && go test ./...
```

Expected: all green. The old `cmd/create.go` had several test helpers in `cmd/helpers_test.go` (look for any references to `createAppViaManifest` etc.) that may now be unused. Run:

```bash
golangci-lint run ./cmd/
```

If `golangci-lint` flags unused code, delete the dead references. (None are expected — the helpers were specific to the inline create flow; provision uses its own.)

- [ ] **Step 6: Commit**

```bash
git add cmd/create.go cmd/provision_test.go
git commit -m "feat(cmd): remove slackline create, return migration error pointing at provision

Hard break, not silent removal. Existing scripts get a discoverable migration
message rather than mysterious failure."
```

---

## Task 6: `slackline react add/remove`

**Why next:** Smallest and most self-contained of the messaging features. Exercises the new `SlackAPI` reaction methods.

**Files:**
- Create: `cmd/react.go`
- Create: `cmd/react_test.go`

### Steps

- [ ] **Step 1: Write failing tests (`cmd/react_test.go`)**

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
)

func TestReactAdd_Success(t *testing.T) {
	api := &fakeSlackAPI{}
	stdout := &bytes.Buffer{}

	err := runReactAddWithAPI(api, "C123", "100.001", "thumbsup", stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.reactionsAdded) != 1 {
		t.Fatalf("expected 1 AddReaction call, got %d", len(api.reactionsAdded))
	}
	got := api.reactionsAdded[0]
	if got.Name != "thumbsup" {
		t.Errorf("name = %q, want thumbsup", got.Name)
	}
	if got.Item.Channel != "C123" || got.Item.Timestamp != "100.001" {
		t.Errorf("item = %+v", got.Item)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out["ok"] != true {
		t.Errorf("ok = %v", out["ok"])
	}
	if out["action"] != "added" {
		t.Errorf("action = %v", out["action"])
	}
	if out["emoji"] != "thumbsup" {
		t.Errorf("emoji = %v", out["emoji"])
	}
}

func TestReactAdd_StripsColons(t *testing.T) {
	api := &fakeSlackAPI{}
	_ = runReactAddWithAPI(api, "C123", "100", ":party:", &bytes.Buffer{})
	if api.reactionsAdded[0].Name != "party" {
		t.Errorf("name = %q, want party (colons stripped)", api.reactionsAdded[0].Name)
	}
}

func TestReactAdd_AlreadyReactedIsIdempotent(t *testing.T) {
	api := &fakeSlackAPI{addReactionErr: errors.New("already_reacted")}
	stdout := &bytes.Buffer{}
	err := runReactAddWithAPI(api, "C123", "100", "thumbsup", stdout)
	if err != nil {
		t.Fatalf("expected no error for already_reacted, got: %v", err)
	}
	var out map[string]interface{}
	_ = json.Unmarshal(stdout.Bytes(), &out)
	if out["no_op"] != true {
		t.Errorf("no_op should be true; got: %v", out)
	}
	if out["ok"] != true {
		t.Errorf("ok should be true; got: %v", out)
	}
}

func TestReactAdd_OtherError(t *testing.T) {
	api := &fakeSlackAPI{addReactionErr: errors.New("channel_not_found")}
	err := runReactAddWithAPI(api, "C123", "100", "thumbsup", &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "channel_not_found") {
		t.Errorf("error should mention channel_not_found, got: %v", err)
	}
}

func TestReactRemove_Success(t *testing.T) {
	api := &fakeSlackAPI{}
	stdout := &bytes.Buffer{}
	err := runReactRemoveWithAPI(api, "C123", "100.001", "thumbsup", stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.reactionsRemoved) != 1 {
		t.Fatalf("expected 1 RemoveReaction call, got %d", len(api.reactionsRemoved))
	}
	if api.reactionsRemoved[0] != (capturedReaction{Name: "thumbsup", Item: goslack.ItemRef{Channel: "C123", Timestamp: "100.001"}}) {
		t.Errorf("captured: %+v", api.reactionsRemoved[0])
	}
	var out map[string]interface{}
	_ = json.Unmarshal(stdout.Bytes(), &out)
	if out["action"] != "removed" {
		t.Errorf("action = %v", out["action"])
	}
}

func TestReactRemove_NoReactionIsIdempotent(t *testing.T) {
	api := &fakeSlackAPI{removeReactionErr: errors.New("no_reaction")}
	stdout := &bytes.Buffer{}
	err := runReactRemoveWithAPI(api, "C123", "100", "thumbsup", stdout)
	if err != nil {
		t.Fatalf("expected no error for no_reaction, got: %v", err)
	}
	var out map[string]interface{}
	_ = json.Unmarshal(stdout.Bytes(), &out)
	if out["no_op"] != true {
		t.Errorf("no_op should be true")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/ -v -run TestReact
```

Expected: build failure ("undefined: runReactAddWithAPI", etc.).

- [ ] **Step 3: Implement `cmd/react.go`**

```go
package cmd

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	reactChannel string
	reactTS      string
	reactEmoji   string
)

var reactCmd = &cobra.Command{
	Use:   "react",
	Short: "Add or remove emoji reactions on a message",
}

var reactAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a reaction to a message",
	RunE: func(cmd *cobra.Command, args []string) error {
		api, err := loadReactAPI()
		if err != nil {
			return err
		}
		channelID, err := resolveChannel(api, reactChannel)
		if err != nil {
			return err
		}
		return runReactAddWithAPI(api, channelID, reactTS, reactEmoji, cmd.OutOrStdout())
	},
}

var reactRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a reaction from a message",
	RunE: func(cmd *cobra.Command, args []string) error {
		api, err := loadReactAPI()
		if err != nil {
			return err
		}
		channelID, err := resolveChannel(api, reactChannel)
		if err != nil {
			return err
		}
		return runReactRemoveWithAPI(api, channelID, reactTS, reactEmoji, cmd.OutOrStdout())
	},
}

func init() {
	for _, sub := range []*cobra.Command{reactAddCmd, reactRemoveCmd} {
		sub.Flags().StringVar(&reactChannel, "channel", "", "channel name (#ops), ID, or URL (required)")
		sub.Flags().StringVar(&reactTS, "ts", "", "message timestamp (required)")
		sub.Flags().StringVar(&reactEmoji, "emoji", "", "emoji name without colons (required)")
		_ = sub.MarkFlagRequired("channel")
		_ = sub.MarkFlagRequired("ts")
		_ = sub.MarkFlagRequired("emoji")
		reactCmd.AddCommand(sub)
	}
	rootCmd.AddCommand(reactCmd)
}

// loadReactAPI loads config and returns a SlackAPI, factored out so both subcommands share it.
func loadReactAPI() (slackpkg.SlackAPI, error) {
	cfg, _, err := loadConfig()
	if err != nil {
		return nil, &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return nil, &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "Run 'slackline init' first."}
	}
	return slackpkg.NewClient(cfg.Bot.BotToken), nil
}

// resolveChannel converts a flag value (#name | C123 | URL) to a channel ID.
func resolveChannel(api slackpkg.SlackAPI, flagValue string) (string, error) {
	resolver := slackpkg.NewResolver(api)
	id, err := resolver.Resolve(flagValue)
	if err != nil {
		return "", &errs.SlackError{Code: errs.SlackAPI, Err: "channel_not_found", Detail: err.Error()}
	}
	return id, nil
}

// stripEmojiColons returns "thumbsup" for inputs like "thumbsup", ":thumbsup:", " :thumbsup: ".
func stripEmojiColons(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, ":")
	s = strings.TrimSuffix(s, ":")
	return s
}

// runReactAddWithAPI is the testable core of `react add`.
func runReactAddWithAPI(api slackpkg.SlackAPI, channelID, ts, emoji string, stdout io.Writer) error {
	emoji = stripEmojiColons(emoji)
	err := api.AddReaction(emoji, goslack.ItemRef{Channel: channelID, Timestamp: ts})
	noOp := false
	if err != nil {
		if strings.Contains(err.Error(), "already_reacted") {
			noOp = true
		} else if isAuthError(err) {
			return errs.AuthError(err.Error())
		} else {
			return &errs.SlackError{Code: errs.SlackAPI, Err: "react_add_failed", Detail: err.Error()}
		}
	}
	return writeReactJSON(stdout, channelID, ts, emoji, "added", noOp)
}

// runReactRemoveWithAPI is the testable core of `react remove`.
func runReactRemoveWithAPI(api slackpkg.SlackAPI, channelID, ts, emoji string, stdout io.Writer) error {
	emoji = stripEmojiColons(emoji)
	err := api.RemoveReaction(emoji, goslack.ItemRef{Channel: channelID, Timestamp: ts})
	noOp := false
	if err != nil {
		if strings.Contains(err.Error(), "no_reaction") {
			noOp = true
		} else if isAuthError(err) {
			return errs.AuthError(err.Error())
		} else {
			return &errs.SlackError{Code: errs.SlackAPI, Err: "react_remove_failed", Detail: err.Error()}
		}
	}
	return writeReactJSON(stdout, channelID, ts, emoji, "removed", noOp)
}

func writeReactJSON(stdout io.Writer, channelID, ts, emoji, action string, noOp bool) error {
	out := struct {
		OK      bool   `json:"ok"`
		NoOp    bool   `json:"no_op,omitempty"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
		Emoji   string `json:"emoji"`
		Action  string `json:"action"`
	}{OK: true, NoOp: noOp, Channel: channelID, TS: ts, Emoji: emoji, Action: action}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/ -v -run TestReact
```

Expected: all PASS.

- [ ] **Step 5: Build + full suite**

```bash
go build ./... && go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add cmd/react.go cmd/react_test.go
git commit -m "feat(cmd): add slackline react add/remove with idempotent error handling

already_reacted (on add) and no_reaction (on remove) treated as success
(no_op:true) so agentic callers retrying after partial failure don't have to
special-case 'I already did this'."
```

---

## Task 7: Listener — rename `reaction` → `reaction_added` and add `reaction_removed`

**Why next:** Pairs with the new `react` command. Touches only the listener, small and contained.

**Files:**
- Modify: `listen/listener.go`
- Modify: `listen/listener_test.go`

### Steps

- [ ] **Step 1: Update existing reaction test in `listener_test.go`**

Find `TestHandleEventsAPI_Reaction` (around line 260) and change the assertion:

```go
	if m["type"] != "reaction_added" {
		t.Errorf("type = %v, want reaction_added", m["type"])
	}
```

(was `"reaction"`)

Add a new failing test below it:

```go
func TestHandleEventsAPI_ReactionRemoved(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.ReactionRemovedEvent{
		User:     "U999",
		Reaction: "thumbsup",
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: "300.001",
		},
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	m := lines[0]
	if m["type"] != "reaction_removed" {
		t.Errorf("type = %v, want reaction_removed", m["type"])
	}
	if m["emoji"] != "thumbsup" {
		t.Errorf("emoji = %v, want thumbsup", m["emoji"])
	}
	if m["item_ts"] != "300.001" {
		t.Errorf("item_ts = %v", m["item_ts"])
	}
}

func TestHandleEventsAPI_ReactionRemovedSelfFiltered(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.ReactionRemovedEvent{
		User:     testBotUserID,
		Reaction: "thumbsup",
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: "300.001",
		},
	}))

	if buf.Len() != 0 {
		t.Errorf("self reaction_removed should be dropped, got: %s", buf.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./listen/ -v -run TestHandleEventsAPI_Reaction
```

Expected: 3 failures — existing reaction test fails on the renamed type, new tests fail (no handler yet).

- [ ] **Step 3: Update `listen/listener.go`**

In `handleEventsAPI`, change the existing `*slackevents.ReactionAddedEvent` case to emit the new type name:

```go
	case *slackevents.ReactionAddedEvent:
		if ev.User == l.botUserID {
			return
		}
		l.emit(Event{
			Type:    "reaction_added",
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})
```

(was `Type: "reaction"`)

Add a new case below it:

```go
	case *slackevents.ReactionRemovedEvent:
		if ev.User == l.botUserID {
			return
		}
		l.emit(Event{
			Type:    "reaction_removed",
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./listen/ -v
```

Expected: all PASS.

- [ ] **Step 5: Build + full suite**

```bash
go build ./... && go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add listen/listener.go listen/listener_test.go
git commit -m "feat(listen): rename reaction event to reaction_added; subscribe to reaction_removed

BREAKING (event schema): consumers parsing type=='reaction' must now parse
type=='reaction_added'. The new type=='reaction_removed' covers user-retracted
reactions. Both apply self-filter."
```

---

## Task 8: `slackline download`

**Why next:** Read-side completion of the file workflow. Self-contained: needs `GetFileInfo` + `GetFile`, both already on the interface.

**Files:**
- Create: `cmd/download.go`
- Create: `cmd/download_test.go`

### Steps

- [ ] **Step 1: Write failing tests (`cmd/download_test.go`)**

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
)

func TestDownload_ToPath(t *testing.T) {
	api := &fakeSlackAPI{
		fileInfo:     &goslack.File{ID: "F123", Name: "report.pdf", Mimetype: "application/pdf", Size: 5, URLPrivate: "https://files.slack.com/F123"},
		getFileBytes: []byte("hello"),
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "report.pdf")
	stderr := &bytes.Buffer{}
	if err := runDownloadWithAPI(api, "F123", out, false, 100*1024*1024, stderr); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "hello" {
		t.Errorf("file contents = %q, want %q", got, "hello")
	}
	// summary JSON should be on stderr
	var summary map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
		t.Fatalf("stderr not valid JSON: %v\n%s", err, stderr.String())
	}
	if summary["ok"] != true {
		t.Errorf("ok = %v", summary["ok"])
	}
	if summary["path"] != out {
		t.Errorf("path = %v, want %s", summary["path"], out)
	}
}

func TestDownload_ToStdout(t *testing.T) {
	api := &fakeSlackAPI{
		fileInfo:     &goslack.File{ID: "F1", Name: "a.txt", Size: 5, URLPrivate: "https://files.slack.com/F1"},
		getFileBytes: []byte("hello"),
	}
	stdout := &bytes.Buffer{}
	if err := runDownloadWithAPIWriter(api, "F1", "-", false, 100*1024*1024, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if stdout.String() != "hello" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "hello")
	}
}

func TestDownload_ExistingFileNoForce(t *testing.T) {
	api := &fakeSlackAPI{
		fileInfo:     &goslack.File{ID: "F1", Name: "a.txt", Size: 5, URLPrivate: "x"},
		getFileBytes: []byte("hello"),
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "x.txt")
	_ = os.WriteFile(out, []byte("existing"), 0o600)

	err := runDownloadWithAPI(api, "F1", out, false, 100*1024*1024, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when target exists and --force is false")
	}
	got, _ := os.ReadFile(out)
	if string(got) != "existing" {
		t.Errorf("file should be untouched, got %q", got)
	}
}

func TestDownload_ExistingFileForce(t *testing.T) {
	api := &fakeSlackAPI{
		fileInfo:     &goslack.File{ID: "F1", Name: "a.txt", Size: 5, URLPrivate: "x"},
		getFileBytes: []byte("new"),
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "x.txt")
	_ = os.WriteFile(out, []byte("existing"), 0o600)

	err := runDownloadWithAPI(api, "F1", out, true, 100*1024*1024, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("download with --force failed: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "new" {
		t.Errorf("file should be overwritten with new content; got %q", got)
	}
}

func TestDownload_SizeExceedsCap(t *testing.T) {
	api := &fakeSlackAPI{
		fileInfo: &goslack.File{ID: "F1", Name: "big.bin", Size: 1024 * 1024},
	}
	err := runDownloadWithAPI(api, "F1", "/tmp/should-not-exist", false, 100, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when file size exceeds cap")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention 'exceeds', got: %v", err)
	}
}

func TestDownload_GetFileInfoError(t *testing.T) {
	api := &fakeSlackAPI{fileInfoErr: errors.New("file_not_found")}
	err := runDownloadWithAPI(api, "F404", "/tmp/x", false, 100*1024*1024, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "file_not_found") {
		t.Errorf("error should mention file_not_found, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/ -v -run TestDownload
```

Expected: build failure ("undefined: runDownloadWithAPI", etc.).

- [ ] **Step 3: Implement `cmd/download.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	"github.com/spf13/cobra"
)

var (
	downloadFile  string
	downloadOut   string
	downloadForce bool
)

const defaultMaxDownloadBytes = int64(100 * 1024 * 1024)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download a file from Slack by file ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		api, err := loadReactAPI() // re-uses the helper from react.go (config + bot token)
		if err != nil {
			return err
		}
		cap := defaultMaxDownloadBytes
		if v := os.Getenv("SLACKLINE_MAX_DOWNLOAD_BYTES"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				cap = n
			}
		}
		if downloadOut == "-" {
			return runDownloadWithAPIWriter(api, downloadFile, "-", downloadForce, cap, cmd.OutOrStdout(), cmd.OutOrStderr())
		}
		return runDownloadWithAPI(api, downloadFile, downloadOut, downloadForce, cap, cmd.OutOrStderr())
	},
}

func init() {
	downloadCmd.Flags().StringVar(&downloadFile, "file", "", "Slack file ID (F...) (required)")
	downloadCmd.Flags().StringVar(&downloadOut, "out", "", "output path, or '-' for stdout (required)")
	downloadCmd.Flags().BoolVar(&downloadForce, "force", false, "overwrite existing file at --out")
	_ = downloadCmd.MarkFlagRequired("file")
	_ = downloadCmd.MarkFlagRequired("out")
	rootCmd.AddCommand(downloadCmd)
}

// runDownloadWithAPI writes to a path on disk.
func runDownloadWithAPI(api slackpkg.SlackAPI, fileID, outPath string, force bool, capBytes int64, stderr io.Writer) error {
	info, _, _, err := api.GetFileInfo(fileID, 0, 0)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "get_file_info_failed", Detail: err.Error()}
	}
	if info.Size > capBytes {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "file_too_large",
			Detail: fmt.Sprintf("file size %d exceeds cap %d (override with SLACKLINE_MAX_DOWNLOAD_BYTES)", info.Size, capBytes),
		}
	}
	if !force {
		if _, statErr := os.Stat(outPath); statErr == nil {
			return &errs.SlackError{Code: errs.Usage, Err: "out_exists", Detail: fmt.Sprintf("%s already exists; pass --force to overwrite", outPath)}
		}
	}
	parent := filepath.Dir(outPath)
	if _, err := os.Stat(parent); err != nil {
		return &errs.SlackError{Code: errs.Usage, Err: "no_parent_dir", Detail: fmt.Sprintf("parent dir %s does not exist", parent)}
	}
	tmpPath := outPath + ".tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return &errs.SlackError{Code: errs.Usage, Err: "tmp_open_failed", Detail: err.Error()}
	}
	if err := api.GetFile(info.URLPrivate, tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return &errs.SlackError{Code: errs.SlackAPI, Err: "download_failed", Detail: err.Error()}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return &errs.SlackError{Code: errs.Usage, Err: "tmp_close_failed", Detail: err.Error()}
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		_ = os.Remove(tmpPath)
		return &errs.SlackError{Code: errs.Usage, Err: "rename_failed", Detail: err.Error()}
	}
	return writeDownloadSummary(stderr, info, outPath)
}

// runDownloadWithAPIWriter writes to a stream (used for --out -).
func runDownloadWithAPIWriter(api slackpkg.SlackAPI, fileID, outPath string, force bool, capBytes int64, stdout, stderr io.Writer) error {
	if outPath != "-" {
		return runDownloadWithAPI(api, fileID, outPath, force, capBytes, stderr)
	}
	info, _, _, err := api.GetFileInfo(fileID, 0, 0)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "get_file_info_failed", Detail: err.Error()}
	}
	if info.Size > capBytes {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "file_too_large",
			Detail: fmt.Sprintf("file size %d exceeds cap %d", info.Size, capBytes),
		}
	}
	if err := api.GetFile(info.URLPrivate, stdout); err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "download_failed", Detail: err.Error()}
	}
	// No summary on stderr for stdout path — keeps consumer parsers simple.
	return nil
}

func writeDownloadSummary(stderr io.Writer, info downloadInfo, path string) error {
	out := struct {
		OK       bool   `json:"ok"`
		File     string `json:"file"`
		Name     string `json:"name"`
		Mimetype string `json:"mimetype"`
		Size     int    `json:"size"`
		Path     string `json:"path"`
	}{OK: true, File: info.fileID(), Name: info.name(), Mimetype: info.mime(), Size: info.size(), Path: path}
	enc := json.NewEncoder(stderr)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// downloadInfo abstracts only the fields we need from goslack.File so the
// summary helper is testable without constructing a full goslack.File.
type downloadInfo interface {
	fileID() string
	name() string
	mime() string
	size() int
}
```

We don't actually need the `downloadInfo` interface since the implementation passes `*goslack.File` directly. Replace the `writeDownloadSummary` signature and call sites:

```go
func writeDownloadSummary(stderr io.Writer, info *goslack.File, path string) error {
	out := struct {
		OK       bool   `json:"ok"`
		File     string `json:"file"`
		Name     string `json:"name"`
		Mimetype string `json:"mimetype"`
		Size     int    `json:"size"`
		Path     string `json:"path"`
	}{OK: true, File: info.ID, Name: info.Name, Mimetype: info.Mimetype, Size: info.Size, Path: path}
	enc := json.NewEncoder(stderr)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
```

And add the import: `goslack "github.com/slack-go/slack"`. Delete the `downloadInfo` interface.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/ -v -run TestDownload
```

Expected: all PASS.

- [ ] **Step 5: Build + full suite**

```bash
go build ./... && go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add cmd/download.go cmd/download_test.go
git commit -m "feat(cmd): add slackline download with atomic write + size cap

--out PATH writes via .tmp+rename atomic. --out - streams to stdout, no
summary line. SLACKLINE_MAX_DOWNLOAD_BYTES env var overrides 100 MB default.
--force required to overwrite existing target."
```

---

## Task 9: `slackline send --attach`

**Why next:** Send-side completion of the file workflow. Uses `CompleteUploadExternal` (Task 3) for the multi-file batch path.

**Files:**
- Modify: `cmd/send.go`
- Create or modify: `cmd/send_test.go` (file does not currently exist; create it)

### Steps

- [ ] **Step 1: Write failing tests (`cmd/send_test.go`)**

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
)

func TestSend_TextOnlyUsesPostMessage(t *testing.T) {
	api := &fakeSlackAPI{}
	stdout := &bytes.Buffer{}
	err := runSendWithAPI(api, "C123", "hello", "", nil, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.lastCompleteUploadCall.ChannelID != "" {
		t.Error("text-only send should NOT use CompleteUploadExternal")
	}
}

func TestSend_WithSingleAttach(t *testing.T) {
	api := &fakeSlackAPI{
		completeUploadResp: []goslack.FileSummary{{ID: "F1"}},
	}
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.txt")
	_ = os.WriteFile(a, []byte("abc"), 0o600)

	stdout := &bytes.Buffer{}
	err := runSendWithAPI(api, "C123", "see this", "", []string{a}, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	got := api.lastCompleteUploadCall
	if got.ChannelID != "C123" {
		t.Errorf("channel = %q", got.ChannelID)
	}
	if got.InitialComment != "see this" {
		t.Errorf("initial_comment = %q", got.InitialComment)
	}
	if len(got.Files) != 1 || got.Files[0].Path != a {
		t.Errorf("files = %+v", got.Files)
	}
	var out map[string]interface{}
	_ = json.Unmarshal(stdout.Bytes(), &out)
	files, _ := out["files"].([]interface{})
	if len(files) != 1 {
		t.Errorf("output files = %+v", files)
	}
}

func TestSend_AttachWithoutMessage(t *testing.T) {
	api := &fakeSlackAPI{
		completeUploadResp: []goslack.FileSummary{{ID: "F1"}},
	}
	tmp := t.TempDir()
	p := filepath.Join(tmp, "f")
	_ = os.WriteFile(p, []byte("x"), 0o600)
	if err := runSendWithAPI(api, "C123", "", "", []string{p}, &bytes.Buffer{}); err != nil {
		t.Fatalf("send without message failed: %v", err)
	}
	if api.lastCompleteUploadCall.InitialComment != "" {
		t.Errorf("initial_comment should be empty, got %q", api.lastCompleteUploadCall.InitialComment)
	}
}

func TestSend_AttachInThread(t *testing.T) {
	api := &fakeSlackAPI{
		completeUploadResp: []goslack.FileSummary{{ID: "F1"}},
	}
	tmp := t.TempDir()
	p := filepath.Join(tmp, "f")
	_ = os.WriteFile(p, []byte("x"), 0o600)
	if err := runSendWithAPI(api, "C123", "", "1000.000", []string{p}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if api.lastCompleteUploadCall.ThreadTS != "1000.000" {
		t.Errorf("thread_ts = %q", api.lastCompleteUploadCall.ThreadTS)
	}
}

func TestSend_AttachMultipleFiles(t *testing.T) {
	api := &fakeSlackAPI{
		completeUploadResp: []goslack.FileSummary{{ID: "F1"}, {ID: "F2"}},
	}
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a")
	b := filepath.Join(tmp, "b")
	_ = os.WriteFile(a, []byte("a"), 0o600)
	_ = os.WriteFile(b, []byte("b"), 0o600)
	if err := runSendWithAPI(api, "C123", "two files", "", []string{a, b}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(api.lastCompleteUploadCall.Files) != 2 {
		t.Errorf("files = %+v", api.lastCompleteUploadCall.Files)
	}
}

func TestSend_AttachMissingFile(t *testing.T) {
	api := &fakeSlackAPI{}
	err := runSendWithAPI(api, "C123", "", "", []string{"/nonexistent/file.txt"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSend_AttachExceedsSizeCap(t *testing.T) {
	api := &fakeSlackAPI{}
	tmp := t.TempDir()
	p := filepath.Join(tmp, "big")
	// 200 bytes file vs 100-byte cap.
	_ = os.WriteFile(p, bytes.Repeat([]byte("x"), 200), 0o600)
	t.Setenv("SLACKLINE_MAX_UPLOAD_BYTES", "100")
	err := runSendWithAPI(api, "C123", "", "", []string{p}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected size-cap error")
	}
}

// Compile-time check on slackpkg import used.
var _ slackpkg.SlackAPI = (*fakeSlackAPI)(nil)
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/ -v -run TestSend
```

Expected: build failure ("undefined: runSendWithAPI" — the existing `runSend` reads global flag values; we need a testable version).

- [ ] **Step 3: Refactor `cmd/send.go` to add a testable core + `--attach` support**

Replace the contents of `cmd/send.go`:

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	sendChannel string
	sendMessage string
	sendThread  string
	sendAttach  []string
)

const defaultMaxUploadBytes = int64(100 * 1024 * 1024)

func init() {
	sendCmd.Flags().StringVar(&sendChannel, "channel", "", "channel name (#ops), ID (C...), or Slack URL (required)")
	sendCmd.Flags().StringVar(&sendMessage, "message", "", "message text (reads stdin if omitted; optional when --attach is used)")
	sendCmd.Flags().StringVar(&sendThread, "thread", "", "thread timestamp to reply to")
	sendCmd.Flags().StringArrayVar(&sendAttach, "attach", nil, "attach a file by path (repeatable)")
	_ = sendCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(sendCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message (and optionally one or more files) to a Slack channel",
	Long:  "Send a message to a channel. Message can be passed via --message, piped via stdin, or omitted entirely when one or more --attach flags are present.",
	RunE: func(cmd *cobra.Command, args []string) error {
		text := sendMessage
		if text == "" && len(sendAttach) == 0 {
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) != 0 {
				return &errs.SlackError{Code: errs.Usage, Err: "no_message", Detail: "Provide --message, pipe text to stdin, or pass at least one --attach"}
			}
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return &errs.SlackError{Code: errs.Usage, Err: "stdin_read_error", Detail: err.Error()}
			}
			text = strings.TrimRight(string(data), "\n")
		}

		cfg, _, err := loadConfig()
		if err != nil {
			return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
		}
		if cfg.Bot.BotToken == "" {
			return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "Run 'slackline init' to set up."}
		}
		api := slackpkg.NewClient(cfg.Bot.BotToken)
		channelID, err := resolveChannel(api, sendChannel)
		if err != nil {
			return err
		}

		return runSendWithAPI(api, channelID, text, sendThread, sendAttach, cmd.OutOrStdout())
	},
}

// runSendWithAPI is the testable core. attachPaths == nil → text-only path.
func runSendWithAPI(api slackpkg.SlackAPI, channelID, text, threadTS string, attachPaths []string, stdout io.Writer) error {
	if len(attachPaths) == 0 {
		if text == "" {
			return &errs.SlackError{Code: errs.Usage, Err: "empty_message", Detail: "Message cannot be empty when no --attach is provided"}
		}
		opts := []goslack.MsgOption{goslack.MsgOptionText(text, false)}
		if threadTS != "" {
			opts = append(opts, goslack.MsgOptionTS(threadTS))
		}
		respChan, ts, err := api.PostMessage(channelID, opts...)
		if err != nil {
			if isAuthError(err) {
				return errs.AuthError(err.Error())
			}
			return &errs.SlackError{Code: errs.SlackAPI, Err: "send_failed", Detail: err.Error()}
		}
		return writeSendJSON(stdout, respChan, ts, threadTS, nil)
	}

	// Validate all files first.
	if err := validateAttachments(attachPaths); err != nil {
		return err
	}

	uploads := make([]slackpkg.FileUpload, len(attachPaths))
	for i, p := range attachPaths {
		uploads[i] = slackpkg.FileUpload{Path: p}
	}
	results, err := api.CompleteUploadExternal(channelID, threadTS, text, uploads)
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "upload_failed", Detail: err.Error()}
	}
	files := make([]map[string]string, len(results))
	for i, r := range results {
		files[i] = map[string]string{"id": r.ID, "permalink": r.Permalink}
	}
	// We don't have a definitive ts from the upload response; leave it empty.
	return writeSendJSON(stdout, channelID, "", threadTS, files)
}

func validateAttachments(paths []string) error {
	cap := defaultMaxUploadBytes
	if v := os.Getenv("SLACKLINE_MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cap = n
		}
	}
	var total int64
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "attach_not_found", Detail: fmt.Sprintf("%s: %v", p, err)}
		}
		if !st.Mode().IsRegular() {
			return &errs.SlackError{Code: errs.Usage, Err: "attach_not_regular", Detail: fmt.Sprintf("%s is not a regular file", p)}
		}
		total += st.Size()
	}
	if total > cap {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "upload_size_exceeded",
			Detail: fmt.Sprintf("combined upload size %d exceeds cap %d (override with SLACKLINE_MAX_UPLOAD_BYTES)", total, cap),
		}
	}
	return nil
}

func writeSendJSON(stdout io.Writer, channelID, ts, threadTS string, files []map[string]string) error {
	out := struct {
		OK       bool                `json:"ok"`
		Channel  string              `json:"channel"`
		TS       string              `json:"ts,omitempty"`
		ThreadTS string              `json:"thread_ts,omitempty"`
		Files    []map[string]string `json:"files,omitempty"`
	}{OK: true, Channel: channelID, TS: ts, ThreadTS: threadTS, Files: files}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/ -v -run TestSend
```

Expected: all PASS.

- [ ] **Step 5: Build + full suite**

```bash
go build ./... && go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add cmd/send.go cmd/send_test.go
git commit -m "feat(cmd): add --attach repeatable flag to slackline send

Accepts N file paths; uploads them as a single Slack message via
CompleteUploadExternal with --message becoming initial_comment.
SLACKLINE_MAX_UPLOAD_BYTES env var caps total upload (default 100 MB).
--message becomes optional when --attach is present."
```

---

## Task 10: Listener — `--include-bot-self` flag

**Why next:** Smallest of the listener changes. Lays the groundwork for the modes flags by introducing the per-listener config struct.

**Files:**
- Modify: `listen/listener.go`
- Modify: `listen/listener_test.go`
- Modify: `cmd/listen.go`

### Steps

- [ ] **Step 1: Write failing test**

Append to `listen/listener_test.go`:

```go
func TestHandleEventsAPI_IncludeBotSelf_Mention(t *testing.T) {
	l, buf := newTestListener()
	l.includeBotSelf = true // bypass the default self-filter
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.AppMentionEvent{
		User:    testBotUserID,
		Text:    "I mentioned myself",
		Channel: testChannelID,
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("expected event to pass through with --include-bot-self, got %d", len(lines))
	}
}

func TestHandleEventsAPI_IncludeBotSelf_Reaction(t *testing.T) {
	l, buf := newTestListener()
	l.includeBotSelf = true
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.ReactionAddedEvent{
		User:     testBotUserID,
		Reaction: "thumbsup",
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: "300.001",
		},
	}))
	if buf.Len() == 0 {
		t.Error("self reaction should pass with --include-bot-self")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./listen/ -v -run IncludeBotSelf
```

Expected: build failure ("l.includeBotSelf undefined").

- [ ] **Step 3: Update `listen/listener.go`**

Add a field on `Listener`:

```go
type Listener struct {
	api            *goslack.Client
	sm             *socketmode.Client
	botUserID      string
	out            io.Writer
	status         io.Writer
	includeBotSelf bool
}
```

Update `NewListener` to accept it:

```go
// NewListener creates a Socket Mode listener. botUserID is used to filter
// self-messages unless includeBotSelf is true.
func NewListener(botToken, appToken, botUserID string, includeBotSelf bool, out, status io.Writer) *Listener {
	api := goslack.New(botToken, goslack.OptionAppLevelToken(appToken))
	sm := socketmode.New(api)
	return &Listener{
		api:            api,
		sm:             sm,
		botUserID:      botUserID,
		out:            out,
		status:         status,
		includeBotSelf: includeBotSelf,
	}
}
```

Replace the four self-filter checks (`if ev.User == l.botUserID { return }`) with a helper:

```go
// shouldFilterSelf returns true when an event by the given user should be dropped.
func (l *Listener) shouldFilterSelf(user string) bool {
	if l.includeBotSelf {
		return false
	}
	return user == l.botUserID
}
```

Replace each `if ev.User == l.botUserID { return }` with `if l.shouldFilterSelf(ev.User) { return }`. (4 spots: AppMention, Message, ReactionAdded, ReactionRemoved.)

- [ ] **Step 4: Update `cmd/listen.go` to pass the new flag**

```go
package cmd

import (
	"os"

	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/prime-radiant-inc/slackline/listen"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	"github.com/spf13/cobra"
)

var listenIncludeBotSelf bool

func init() {
	listenCmd.Flags().BoolVar(&listenIncludeBotSelf, "include-bot-self", false, "include events authored by the bot itself (default: filtered)")
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

	api := slackpkg.NewClient(cfg.Bot.BotToken)
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: err.Error()}
	}

	listener := listen.NewListener(cfg.Bot.BotToken, cfg.Bot.AppToken, authResp.UserID, listenIncludeBotSelf, os.Stdout, os.Stderr)
	return listener.Run()
}
```

- [ ] **Step 5: Run all listen tests + cmd tests**

```bash
go test ./listen/ ./cmd/ -v
```

Expected: all green. The existing self-filter tests still pass (default `includeBotSelf=false`).

- [ ] **Step 6: Build full**

```bash
go build ./... && go test ./...
```

Expected: green.

- [ ] **Step 7: Commit**

```bash
git add listen/listener.go listen/listener_test.go cmd/listen.go
git commit -m "feat(listen): add --include-bot-self flag

Defeats the default self-filter so observability bots can audit their own
outputs. Self-filter still applies to all four event types (mention, dm,
reaction_added, reaction_removed) when the flag is absent."
```

---

## Task 11: Listener — `files` array on existing event types

**Why next:** Surface attached files on all currently-emitted message events (`mention`, `dm`). Schema also lays the foundation for Tasks 12–13 which add `thread_reply`/`channel_message`.

**Files:**
- Modify: `listen/events.go`
- Modify: `listen/listener.go`
- Modify: `listen/listener_test.go`
- Modify: `cmd/read.go`
- Modify: `cmd/helpers_test.go` (existing `TestMessageOutput_*` tests need update to allow files field)

### Steps

- [ ] **Step 1: Write failing test (`listener_test.go`)**

```go
func TestHandleEventsAPI_DM_WithFiles(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      "U999",
		Text:      "see this",
		Channel:   "D01TESTDM00",
		TimeStamp: "200.001",
		Files: []slackevents.File{
			{ID: "F123", Name: "report.pdf", Mimetype: "application/pdf", Size: 12345, Title: "Q4 Report"},
			{ID: "F456", Name: "extra.png", Mimetype: "image/png", Size: 6789},
		},
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	files, ok := lines[0]["files"].([]interface{})
	if !ok {
		t.Fatalf("files field missing or wrong type: %v", lines[0])
	}
	if len(files) != 2 {
		t.Fatalf("files length = %d, want 2", len(files))
	}
	first := files[0].(map[string]interface{})
	if first["id"] != "F123" {
		t.Errorf("files[0].id = %v", first["id"])
	}
	if first["name"] != "report.pdf" {
		t.Errorf("files[0].name = %v", first["name"])
	}
	if first["mimetype"] != "application/pdf" {
		t.Errorf("files[0].mimetype = %v", first["mimetype"])
	}
}

func TestHandleEventsAPI_Mention_WithFiles(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.AppMentionEvent{
		User:      "U999",
		Text:      "look at this",
		Channel:   testChannelID,
		TimeStamp: "100.001",
		Files: []slackevents.File{
			{ID: "F1", Name: "a.txt", Mimetype: "text/plain", Size: 5},
		},
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	files, _ := lines[0]["files"].([]interface{})
	if len(files) != 1 {
		t.Fatalf("expected 1 file in mention event, got %d", len(files))
	}
}

func TestHandleEventsAPI_DM_NoFilesOmitsArray(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      "U999",
		Text:      "no files",
		Channel:   "D01TESTDM00",
		TimeStamp: "200.001",
	}))

	lines := parseJSONL(t, buf)
	if _, ok := lines[0]["files"]; ok {
		t.Error("files key should be omitted when there are no attachments")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./listen/ -v -run "WithFiles|NoFilesOmitsArray"
```

Expected: failure ("Files field undefined on Event") or test failure.

- [ ] **Step 3: Update `listen/events.go`**

```go
package listen

// Event represents a Slack event to be serialized as JSONL output.
type Event struct {
	Type           string     `json:"type"`
	Channel        string     `json:"channel"`
	User           string     `json:"user,omitempty"`
	Text           string     `json:"text,omitempty"`
	TS             string     `json:"ts,omitempty"`
	ThreadTS       string     `json:"thread_ts,omitempty"`
	Emoji          string     `json:"emoji,omitempty"`
	ItemTS         string     `json:"item_ts,omitempty"`
	ParentUserID   string     `json:"parent_user_id,omitempty"`
	Files          []FileMeta `json:"files,omitempty"`
}

// FileMeta is the receive-side schema for attached files on message events.
// URLs and download tokens are intentionally absent — caller fetches via
// `slackline download --file ID --out PATH`.
type FileMeta struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Mimetype string `json:"mimetype,omitempty"`
	Size     int    `json:"size"`
	Title    string `json:"title,omitempty"`
}
```

- [ ] **Step 4: Update `listen/listener.go` to populate Files on existing handlers**

In `handleEventsAPI`, update the `AppMentionEvent` and DM `MessageEvent` cases to include files:

```go
	case *slackevents.AppMentionEvent:
		if l.shouldFilterSelf(ev.User) {
			return
		}
		l.emit(Event{
			Type:     "mention",
			Channel:  ev.Channel,
			User:     ev.User,
			Text:     ev.Text,
			TS:       ev.TimeStamp,
			ThreadTS: ev.ThreadTimeStamp,
			Files:    convertFiles(ev.Files),
		})

	case *slackevents.MessageEvent:
		if len(ev.Channel) == 0 || ev.Channel[0] != 'D' {
			return
		}
		if l.shouldFilterSelf(ev.User) {
			return
		}
		// "file_share" subtype is allowed (it carries Files); skip other subtypes.
		if ev.SubType != "" && ev.SubType != "file_share" {
			return
		}
		l.emit(Event{
			Type:     "dm",
			Channel:  ev.Channel,
			User:     ev.User,
			Text:     ev.Text,
			TS:       ev.TimeStamp,
			ThreadTS: ev.ThreadTimeStamp,
			Files:    convertFiles(ev.Files),
		})
```

Add a helper at the bottom of `listener.go`:

```go
func convertFiles(in []slackevents.File) []FileMeta {
	if len(in) == 0 {
		return nil
	}
	out := make([]FileMeta, len(in))
	for i, f := range in {
		out[i] = FileMeta{
			ID:       f.ID,
			Name:     f.Name,
			Mimetype: f.Mimetype,
			Size:     f.Size,
			Title:    f.Title,
		}
	}
	return out
}
```

- [ ] **Step 5: Update `cmd/read.go` to include files in JSONL**

Replace the `messageOutput` struct and the loop body in `runRead`:

```go
type messageOutput struct {
	TS       string         `json:"ts"`
	User     string         `json:"user"`
	Text     string         `json:"text"`
	ThreadTS string         `json:"thread_ts,omitempty"`
	Files    []fileMetaJSON `json:"files,omitempty"`
}

type fileMetaJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Mimetype string `json:"mimetype,omitempty"`
	Size     int    `json:"size"`
	Title    string `json:"title,omitempty"`
}
```

In the loop:

```go
	for _, m := range messages {
		threadTS := m.ThreadTimestamp
		if threadTS == m.Timestamp {
			threadTS = ""
		}
		var files []fileMetaJSON
		for _, f := range m.Files {
			files = append(files, fileMetaJSON{
				ID:       f.ID,
				Name:     f.Name,
				Mimetype: f.Mimetype,
				Size:     f.Size,
				Title:    f.Title,
			})
		}
		out := messageOutput{
			TS:       m.Timestamp,
			User:     m.User,
			Text:     m.Text,
			ThreadTS: threadTS,
			Files:    files,
		}
		if err := enc.Encode(out); err != nil {
			return err
		}
	}
```

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add listen/events.go listen/listener.go listen/listener_test.go cmd/read.go
git commit -m "feat(listen,read): include files array on message events

mention/dm events emitted from listener now carry a files array when the
underlying Slack event has attachments. Same schema appears in the JSONL
output of slackline read. URLs are intentionally omitted; caller uses
slackline download --file ID --out PATH to fetch bytes."
```

---

## Task 12: Listener — `--threads` mode (thread_reply event)

**Why next:** Builds on Task 11's files schema. Adds the new `thread_reply` event type emitted only when the bot is the thread parent.

**Files:**
- Modify: `listen/listener.go`
- Modify: `listen/listener_test.go`
- Modify: `cmd/listen.go`

### Steps

- [ ] **Step 1: Write failing tests**

```go
func TestHandleEventsAPI_ThreadsMode_BotParentReply(t *testing.T) {
	l, buf := newTestListener()
	l.threads = true

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:            "U999",
		Text:            "thanks bot",
		Channel:         "C01TESTCHAN",
		TimeStamp:       "200.001",
		ThreadTimeStamp: "100.000",
		ParentUserId:    testBotUserID,
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	if lines[0]["type"] != "thread_reply" {
		t.Errorf("type = %v, want thread_reply", lines[0]["type"])
	}
	if lines[0]["thread_ts"] != "100.000" {
		t.Errorf("thread_ts = %v", lines[0]["thread_ts"])
	}
	if lines[0]["parent_user_id"] != testBotUserID {
		t.Errorf("parent_user_id = %v", lines[0]["parent_user_id"])
	}
}

func TestHandleEventsAPI_ThreadsMode_NotBotParentDropped(t *testing.T) {
	l, buf := newTestListener()
	l.threads = true

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:            "U999",
		Text:            "talking among themselves",
		Channel:         "C01TESTCHAN",
		TimeStamp:       "200.001",
		ThreadTimeStamp: "100.000",
		ParentUserId:    "U_OTHER",
	}))

	if buf.Len() != 0 {
		t.Errorf("thread reply with non-bot parent should be dropped in --threads mode, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_ThreadsMode_NonThreadDropped(t *testing.T) {
	l, buf := newTestListener()
	l.threads = true

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      "U999",
		Text:      "top-level channel msg",
		Channel:   "C01TESTCHAN",
		TimeStamp: "200.001",
	}))

	if buf.Len() != 0 {
		t.Error("non-thread channel message should be dropped in --threads mode without --all-messages")
	}
}

func TestHandleEventsAPI_DefaultMode_ChannelMessageStillDropped(t *testing.T) {
	l, buf := newTestListener()
	// Default — no --threads, no --all-messages.

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:            "U999",
		Text:            "thanks bot",
		Channel:         "C01TESTCHAN",
		TimeStamp:       "200.001",
		ThreadTimeStamp: "100.000",
		ParentUserId:    testBotUserID,
	}))

	if buf.Len() != 0 {
		t.Errorf("default mode should drop all channel messages, got: %s", buf.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./listen/ -v -run ThreadsMode
```

Expected: build failure ("l.threads undefined").

- [ ] **Step 3: Update `listen/listener.go`**

Add field to `Listener`:

```go
type Listener struct {
	api            *goslack.Client
	sm             *socketmode.Client
	botUserID      string
	out            io.Writer
	status         io.Writer
	includeBotSelf bool
	threads        bool
	allMessages    bool
}
```

Update `NewListener` signature:

```go
func NewListener(botToken, appToken, botUserID string, opts ListenerOptions, out, status io.Writer) *Listener {
	api := goslack.New(botToken, goslack.OptionAppLevelToken(appToken))
	sm := socketmode.New(api)
	return &Listener{
		api:            api,
		sm:             sm,
		botUserID:      botUserID,
		out:            out,
		status:         status,
		includeBotSelf: opts.IncludeBotSelf,
		threads:        opts.Threads,
		allMessages:    opts.AllMessages,
	}
}

// ListenerOptions bundles the per-mode flags so signature growth is
// contained as we add modes.
type ListenerOptions struct {
	IncludeBotSelf bool
	Threads        bool
	AllMessages    bool
}
```

In the `*slackevents.MessageEvent` case in `handleEventsAPI`, extend the channel-prefix check to handle non-DM channels:

```go
	case *slackevents.MessageEvent:
		if len(ev.Channel) == 0 {
			return
		}
		// DM channels (D...) flow through the existing DM path.
		if ev.Channel[0] == 'D' {
			if l.shouldFilterSelf(ev.User) {
				return
			}
			if ev.SubType != "" && ev.SubType != "file_share" {
				return
			}
			l.emit(Event{
				Type:     "dm",
				Channel:  ev.Channel,
				User:     ev.User,
				Text:     ev.Text,
				TS:       ev.TimeStamp,
				ThreadTS: ev.ThreadTimeStamp,
				Files:    convertFiles(ev.Files),
			})
			return
		}
		// Non-DM channels (C... public, G... legacy private). Emit only when a mode allows it.
		if l.shouldFilterSelf(ev.User) {
			return
		}
		if ev.SubType != "" && ev.SubType != "file_share" {
			return
		}
		isThread := ev.ThreadTimeStamp != "" && ev.ThreadTimeStamp != ev.TimeStamp
		switch {
		case l.allMessages:
			eventType := "channel_message"
			if isThread {
				eventType = "thread_reply"
			}
			l.emit(Event{
				Type:         eventType,
				Channel:      ev.Channel,
				User:         ev.User,
				Text:         ev.Text,
				TS:           ev.TimeStamp,
				ThreadTS:     ev.ThreadTimeStamp,
				ParentUserID: ev.ParentUserId,
				Files:        convertFiles(ev.Files),
			})
		case l.threads && isThread && ev.ParentUserId == l.botUserID:
			l.emit(Event{
				Type:         "thread_reply",
				Channel:      ev.Channel,
				User:         ev.User,
				Text:         ev.Text,
				TS:           ev.TimeStamp,
				ThreadTS:     ev.ThreadTimeStamp,
				ParentUserID: ev.ParentUserId,
				Files:        convertFiles(ev.Files),
			})
		}
```

(Note: the field name on `slackevents.MessageEvent` is `ParentUserId` — verify in the version of slack-go pinned by `go.mod`. If it's `ParentUserID` instead, swap accordingly.)

- [ ] **Step 4: Update `cmd/listen.go`**

```go
var (
	listenIncludeBotSelf bool
	listenThreads        bool
	listenAllMessages    bool
)

func init() {
	listenCmd.Flags().BoolVar(&listenIncludeBotSelf, "include-bot-self", false, "include events authored by the bot itself (default: filtered)")
	listenCmd.Flags().BoolVar(&listenThreads, "threads", false, "also emit thread_reply events for threads the bot has participated in")
	listenCmd.Flags().BoolVar(&listenAllMessages, "all-messages", false, "firehose: emit every message in every channel the bot is in (implies --threads)")
	rootCmd.AddCommand(listenCmd)
}

func runListen(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "Run 'slackline init'."}
	}
	if cfg.Bot.AppToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_app_token", Detail: "Socket Mode requires an app token (xapp-)."}
	}
	api := slackpkg.NewClient(cfg.Bot.BotToken)
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: err.Error()}
	}
	listener := listen.NewListener(cfg.Bot.BotToken, cfg.Bot.AppToken, authResp.UserID, listen.ListenerOptions{
		IncludeBotSelf: listenIncludeBotSelf,
		Threads:        listenThreads || listenAllMessages, // --all-messages implies --threads
		AllMessages:    listenAllMessages,
	}, os.Stdout, os.Stderr)
	return listener.Run()
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./listen/ ./cmd/ -v
```

Expected: green. The previous Task 10 IncludeBotSelf tests will need updates (the `NewListener` signature changed). Find any test calling `NewListener` directly and update; the listener_test.go uses `&Listener{...}` literals so those don't change.

- [ ] **Step 6: Build + full**

```bash
go build ./... && go test ./...
```

Expected: green.

- [ ] **Step 7: Commit**

```bash
git add listen/listener.go listen/listener_test.go cmd/listen.go
git commit -m "feat(listen): add --threads and --all-messages flags

--threads: emit thread_reply events for replies in threads where the bot is
parent_user_id (stateless — no API lookups). --all-messages: firehose mode,
emits both channel_message and thread_reply for every visible channel msg.
Default mode unchanged (mentions + DMs only)."
```

---

## Task 13: Plugin manifest + `slackline-provision-bot` skill

**Why next:** All of the runtime work is complete. Now ship the documentation surface — the plugin manifest making slackline a Claude Code plugin, and the new skill that documents the agentic recipe.

**Files:**
- Create: `.claude-plugin/plugin.json`
- Create: `skills/slackline-provision-bot/SKILL.md`
- Create: `skills/slackline-provision-bot/copy-buttons.md`

### Steps

- [ ] **Step 1: Create `.claude-plugin/plugin.json`**

```bash
mkdir -p .claude-plugin
```

```json
{
  "name": "slackline",
  "version": "0.1.0",
  "description": "Give AI agents a Slack identity. CLI for sending messages, reading channels, listening for events, with reactions and file attachments.",
  "author": {
    "name": "slackline contributors"
  }
}
```

(Place at `.claude-plugin/plugin.json`.)

- [ ] **Step 2: Create `skills/slackline-provision-bot/SKILL.md`**

```bash
mkdir -p skills/slackline-provision-bot
```

````markdown
---
name: slackline-provision-bot
description: Use when asked to create, deploy, provision, or set up a new Slackline bot in a Slack workspace. Drives the end-to-end flow with browser automation for the install + token-collection steps that Slack requires through their admin UI.
---

# Provisioning a Slackline Bot

Use this skill when you need to deploy a new Slackline bot in a Slack workspace. The flow assumes:

- `slackline` binary is installed and on PATH.
- A workspace admin's browser session is established on this host (one-time per machine — Chrome/Playwright/etc. is signed into Slack as an admin).
- `slackline provision bootstrap` has been run on this host once to seed `~/.config/slackline/provision.json` with App Configuration Tokens. (If not, run it first; see `## Bootstrap` below.)
- You have access to a browser-driving tool (`use_browser`, Playwright MCP, or equivalent).

The full agentic recipe is below.

## End-to-end recipe

```bash
# 1. Provision the app via API.
slackline provision my-bot-name > /tmp/prov.json
# stdout: {"ok":true,"app_id":"A...","install_url":"...","oauth_authorize_url":"...","oauth_page_url":"...","general_page_url":"..."}

INSTALL_URL=$(jq -r .oauth_authorize_url /tmp/prov.json)
OAUTH_PAGE=$(jq -r .oauth_page_url /tmp/prov.json)
GENERAL_PAGE=$(jq -r .general_page_url /tmp/prov.json)
```

```text
# 2. Drive browser:
#    - navigate to $INSTALL_URL
#    - click button[data-qa="oauth_submit_button"]   ("Allow")
#    - navigate to $OAUTH_PAGE
#    - click button.c-button.c-button--outline.c-button--small  (the "Copy" button — one per page on /oauth)
#    - read clipboard via `pbpaste` → save bot token (xoxb-)
#    - navigate to $GENERAL_PAGE
#    - click button.c-button.c-button--outline.c-button--medium.margin_top_150  ("Generate Token and Scopes")
#    - dismiss any "Got it!" tutorial popup
#    - type "socket-mode" into input[name="app_level_tokens_generate_modal_description"]
#    - click button[data-qa="app_scopes_list_add_oauth_scope"]  ("Add Scope")
#    - type "connections:write" into input[role="combobox"][aria-label="Select Scopes"]
#    - press Enter (commits the option — direct .click() on the option doesn't fire React handler)
#    - click button.c-button.c-button--primary.c-button--medium  ("Generate")
#    - click button.p-app_level_tokens_info__input_button  (the "Copy" button on the new app-level token modal)
#    - read clipboard via `pbpaste` → save app token (xapp-)
```

```bash
# 3. Configure slackline with the captured tokens.
SLACKLINE_BOT_TOKEN="$BOT_TOKEN" \
SLACKLINE_APP_TOKEN="$APP_TOKEN" \
SLACKLINE_WORKSPACE_URL="https://${WORKSPACE_DOMAIN}.slack.com" \
slackline init
# Writes ~/.config/slackline/config.json. Validates via auth.test.

# 4. Smoke test.
slackline auth status
slackline channels --all
```

The bot is now provisioned and ready. To actually post in a channel, the bot needs to be invited (`/invite @bot-name` from any channel member). Driving that through the browser is similar — see `slackline-claude` provisioning recipe used 2026-04-25 for a worked example.

## Critical browser-automation gotchas

1. **Use real CDP clicks, not JS-driven `element.click()`.** Slack's React handlers don't fire on synthetic JS click events for the Copy buttons. With `use_browser`, the `click` action does the right thing; the `eval` action with `el.click()` does not.

2. **The clipboard read pattern only works after a real click.** First click → check clipboard with `pbpaste`. If the clipboard is empty, the click didn't trigger Slack's copy handler.

3. **Combobox selection requires Enter, not click.** The "Select Scopes" combobox in the generate-app-level-token modal: type the scope name, then press Enter. Clicking the option in the listbox does NOT add it to the form (verified 2026-04-25).

4. **Slack's `/messages/<channel-id>` page sometimes shows "couldn't load" briefly while the web app boots.** Use `await_element` for the composer (`div[role="textbox"][aria-label^="Message"]`) before typing.

5. **`/invite @bot-name` slash command in a channel:** type `/invite @<name>`, press Enter — that triggers Slack's autocomplete which resolves the @mention to a real user. Press Enter AGAIN to actually send the slash command. Two Enters total.

For the full selector reference and the rationale behind each, see `copy-buttons.md` next to this file.

## Bootstrap (one-time per machine)

Run before any per-bot provisioning:

```bash
# Option A: env vars (CI/scripts).
SLACKLINE_CONFIG_TOKEN=xoxe.xoxp-... \
SLACKLINE_REFRESH_TOKEN=xoxe-... \
slackline provision bootstrap

# Option B: interactive (paste from https://api.slack.com/apps "Your App Configuration Tokens").
slackline provision bootstrap
# Prompts for both tokens via stdin.
```

Bootstrap writes `~/.config/slackline/provision.json` with mode 0600.

## Migration note

The old `slackline create` command has been removed. It returns a discoverable migration error pointing here. Update any scripts referencing `slackline create --init` / `slackline create --name X` to use `slackline provision bootstrap` and `slackline provision <name>` respectively, plus `slackline init` (with env vars) for the per-bot config.json write.
````

- [ ] **Step 3: Create `skills/slackline-provision-bot/copy-buttons.md`**

```markdown
# Slack Admin UI — selector reference

These selectors were verified 2026-04-25 against api.slack.com. Slack updates their admin UI periodically; if a step fails, re-grep the rendered DOM for an `aria-label` or `data-qa` close to the broken selector — those tend to be the most stable.

## Configure App Configuration Tokens
URL: `https://api.slack.com/apps`

| Action | Selector |
|---|---|
| "Generate Token" button (under Your App Configuration Tokens) | `button.c-button.c-button--outline.c-button--medium` containing text "Generate Token" — there is one such button on the page |
| Workspace dropdown in the modal | `[role="combobox"][aria-label="Select a team"]` |
| Workspace option | `#team-picker_option_0` (or `[data-qa="team_picker_option_0"]`) |
| "Generate" button (modal) | `button.c-button.c-button--primary.c-button--medium` |
| Copy access token (post-generate row) | `button[aria-label="Copy access token"]` |
| Copy refresh token | `button[aria-label="Copy refresh token"]` |

## OAuth install URL
URL: `https://slack.com/oauth/v2/authorize?client_id=…&team=…&scope=…` (returned in `provision`'s `oauth_authorize_url` field)

| Action | Selector |
|---|---|
| Allow button | `button[data-qa="oauth_submit_button"]` |

## Bot token page
URL: `https://api.slack.com/apps/<app_id>/oauth`

| Action | Selector |
|---|---|
| Copy bot token (xoxb-) | `button.c-button.c-button--outline.c-button--small` (single Copy button on this page) |

## App-level token generation page
URL: `https://api.slack.com/apps/<app_id>/general`

| Action | Selector |
|---|---|
| "Generate Token and Scopes" | `button.c-button.c-button--outline.c-button--medium.margin_top_150` |
| Token name input | `input[name="app_level_tokens_generate_modal_description"]` |
| "Add Scope" button | `button[data-qa="app_scopes_list_add_oauth_scope"]` |
| Scope search combobox | `input[role="combobox"][aria-label="Select Scopes"]` |
| Scope option (after typing) | `[data-qa="app_scopes_picker_option_0"]` — but DO NOT click; press Enter to commit selection |
| "Generate" button (modal) | `button.c-button.c-button--primary.c-button--medium` |
| Copy app token (xapp-) (post-generate modal) | `button.p-app_level_tokens_info__input_button` |
| Dismiss "Got it!" tutorial popup if it appears | the button containing literal text "Got it!" |
```

- [ ] **Step 4: Verify the plugin loads (or document if it can't be tested locally)**

```bash
ls .claude-plugin/plugin.json skills/slackline-provision-bot/SKILL.md skills/slackline-provision-bot/copy-buttons.md
```

If you have access to a Claude Code plugin install path that picks up local plugin dirs, install and verify the skill appears. Otherwise note that the skill's discoverability is implicit on plugin marketplace publish.

- [ ] **Step 5: Commit**

```bash
git add .claude-plugin skills
git commit -m "docs: ship slackline as a Claude Code plugin with provision-bot skill

.claude-plugin/plugin.json declares slackline as a plugin so the new
skills/slackline-provision-bot/SKILL.md is discoverable by agents. The skill
documents the end-to-end agentic recipe (provision → drive browser → init)
with the exact CSS selectors verified during 2026-04-25 deployment of the
slackline-claude bot. copy-buttons.md is the selector reference, kept
separate so it can be updated when Slack drifts their admin UI."
```

---

## Task 14: Update existing slackline-related skills

**Why next:** The user said "all slackline-related skills you can find anywhere". Two skills exist in the cache:
- `~/.claude/plugins/cache/primeradiant/primeradiant-ops/.../skills/slack-messaging/SKILL.md`
- `~/.claude/plugins/cache/superpowers-marketplace/superpowers-lab/.../skills/slack-messaging/SKILL.md` (already deprecated per skill list)

The cache copies are read-only snapshots. Editing in place won't survive a plugin update. We need to find the source repos.

**Files:**
- Modify: source repo for `primeradiant-ops:slackline` skill
- Modify (or just deprecate-with-pointer): source repo for `superpowers-lab:slack-messaging` skill

### Steps

- [ ] **Step 1: Locate the source repo for `primeradiant-ops:slackline`**

```bash
# Likely candidates (run all):
ls ~/prime-radiant 2>/dev/null
find ~/prime-radiant -type d -name 'slack-messaging' 2>/dev/null | head -5
find ~/git -type d -name 'slack-messaging' -path '*primeradiant*' 2>/dev/null | head -5
```

If found at e.g. `~/prime-radiant/cc-plugin-primeradiant-ops/skills/slack-messaging/SKILL.md`, edit there. If not found, ask the user where the source lives — do NOT edit the cache.

- [ ] **Step 2: Read the current SKILL.md**

```bash
cat <SOURCE_PATH>/SKILL.md
```

- [ ] **Step 3: Update the skill with new commands and event shape**

Add a section describing the new commands and the breaking changes. The diff is roughly:

- Document `slackline react add/remove`.
- Document `slackline download --file ID --out PATH` (and `--out -` for stdout).
- Document `slackline send --attach PATH` (repeatable).
- Document `slackline listen` flag modes (`--threads`, `--all-messages`, `--include-bot-self`).
- Document the event-type rename: `reaction` → `reaction_added`. New event types: `reaction_removed`, `thread_reply`, `channel_message`. New `files` array on all message events.
- Document the `slackline create` removal: replaced by `slackline provision bootstrap` + `slackline provision <name>` + `slackline init`.

Concrete diff is left to the implementer because the existing SKILL.md content is unknown until Step 2; preserve its overall structure and tone.

- [ ] **Step 4: Locate the source repo for `superpowers-lab:slack-messaging`**

```bash
find ~ -type d -name 'slack-messaging' -path '*superpowers-lab*' 2>/dev/null | grep -v cache | head -5
```

- [ ] **Step 5: Update or deprecate**

If found: apply the same diff as Step 3, OR replace the SKILL.md content with a clear pointer:

```markdown
# Deprecated

This skill is deprecated. Use `primeradiant-ops:slackline` (or the slackline plugin's `slackline-provision-bot` skill for deploying new bots).
```

If not found, this step is a no-op and worth noting in the commit message.

- [ ] **Step 6: Commit (in the appropriate source repo, NOT this repo)**

```bash
# In each source repo:
git add skills/slack-messaging/SKILL.md
git commit -m "docs(slackline): update commands and event shapes

- Adds react/download/--attach docs
- Documents listener flag modes
- Notes reaction → reaction_added rename
- Notes create → provision migration"
```

If editing in the source repos isn't possible from this session, document the changes you would have made in a follow-up note for the user.

---

## Task 15: Update `CLAUDE.md` and `README.md` in this repo

**Why last:** All command surfaces are final. Docs reference what now exists.

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

### Steps

- [ ] **Step 1: Read existing CLAUDE.md and README.md**

```bash
cat CLAUDE.md README.md
```

- [ ] **Step 2: Update CLAUDE.md**

Modifications:

- In the **Commands** section, add `golangci-lint run ./...` (already there) — no changes needed.
- In the **Architecture / Package roles** section:
  - Add `cmd/provision.go` (provision + bootstrap subcommands).
  - Add `cmd/react.go` (react add/remove).
  - Add `cmd/download.go` (file download).
  - Add `slack/files.go` (CompleteUploadExternal multi-file batched upload).
  - Update `cmd/create.go` description to "migration stub returning errs.Usage with a pointer to provision".
  - Update `listen/` description to enumerate all event types: `mention`, `dm`, `reaction_added`, `reaction_removed`, `thread_reply`, `channel_message`. Note that the latter two are gated by `--threads` / `--all-messages` flags.
- In the **Two config files** section: no change.
- In the **Admin vs user flow** section:
  - Replace `slackline create` description with `slackline provision bootstrap` + `slackline provision <name>` (machine-readable JSON output, no interactive prompts in the per-bot subcommand).
  - Mention the `slackline-provision-bot` skill as the agentic recipe documentation.
- In the **Testing approach** section:
  - Note that `cmd.fakeSlackAPI` now stubs reactions, file upload/download, and CompleteUploadExternal.
  - Note that `provision/manifest_test.go` has a golden-file regression test pinning the exact scope and event set.
- In the **slack-go API quirks** section:
  - Add: "`UploadFileV2` works for single files only; multi-file batched shares require the raw `files.completeUploadExternal` endpoint, wrapped in `slack/files.go`."
  - Add: "`MessageEvent.Files` carries attached files; `subtype: file_share` covers files-without-text."

- [ ] **Step 3: Update README.md**

Modifications:

- Refresh the command list to show the current surface.
- Add a "Provisioning a new bot" section pointing at the `slackline-provision-bot` skill (or a brief inline recipe similar to the skill's content).
- Add an "Event reference" section enumerating all JSONL event types and their fields.
- Document the breaking changes (`reaction` → `reaction_added`, `create` → `provision`) in a "Migration" section.

- [ ] **Step 4: Build + tests pass (sanity)**

```bash
go build ./... && go test ./...
```

Expected: green (no code touched in this task).

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: refresh CLAUDE.md and README.md for new commands and event shape

Documents react/download/provision commands, listener flag modes,
files array on events, and the create→provision migration."
```

---

## Self-review

The plan was reviewed against `docs/specs/2026-04-25-slackline-threading-reactions-attachments-design.md` (commit `6686c0b`) for spec coverage.

Spec coverage check:

| Spec section | Plan task(s) |
|---|---|
| Architecture / file map | Task 1–15 (all touched files mapped) |
| Listener flags & event schemas | Tasks 7, 10, 11, 12 |
| `files` array on receive | Task 11 |
| Files-without-text via `subtype: file_share` | Task 11 (handler accepts it) |
| `reaction_added`/`reaction_removed` | Task 7 |
| `slackline send --attach` | Task 9 |
| `slackline react` | Task 6 |
| `slackline download` | Task 8 |
| `slackline provision` (and bootstrap) | Task 4 |
| `slackline create` removal | Task 5 |
| `slack.SlackAPI` interface additions | Task 2 |
| `CompleteUploadExternal` (slack/files.go) | Task 3 |
| Manifest changes (scopes, events, parameterization) | Task 1 |
| Skills + docs (plugin manifest, new SKILL.md, copy-buttons.md, primeradiant + lab updates, CLAUDE.md, README.md) | Tasks 13, 14, 15 |
| Testing strategy (golden manifest, httptest for HTTP, fakeSlackAPI for unit) | Task 1 (golden), Tasks 3/4/8 (httptest), Tasks 6/9 (fake) |
| Backwards compatibility | Documented in Tasks 5, 7, 15 |

No spec section is left unimplemented.

Placeholder check: every code block in the plan contains complete, runnable code or commands. No "TBD" / "TODO" / "implement later". The single exception is Task 14 Step 3, where the SKILL.md content diff cannot be made concrete without first reading the current SKILL.md from the source repo (which the implementer locates in Step 1) — this is a structural unknown, not laziness.

Type / signature consistency check:

- `slack.FileUpload {Path, Title}` is defined in Task 2 (slack/api.go) and used in Tasks 3 (slack/files.go), 6 (cmd/react.go does NOT use it — correct, reactions don't), 9 (cmd/send.go).
- `listen.FileMeta` is defined in Task 11 (listen/events.go) and used by `Event.Files` in Tasks 11/12.
- `cmd.fakeSlackAPI` fields `reactionsAdded`, `reactionsRemoved`, `addReactionErr`, `removeReactionErr`, `completeUploadResp`, `lastCompleteUploadCall` are added in Task 2 and consumed in Task 6 (react tests) and Task 9 (send tests).
- `runReactAddWithAPI` / `runReactRemoveWithAPI` / `runDownloadWithAPI` / `runDownloadWithAPIWriter` / `runSendWithAPI` / `runProvisionWithDeps` / `runProvisionBootstrapWithDeps` / `runCreateRemoved` — all introduced in their respective tasks and only referenced in their own tests.
- `listen.ListenerOptions` introduced in Task 12; `cmd/listen.go` updated in the same task to pass it. Task 10's `NewListener(..., includeBotSelf bool, ...)` is an interim signature that Task 12 supersedes — the implementer must update `listen/listener_test.go` test setups when going from Task 10 → Task 12, but since the tests use struct literals (`&Listener{...}`) rather than `NewListener` directly, only `cmd/listen.go` needs the signature swap.

All consistent.
