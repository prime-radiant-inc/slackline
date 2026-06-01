# Cut jq friction + consolidate Slack skills — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce the `jq`/exit-code friction the `using-slack` skill exposed — add `listen --type`, collapse the two reaction events into one `reaction` event with an `action` field, give `ask` a distinct timeout exit code — then rewrite/consolidate the Slack skills, and release v0.3.0.

**Architecture:** Three small, independent binary changes in `listen/`, `cmd/`, and `errs/`, each behind the existing fake-based command/listener tests. Then a docs + skill-restructure pass, then a standard `make release`. No package restructuring.

**Tech Stack:** Go (cobra CLI, slack-go), `golangci-lint`/`gofumpt`, lefthook hooks, Markdown skills, Keep-a-Changelog.

**Spec:** `docs/specs/2026-06-01-cut-jq-friction-and-consolidate-skills-design.md`

**Branch:** `jesse/pri-2017-cut-jq-friction` (already exists, spec committed there).

---

## File Structure

- `errs/errors.go` — add `Timeout = 5` constant. `errs/errors_test.go` (new or extend) — assert the value.
- `listen/events.go` — replace the two reaction constants with `EventTypeReaction`; add `Action` field to `Event`.
- `listen/listener.go` — emit unified reaction events; add `types` filter field + filter in `emit()`; `ListenerOptions.Types`; wire in `NewListener`.
- `listen/events_test.go`, `listen/listener_test.go` — update reaction tests; add emit-filter test.
- `cmd/listen.go` — `--type` flag, `parseListenTypes` validation (runs first), wire `Types`, help text.
- `cmd/listen_test.go` (new) — `parseListenTypes` unit tests.
- `cmd/ask.go` — extract `runAskWithAPI` with injected `now`/`sleep`; timeout returns `errs.Timeout`; help text.
- `cmd/ask_test.go` (new) — timeout→exit-5 and reply→0 via fakes.
- `README.md`, `CLAUDE.md`, `AGENTS.md` — doc reconciliation.
- `skills/using-slack/SKILL.md` (rewrite), `skills/using-slack/provisioning.md` (moved), `skills/using-slack/copy-buttons.md` (moved); delete `skills/slackline-provision-bot/`.
- `.claude-plugin/marketplace.json` — description string.
- `cc-plugin-primeradiant-ops/skills/using-slack-at-prime-radiant/SKILL.md` — cross-repo reference.
- `CHANGELOG.md`, `.claude-plugin/plugin.json` — release.

---

## Task 1: `errs.Timeout = 5` exit code

**Files:**
- Modify: `errs/errors.go:9-15`
- Test: `errs/errors_test.go`

- [ ] **Step 1: Write the failing test**

Append to `errs/errors_test.go` (create the file with `package errs` + `import "testing"` if it does not exist):

```go
func TestTimeoutExitCode(t *testing.T) {
	if Timeout != 5 {
		t.Errorf("Timeout = %d, want 5", Timeout)
	}
	// Guard the rest of the taxonomy so the codes stay stable.
	if Success != 0 || SlackAPI != 1 || Auth != 2 || Config != 3 || Usage != 4 {
		t.Error("exit-code taxonomy 0-4 changed unexpectedly")
	}
}
```

(If `errs/errors_test.go` already exists, add only the `TestTimeoutExitCode` function and drop the redundant `import`/`package` lines.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./errs/ -run TestTimeoutExitCode -v`
Expected: FAIL — `undefined: Timeout` (compile error).

- [ ] **Step 3: Add the constant**

In `errs/errors.go`, change the const block:

```go
const (
	Success  = 0
	SlackAPI = 1
	Auth     = 2
	Config   = 3
	Usage    = 4
	Timeout  = 5
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./errs/ -run TestTimeoutExitCode -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add errs/errors.go errs/errors_test.go
git commit -m "feat(errs): add Timeout=5 exit code (PRI-2017)"
```

---

## Task 2: Collapse reaction events into a unified `reaction` event

The listener emits `reaction_added`/`reaction_removed` as separate `type` values. Collapse to one `reaction` type with an `action` field. Removing the old constants breaks compilation of `listener.go`, `events_test.go`, and `listener_test.go`, so all change together.

**Files:**
- Modify: `listen/events.go:7-28`
- Modify: `listen/listener.go:206-228`
- Modify: `listen/events_test.go:56-94`
- Modify: `listen/listener_test.go:276-335`

- [ ] **Step 1: Update the events_test reaction tests to the unified shape (red)**

Replace `TestReactionAddedEvent_JSON` and `TestReactionRemovedEvent_JSON` in `listen/events_test.go` with one test:

```go
func TestReactionEvent_JSON(t *testing.T) {
	for _, action := range []string{"added", "removed"} {
		e := Event{Type: EventTypeReaction, Action: action, Channel: fixtureChannelID, User: fixtureUserID, Emoji: fixtureEmojiEyes, ItemTS: fixtureMessageTS}
		data, _ := json.Marshal(e)
		var got map[string]interface{}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got["type"] != EventTypeReaction {
			t.Errorf("type = %v, want %s", got["type"], EventTypeReaction)
		}
		if got["action"] != action {
			t.Errorf("action = %v, want %s", got["action"], action)
		}
		if got["emoji"] != fixtureEmojiEyes {
			t.Errorf("emoji = %v, want %s", got["emoji"], fixtureEmojiEyes)
		}
		if got["item_ts"] != fixtureMessageTS {
			t.Errorf("item_ts = %v", got["item_ts"])
		}
		if _, ok := got["text"]; ok {
			t.Error("reaction event should not have text")
		}
	}
}
```

- [ ] **Step 2: Update the listener_test reaction assertions (still red — constants undefined)**

In `listen/listener_test.go`:
- In `TestHandleEventsAPI_Reaction` (~:282), change the assertion block to:

```go
	if m["type"] != EventTypeReaction {
		t.Errorf("type = %v, want reaction", m["type"])
	}
	if m["action"] != "added" {
		t.Errorf("action = %v, want added", m["action"])
	}
```

- In `TestHandleEventsAPI_ReactionRemoved` (~:328), change to:

```go
	if m["type"] != EventTypeReaction {
		t.Errorf("type = %v, want reaction", m["type"])
	}
	if m["action"] != "removed" {
		t.Errorf("action = %v, want removed", m["action"])
	}
```

(Leave the `emoji`/`item_ts`/`channel` assertions in those tests unchanged.)

- [ ] **Step 3: Run to verify the listen package fails to compile**

Run: `go test ./listen/ 2>&1 | head`
Expected: FAIL — `undefined: EventTypeReaction` (and the old constants still referenced by `listener.go`).

- [ ] **Step 4: Update `events.go` — constant + Action field**

In `listen/events.go`, replace the two reaction constants and add `Action`:

```go
const (
	EventTypeMention        = "mention"
	EventTypeDM             = "dm"
	EventTypeThreadReply    = "thread_reply"
	EventTypeChannelMessage = "channel_message"
	EventTypeReaction       = "reaction"
)

// Event represents a Slack event to be serialized as JSONL output.
type Event struct {
	Type         string     `json:"type"`
	Action       string     `json:"action,omitempty"`
	Channel      string     `json:"channel"`
	User         string     `json:"user,omitempty"`
	Text         string     `json:"text,omitempty"`
	TS           string     `json:"ts,omitempty"`
	ThreadTS     string     `json:"thread_ts,omitempty"`
	Emoji        string     `json:"emoji,omitempty"`
	ItemTS       string     `json:"item_ts,omitempty"`
	ParentUserID string     `json:"parent_user_id,omitempty"`
	Files        []FileMeta `json:"files,omitempty"`
}
```

- [ ] **Step 5: Update `listener.go` — both reaction branches**

In `listen/listener.go`, replace the two reaction emit blocks (`:206-228`):

```go
	case *slackevents.ReactionAddedEvent:
		if l.shouldFilterSelf(ev.User) {
			return // Self-filter
		}
		l.emit(Event{
			Type:    EventTypeReaction,
			Action:  "added",
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})

	case *slackevents.ReactionRemovedEvent:
		if l.shouldFilterSelf(ev.User) {
			return
		}
		l.emit(Event{
			Type:    EventTypeReaction,
			Action:  "removed",
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})
	}
```

- [ ] **Step 6: Run the listen tests to verify green**

Run: `go test ./listen/ -v 2>&1 | tail -20`
Expected: PASS (all listen tests, including the rewritten reaction tests).

- [ ] **Step 7: Commit**

```bash
git add listen/events.go listen/listener.go listen/events_test.go listen/listener_test.go
git commit -m "feat(listen): unify reaction_added/removed into reaction event with action field (PRI-2017)"
```

---

## Task 3: `listen --type` emit-time filter (listener side)

Add the allowlist filter to the listener: a `types` set; `emit()` drops non-matching events.

**Files:**
- Modify: `listen/listener.go:17-51` (struct, options, constructor), `:232-242` (emit)
- Test: `listen/listener_test.go`

- [ ] **Step 1: Write the failing emit-filter test**

Append to `listen/listener_test.go`:

```go
func TestEmit_TypeFilter(t *testing.T) {
	buf := &bytes.Buffer{}
	l := &Listener{
		botUserID: testBotUserID,
		out:       buf,
		status:    &bytes.Buffer{},
		types:     map[string]bool{EventTypeMention: true},
	}
	l.emit(Event{Type: EventTypeMention, Channel: testChannelID})
	l.emit(Event{Type: EventTypeReaction, Action: "added", Channel: testChannelID})

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1 (only mention)", len(lines))
	}
	if lines[0]["type"] != EventTypeMention {
		t.Errorf("emitted type = %v, want mention", lines[0]["type"])
	}
}

func TestEmit_NoTypeFilter_EmitsAll(t *testing.T) {
	l, buf := newTestListener() // types is nil
	l.emit(Event{Type: EventTypeMention, Channel: testChannelID})
	l.emit(Event{Type: EventTypeReaction, Action: "removed", Channel: testChannelID})
	if got := len(parseJSONL(t, buf)); got != 2 {
		t.Fatalf("got %d events, want 2 (no filter)", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./listen/ -run TestEmit_TypeFilter -v`
Expected: FAIL — `unknown field 'types' in struct literal`.

- [ ] **Step 3: Add the `types` field, option, constructor wiring, and emit filter**

In `listen/listener.go`, add to the `Listener` struct (after `allMessages bool`):

```go
	types          map[string]bool
```

Add to `ListenerOptions` (after `AllMessages bool`):

```go
	Types          map[string]bool
```

In `NewListener`, add to the returned `&Listener{...}` literal (after `allMessages: opts.AllMessages,`):

```go
		types:          opts.Types,
```

Change `emit` to filter first:

```go
func (l *Listener) emit(e Event) {
	if l.types != nil && !l.types[e.Type] {
		return
	}
	// Strip thread_ts when empty or equals ts (top-level message, not a reply)
	if e.ThreadTS == "" || e.ThreadTS == e.TS {
		e.ThreadTS = ""
	}
	data, err := json.Marshal(e)
	if err != nil {
		return // Should never happen with simple structs
	}
	_, _ = fmt.Fprintln(l.out, string(data))
}
```

- [ ] **Step 4: Run to verify green**

Run: `go test ./listen/ -v 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add listen/listener.go listen/listener_test.go
git commit -m "feat(listen): add emit-time event-type filter to Listener (PRI-2017)"
```

---

## Task 4: `listen --type` flag, validation, and help (command side)

Add the flag, validate it FIRST in `runListen` (before `loadConfig`/`AuthTest`), and wire it into `ListenerOptions`.

**Files:**
- Modify: `cmd/listen.go`
- Test: `cmd/listen_test.go` (new)

- [ ] **Step 1: Write the failing validation test**

Create `cmd/listen_test.go`:

```go
package cmd

import (
	"errors"
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
)

func TestParseListenTypes(t *testing.T) {
	t.Run("empty returns nil (emit all)", func(t *testing.T) {
		set, err := parseListenTypes("", false)
		if err != nil || set != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", set, err)
		}
	})

	t.Run("valid types", func(t *testing.T) {
		set, err := parseListenTypes("mention, reaction", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !set["mention"] || !set["reaction"] || len(set) != 2 {
			t.Fatalf("set = %v", set)
		}
	})

	t.Run("unknown type is a usage error", func(t *testing.T) {
		_, err := parseListenTypes("mentions", false)
		var se *errs.SlackError
		if !errors.As(err, &se) || se.Code != errs.Usage {
			t.Fatalf("err = %v, want Usage SlackError", err)
		}
	})

	t.Run("channel_message requires --all-messages", func(t *testing.T) {
		_, err := parseListenTypes("channel_message", false)
		var se *errs.SlackError
		if !errors.As(err, &se) || se.Code != errs.Usage {
			t.Fatalf("err = %v, want Usage SlackError", err)
		}
		if _, err := parseListenTypes("channel_message", true); err != nil {
			t.Fatalf("with --all-messages: unexpected error %v", err)
		}
	})
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/ -run TestParseListenTypes -v`
Expected: FAIL — `undefined: parseListenTypes`.

- [ ] **Step 3: Add the flag, the parser, validation ordering, and wiring**

In `cmd/listen.go`:

Add to the `var (...)` block:

```go
	listenTypes          string
```

Add to `init()` (with the other flags):

```go
	listenCmd.Flags().StringVar(&listenTypes, "type", "", "comma-separated event types to emit: mention, dm, thread_reply, channel_message, reaction (default: all). Emit-time filter — does not widen subscription; channel_message requires --all-messages")
```

Update the command `Long` to mention `--type`:

```go
	Long:  "Connect via Socket Mode and stream events as JSONL to stdout. Use --type to emit only specific event types (mention, dm, thread_reply, channel_message, reaction).",
```

Add the parser (package-level, e.g. above `runListen`):

```go
var validListenTypes = map[string]bool{
	listen.EventTypeMention:        true,
	listen.EventTypeDM:             true,
	listen.EventTypeThreadReply:    true,
	listen.EventTypeChannelMessage: true,
	listen.EventTypeReaction:       true,
}

// parseListenTypes turns the comma-separated --type value into an allowlist set.
// Empty input returns (nil, nil) meaning "emit all". Unknown types and the
// channel_message-without-firehose case are usage errors.
func parseListenTypes(raw string, allMessages bool) (map[string]bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	set := map[string]bool{}
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !validListenTypes[t] {
			return nil, &errs.SlackError{Code: errs.Usage, Err: "invalid_type", Detail: fmt.Sprintf("unknown --type %q; valid: mention, dm, thread_reply, channel_message, reaction", t)}
		}
		set[t] = true
	}
	if set[listen.EventTypeChannelMessage] && !allMessages {
		return nil, &errs.SlackError{Code: errs.Usage, Err: "invalid_type", Detail: "channel_message events require --all-messages"}
	}
	return set, nil
}
```

Add the imports `"fmt"` and `"strings"` to `cmd/listen.go` (it currently imports only `"os"`, the project packages, and cobra).

In `runListen`, make validation the FIRST statement (before `loadConfig()`):

```go
func runListen(cmd *cobra.Command, args []string) error {
	types, err := parseListenTypes(listenTypes, listenAllMessages)
	if err != nil {
		return err
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeConfigError, Detail: err.Error()}
	}
	// ... unchanged through AuthTest ...
```

And pass `Types` into the options literal:

```go
	listener := listen.NewListener(cfg.Bot.BotToken, cfg.Bot.AppToken, authResp.UserID, listen.ListenerOptions{
		IncludeBotSelf: listenIncludeBotSelf,
		Threads:        listenThreads || listenAllMessages,
		AllMessages:    listenAllMessages,
		Types:          types,
	}, os.Stdout, os.Stderr)
```

- [ ] **Step 4: Run to verify green**

Run: `go test ./cmd/ -run TestParseListenTypes -v`
Expected: PASS.

- [ ] **Step 5: Build to confirm the command compiles**

Run: `go build ./... && go vet ./...`
Expected: no output (success).

- [ ] **Step 6: Commit**

```bash
git add cmd/listen.go cmd/listen_test.go
git commit -m "feat(listen): add --type allowlist flag with up-front validation (PRI-2017)"
```

---

## Task 5: `ask` timeout exit code + testability seam

Extract `runAskWithAPI` (injected `now`/`sleep`), return `errs.Timeout` on timeout, and add the missing test file.

**Files:**
- Modify: `cmd/ask.go`
- Test: `cmd/ask_test.go` (new)

- [ ] **Step 1: Write the failing tests**

Create `cmd/ask_test.go`:

```go
package cmd

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/prime-radiant-inc/slackline/errs"
)

func TestRunAskWithAPI_Timeout(t *testing.T) {
	api := &fakeSlackAPI{} // no replies queued
	// Clock that jumps past the deadline after the first (deadline-computing) call.
	calls := 0
	base := time.Unix(1_000_000, 0)
	now := func() time.Time {
		calls++
		if calls == 1 {
			return base
		}
		return base.Add(time.Hour)
	}
	err := runAskWithAPI(api, "C123", "UBOT", "hi", 300, 10, now, func(time.Duration) {}, &bytes.Buffer{})
	var se *errs.SlackError
	if !errors.As(err, &se) || se.Code != errs.Timeout {
		t.Fatalf("err = %v, want Timeout SlackError", err)
	}
}

func TestRunAskWithAPI_Reply(t *testing.T) {
	api := &fakeSlackAPI{
		repliesMessages: []goslack.Message{makeMessage("200.1", "U_other", "here you go")},
	}
	base := time.Unix(1_000_000, 0)
	now := func() time.Time { return base } // never advances; deadline never reached
	out := &bytes.Buffer{}
	err := runAskWithAPI(api, "C123", "UBOT", "hi", 300, 10, now, func(time.Duration) {}, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("here you go")) {
		t.Fatalf("reply not written to out: %q", out.String())
	}
}
```

Note: `fakeSlackAPI`, `makeMessage`, and the `goslack` import alias already exist in `cmd/helpers_test.go`. The fake's `PostMessage` returns an empty `ts`, so the reply fixture (`ts="200.1"`, user `U_other`) is neither the parent (`""`) nor the bot (`UBOT`) and is emitted.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/ -run TestRunAskWithAPI -v`
Expected: FAIL — `undefined: runAskWithAPI`.

- [ ] **Step 3: Refactor `runAsk` into a wrapper + `runAskWithAPI`**

In `cmd/ask.go`, replace the body of `runAsk` from the `api := slackpkg.NewClient(...)` line through the end of the polling loop. Keep the stdin/message handling and config/token checks in the wrapper; move PostMessage + poll loop into `runAskWithAPI`.

The wrapper tail (after the `cfg.Bot.BotToken == ""` check) becomes:

```go
	api := slackpkg.NewClient(cfg.Bot.BotToken)

	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: errs.CodeAuthTestFailed, Detail: err.Error()}
	}

	resolver := slackpkg.NewResolver(api)
	channelID, err := resolver.Resolve(askChannel)
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "channel_resolve_error", Detail: err.Error()}
	}

	return runAskWithAPI(api, channelID, authResp.UserID, text, askTimeout, askPoll, time.Now, time.Sleep, cmd.OutOrStdout())
}

// runAskWithAPI posts text to channelID, then polls the thread until a reply
// from another user arrives (exit 0) or the deadline passes (Timeout). now/sleep
// are injected for deterministic tests; production passes time.Now/time.Sleep.
func runAskWithAPI(api slackpkg.SlackAPI, channelID, botUserID, text string, timeoutSec, pollSec int, now func() time.Time, sleep func(time.Duration), out io.Writer) error {
	_, ts, err := api.PostMessage(channelID, goslack.MsgOptionText(text, false))
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "send_failed", Detail: err.Error()}
	}

	deadline := now().Add(time.Duration(timeoutSec) * time.Second)
	pollInterval := time.Duration(pollSec) * time.Second

	for {
		sleep(pollInterval)
		if now().After(deadline) {
			return &errs.SlackError{Code: errs.Timeout, Err: "timeout", Detail: fmt.Sprintf("No reply received within %d seconds.", timeoutSec)}
		}

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

		var replies []goslack.Message
		for _, m := range msgs {
			if m.Timestamp == ts {
				continue // skip parent
			}
			if m.User == botUserID {
				continue // skip bot's own
			}
			replies = append(replies, m)
		}

		if len(replies) > 0 {
			for _, m := range replies {
				writeMessage(out, m)
			}
			return nil
		}
	}
}
```

- [ ] **Step 4: Update the `ask` Long help text**

In `cmd/ask.go`, change the command `Long`:

```go
	Long:  "Sends a message to a channel and polls the thread for replies from other users. Exits 0 when a reply is received, exits 5 on timeout (1/2/3 for API/auth/config errors).",
```

- [ ] **Step 5: Run to verify green**

Run: `go test ./cmd/ -run TestRunAskWithAPI -v`
Expected: PASS (both subtests). The timeout test must complete instantly (no wall-clock wait).

- [ ] **Step 6: Commit**

```bash
git add cmd/ask.go cmd/ask_test.go
git commit -m "feat(ask): distinct exit 5 on timeout + injectable seam for tests (PRI-2017)"
```

---

## Task 6: Full test/vet/lint gate

- [ ] **Step 1: Run the whole suite + vet + lint**

Run:
```bash
go test ./... 2>&1 | tail -20
go vet ./...
golangci-lint run ./...
```
Expected: all packages `ok`; vet silent; `0 issues.`

- [ ] **Step 2: If anything fails, fix it before continuing.** Do not proceed to docs/release with a red tree.

---

## Task 7: Documentation reconciliation (README, CLAUDE.md, AGENTS.md)

Pure text edits — no code. Make each replacement exactly.

**Files:** `README.md`, `CLAUDE.md`, `AGENTS.md`

- [ ] **Step 1: README — `ask` exits line (`:100`)**

Replace `**exits 1 on timeout**` with `**exits 5 on timeout**` in the `ask` section sentence.

- [ ] **Step 2: README — listen usage synopsis (`:105`)**

```
slackline listen [--type mention,dm,...] [--threads] [--all-messages] [--include-bot-self]
```

- [ ] **Step 3: README — listen flags table (`:110-115`)**

Replace the table with:

```markdown
| Flag | Effect |
|------|--------|
| (none) | `mention`, `dm`, `reaction` only |
| `--type <list>` | emit only the named types (`mention`, `dm`, `thread_reply`, `channel_message`, `reaction`); emit-time filter, does not widen subscription; `channel_message` requires `--all-messages` |
| `--threads` | also emits `thread_reply` for threads the bot has participated in |
| `--all-messages` | firehose: every message in every channel the bot is in (implies `--threads`) |
| `--include-bot-self` | do not filter out events from the bot's own user ID |
```

- [ ] **Step 4: README — Exit Codes table (`:180-188`)**

Add a row after the `| 4 | … |` row:

```markdown
| 5 | Timeout (`ask` received no reply) |
```

- [ ] **Step 5: README — reaction event reference (`:266-276`)**

Replace the two `### reaction_added` / `### reaction_removed` sections with one:

```markdown
### reaction

Emitted when a reaction is added or removed. `action` is `added` or `removed`.

```json
{"type":"reaction","action":"added","channel":"C...","user":"U...","emoji":"thumbsup","item_ts":"..."}
```
```

- [ ] **Step 6: README — Migration section (`:286-290`)**

Replace the `### reaction → reaction_added` entry with a reconciled, current entry:

```markdown
### reaction_added / reaction_removed → reaction

The split `reaction_added` / `reaction_removed` listen events (introduced in 0.2.0) are unified back into a single `reaction` event carrying an `action` field (`"added"` | `"removed"`). Update listeners that match `"type":"reaction_added"` / `"reaction_removed"` to match `"type":"reaction"` and branch on `action`.
```

- [ ] **Step 7: CLAUDE.md and AGENTS.md — listen event-type list (`:35`)**

In both files, in the `listen/` bullet, replace `reaction_added`, `reaction_removed` with the unified description. Change the fragment:

`Event types: \`mention\`, \`dm\`, \`reaction_added\`, \`reaction_removed\`, \`thread_reply\` …`

to:

`Event types: \`mention\`, \`dm\`, \`reaction\` (with an \`action\` field, \`added\`/\`removed\`), \`thread_reply\` …`

- [ ] **Step 8: CLAUDE.md and AGENTS.md — provision skill path (`:55`)**

In both files, change `skills/slackline-provision-bot/SKILL.md` to `skills/using-slack/provisioning.md`, and the phrase "lives in the `slackline-provision-bot` skill" to "lives in the `using-slack` skill's `provisioning.md`". (Leave the `.claude-plugin/` vs `.Codex-plugin/` token as-is in each file.)

- [ ] **Step 9: Verify no stale references remain**

Run:
```bash
grep -rn "reaction_added\|reaction_removed" README.md CLAUDE.md AGENTS.md
grep -rn "slackline-provision-bot" README.md CLAUDE.md AGENTS.md
```
Expected: README/CLAUDE/AGENTS show no `reaction_added`/`reaction_removed` except the Migration entry's description of the old names; `slackline-provision-bot` only in the README "Provisioning a new bot" section (fixed in Task 8) — note any remaining for Task 8.

- [ ] **Step 10: Commit**

```bash
git add README.md CLAUDE.md AGENTS.md
git commit -m "docs: reconcile reaction event + exit codes + listen --type (PRI-2017)"
```

---

## Task 8: Skill restructure (fold provisioning into using-slack)

**Files:**
- Move: `skills/slackline-provision-bot/SKILL.md` → `skills/using-slack/provisioning.md`
- Move: `skills/slackline-provision-bot/copy-buttons.md` → `skills/using-slack/copy-buttons.md`
- Delete: `skills/slackline-provision-bot/`
- Rewrite: `skills/using-slack/SKILL.md`
- Modify: `README.md:200,228`, `.claude-plugin/marketplace.json:12`
- Modify (cross-repo): `cc-plugin-primeradiant-ops/skills/using-slack-at-prime-radiant/SKILL.md:43`

- [ ] **Step 1: Move the provisioning files**

```bash
git mv skills/slackline-provision-bot/copy-buttons.md skills/using-slack/copy-buttons.md
git mv skills/slackline-provision-bot/SKILL.md skills/using-slack/provisioning.md
rmdir skills/slackline-provision-bot
```

- [ ] **Step 2: Convert `provisioning.md` from a skill to a supporting doc**

Edit `skills/using-slack/provisioning.md`: delete the YAML frontmatter block (the `---` … `---` at the top). Keep the body. Update its first line to read: `# Provisioning a new slackline bot` and ensure the `copy-buttons.md` reference still resolves (it's now a sibling — the existing relative reference "see `copy-buttons.md` next to this file" remains correct).

- [ ] **Step 3: Rewrite `skills/using-slack/SKILL.md`**

Replace the entire file with:

```markdown
---
name: using-slack
description: Use when sending, reading, or reacting to Slack messages, watching for bot events (mentions, DMs, reactions, thread replies), downloading Slack files, or provisioning/creating/deploying a new Slack bot — all from the command line via the slackline CLI.
allowed-tools: Bash(slackline:*)
---

# Using Slack via slackline

`slackline` gives an AI agent its own Slack identity. One binary, one config file, one bot. Messages appear from the bot, not from a human.

## Prerequisites

- `slackline` on PATH (`~/.local/bin/slackline`). Install: see the repo README.
- A configured bot: `slackline auth status` should print `Bot:` and a `(valid)` token. If not, run `slackline init` (needs already-provisioned `xoxb-`/`xapp-` tokens). To create a brand-new bot, see the Provisioning section below.
- `jq` only if you want to reshape output — most tasks no longer need it.

## The membership gotcha (read this first)

The bot can only `read`, `ask`, and `listen` in channels it has **joined**. Otherwise these fail with `{"error":"not_in_channel"}`. `slackline channels` lists the channels it's in. Fix: a human runs `/invite @bot-name` in the target channel. (Posting with `send` to a public channel often works without joining; reading never does.)

`--channel` accepts a name (`#ops`), an ID (`C...`), or a pasted Slack URL.

## Commands

| Task | Command |
|------|---------|
| Post a message | `slackline send --channel '#ops' --message 'text'` |
| Post from stdin | `echo text \| slackline send --channel '#ops'` |
| Reply in a thread | `slackline send --channel '#ops' --thread <ts> --message 'text'` |
| Attach files | `slackline send --channel '#ops' --attach a.png --attach b.pdf` |
| Read recent messages | `slackline read --channel '#ops' --limit 10` |
| Read the single newest message | `slackline read --channel '#ops' --limit 1` |
| Read since a time | `slackline read --channel '#ops' --since 2026-03-17T00:00:00Z` |
| Read a thread (newest reply: `--limit 1`) | `slackline read --channel '#ops' --thread <ts> --limit 20` |
| Ask and wait for a reply | `slackline ask --channel '#ops' --message 'q?' --timeout 120` |
| React to a message | `slackline react add --channel '#ops' --ts <ts> --emoji white_check_mark` |
| List the bot's channels | `slackline channels` (`--all`, `--json`) |
| Download a file | `slackline download --file <F...> --out path` (`--out -` to stdout) |
| Stream live events | `slackline listen` (filter with `--type`) |

Run `slackline <command> --help` for every flag.

## Reading messages

`read` emits JSONL, oldest-first. To get just the newest, use `--limit 1` (it returns the most recent N counted from the newest) — no `tail` needed:

```bash
slackline read --channel '#ops' --limit 1
```

- This holds for threads too: `read --thread <ts> --limit 1` is the newest reply. A thread read includes the parent, which counts toward `--limit`; a line is a real reply when its `ts` differs from the thread parent `ts`.
- The `user` field is a Slack user **ID** (`U...`), not a display name.

## Ask: reply vs. timeout

`ask` posts a message, then polls the thread for a reply from someone other than the bot.

- **Got a reply:** exit `0`, the reply printed as JSONL on stdout.
- **Timed out:** exit `5`, `{"error":"timeout",...}` on stderr.
- Other failures (API/auth/config) use exit `1`/`2`/`3`.

Branch cleanly on the exit code — no stderr parsing:

```bash
if out=$(slackline ask --channel '#ops' --message 'ready?' --timeout 120); then
  echo "reply: $out"
elif [ $? -eq 5 ]; then
  echo "timed out"
else
  echo "error"
fi
```

The poll interval (`--poll`, default 10s) means the wait can overshoot `--timeout` by up to one interval; use `--poll 5` for tighter windows.

## Listening for events

`slackline listen` streams events as JSONL to **stdout**; connection status (`connected`, `reconnecting`, `disconnected`) goes to **stderr**. Requires both bot and app tokens (Socket Mode).

Use `--type` to emit only what you care about — no `jq .type` filter needed:

```bash
slackline listen --type mention | while IFS= read -r line; do
  ch=$(jq -r .channel <<<"$line")
  ts=$(jq -r .ts <<<"$line")
  slackline react add --channel "$ch" --emoji eyes --ts "$ts"
done
```

- Valid `--type` values: `mention`, `dm`, `thread_reply`, `channel_message`, `reaction`. Comma-separate for several (`--type mention,dm`). Unknown values error.
- `--type` is an emit-time filter, not a subscription widener: `--type thread_reply` still only sees the bot's own threads unless you add `--all-messages`. `channel_message` **requires** `--all-messages`.
- Default (no `--type`, no flags): `mention`, `dm`, `reaction`, and bot-parent `thread_reply`. Bot self-events are filtered unless `--include-bot-self`.

A reaction event is a single `reaction` type with an `action` field:

```json
{"type":"reaction","action":"added","channel":"C...","user":"U...","emoji":"thumbsup","item_ts":"..."}
```

`react` is idempotent; `--emoji` takes the bare name (`white_check_mark`, not `:white_check_mark:`).

## Files

A file shows up as a `files` array on an event (`id`/`name`/`mimetype`/`size`/`title`, **no URL**). Download by ID with `slackline download --file <id> --out <path>`. File uploads arrive as a `dm`/`channel_message`/`thread_reply` event (Slack subtype `file_share`), **never as a `mention`** — to catch them in a channel, `listen --all-messages` (optionally `--type channel_message,thread_reply`).

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Slack API error |
| 2 | Auth error (invalid/revoked token) |
| 3 | Config error (missing file/token) |
| 4 | Usage error (bad flags) |
| 5 | Timeout (`ask` received no reply) |

All errors are written to stderr as JSON: `{"error":"...","detail":"..."}`.

## Provisioning a new bot

Creating a brand-new bot is an admin flow (browser automation for Slack's install + token-collection screens). See **`provisioning.md`** next to this file for the end-to-end recipe, and `copy-buttons.md` for the selector reference.

## Common mistakes

- **Reading a channel the bot hasn't joined.** Returns `not_in_channel`; the bot must be invited.
- **Parsing `listen` stdout as if it included status.** Status is on stderr; only stdout is JSONL.
- **Filtering `listen` for `mention` to catch file uploads.** Files come on `message`-family events with `--all-messages`, not on mentions.
- **Passing emoji with colons.** Use the bare name.
- **Matching `reaction_added`/`reaction_removed`.** It's one `reaction` event now — branch on `action`.
```

- [ ] **Step 4: README — Provisioning section (`:200,228`)**

In `README.md`, change both references from the `slackline-provision-bot` skill to the `using-slack` skill's provisioning doc:
- `:200` — "The `slackline-provision-bot` skill (in `skills/slackline-provision-bot/SKILL.md`)" → "The `using-slack` skill's provisioning recipe (in `skills/using-slack/provisioning.md`)".
- `:228` — "The `slackline-provision-bot` skill contains the full browser selector reference…" → "`skills/using-slack/copy-buttons.md` contains the full browser selector reference…".

- [ ] **Step 5: marketplace.json description (`:12`)**

In `.claude-plugin/marketplace.json`, change the plugin `description`: replace "Bundles the using-slack and slackline-provision-bot skills." with "Bundles the using-slack skill (with a provisioning recipe)."

- [ ] **Step 6: Cross-repo skill reference**

In `cc-plugin-primeradiant-ops/skills/using-slack-at-prime-radiant/SKILL.md:43`, change "(to create a new bot, use the `slackline-provision-bot` skill)" to "(to create a new bot, see the Provisioning section of the `using-slack` skill)".

- [ ] **Step 7: Verify no dangling references and that the skill files exist**

Run:
```bash
grep -rn "slackline-provision-bot" README.md CLAUDE.md AGENTS.md .claude-plugin/ skills/ ; echo "---"
ls skills/using-slack/SKILL.md skills/using-slack/provisioning.md skills/using-slack/copy-buttons.md
test ! -e skills/slackline-provision-bot && echo "old skill dir removed"
```
Expected: no `slackline-provision-bot` matches in the live files; all three using-slack files present; old dir gone.

- [ ] **Step 8: Commit (slackline repo)**

```bash
git add -A skills/ README.md .claude-plugin/marketplace.json
git commit -m "refactor(skills): fold slackline-provision-bot into using-slack via progressive disclosure (PRI-2017)"
```

- [ ] **Step 9: Commit + branch the cross-repo change separately**

```bash
cd /Users/jesse/Documents/GitHub/prime-radiant-inc/cc-plugin-primeradiant-ops
git checkout -b jesse/pri-2017-using-slack-provisioning-ref
git add skills/using-slack-at-prime-radiant/SKILL.md
git commit -m "docs(using-slack-at-prime-radiant): point provisioning at using-slack skill (PRI-2017)"
cd -
```

(Coordinate landing: the downstream repo edit and the slackline release should both land before announcing, so no window names a deleted skill.)

---

## Task 9: Release v0.3.0

**Files:** `CHANGELOG.md`, `.claude-plugin/plugin.json`

- [ ] **Step 1: CHANGELOG entry**

Add at the top of `CHANGELOG.md` (above `## [0.2.3]`), dated deliberately (today is 2026-06-01; reusing it alongside `[0.2.3]` is acceptable):

```markdown
## [0.3.0] - 2026-06-01

### Added
- `slackline listen --type <types>` — emit only the named event types (`mention`, `dm`, `thread_reply`, `channel_message`, `reaction`), removing the need to `jq`-filter the stream. Unknown types error; `channel_message` requires `--all-messages`.

### Changed
- **Breaking:** the two listen reaction events `reaction_added` / `reaction_removed` are unified into a single `reaction` event with an `action` field (`"added"` | `"removed"`).
- **Breaking:** `slackline ask` now exits `5` on timeout (was `1`), so callers can distinguish a timeout from an API/auth/config error without parsing stderr.
```

- [ ] **Step 2: Bump plugin.json**

In `.claude-plugin/plugin.json`, change `"version": "0.2.3"` to `"version": "0.3.0"`.

- [ ] **Step 3: Check for version drift**

Run: `grep -rn "0\.2\.3" --include="*.json" --include="*.go" . | grep -v "\.git/"`
Expected: no matches (CHANGELOG keeps `[0.2.3]` history, which is fine — it's a `.md`).

- [ ] **Step 4: Final gate**

Run:
```bash
go test ./... 2>&1 | tail -5
go vet ./...
golangci-lint run ./...
```
Expected: all `ok`; `0 issues.`

- [ ] **Step 5: Commit the release prep and tag**

```bash
git add CHANGELOG.md .claude-plugin/plugin.json
git commit -m "Release v0.3.0: listen --type, unified reaction event, ask timeout exit code (PRI-2017)"
```

The branch must be merged to `main` before tagging (the `make release` flow tags `main`). Merge per the project's normal flow, then on `main`:

```bash
make release VERSION=0.3.0
```

This tags `v0.3.0`, pushes, and triggers `.github/workflows/release.yml` to build `slackline-darwin-arm64` + `slackline-linux-amd64` and attach them to the release.

- [ ] **Step 6: Set release notes and verify**

```bash
gh run watch "$(gh run list --workflow release.yml --limit 1 --json databaseId -q '.[0].databaseId')" --repo prime-radiant-inc/slackline --exit-status
gh release edit v0.3.0 --repo prime-radiant-inc/slackline --notes "$(printf '### Added\n- listen --type filter\n\n### Changed\n- Unified reaction event (action field); ask exits 5 on timeout')"
gh release view v0.3.0 --repo prime-radiant-inc/slackline --json tagName,assets,isDraft -q '{tag:.tagName,draft:.isDraft,assets:[.assets[].name]}'
```
Expected: both binaries attached, `draft:false`.

- [ ] **Step 7: Verify the published binary**

```bash
bash <(gh api repos/prime-radiant-inc/slackline/contents/install.sh --jq '.content | @base64d') | tail -3
slackline listen --type bogus 2>&1 || true   # should print an invalid_type usage error (exit 4)
```
Expected: install reports `slackline version v0.3.0`; the bogus `--type` prints the usage error.

---

## Self-Review notes (for the executor)

- **Spec coverage:** §1 listen --type → Tasks 3,4 + docs Task 7 + skill Task 8. §2 reaction collapse → Task 2 + docs/skill. §3 ask exit code + seam → Tasks 1,5. §4 skill restructure → Tasks 7,8. Versioning & docs → Tasks 7,9.
- **Ordering:** Task 1 (errs) and Task 2 (reaction) are prerequisites for Task 4 (uses `EventTypeReaction`) and Task 5 (uses `errs.Timeout`). Keep the order.
- **Breaking-change reminder:** after Task 2, any not-yet-updated consumer keying on `reaction_added`/`reaction_removed` breaks — that's intended (documented in CHANGELOG/README Migration).
