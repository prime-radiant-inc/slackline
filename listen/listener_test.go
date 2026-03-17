package listen

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/slack-go/slack/slackevents"
)

const (
	testBotUserID = "UBOT123"
	testChannelID = "C123"
	testEventType = "mention"
)

// newTestListener creates a Listener with only the fields needed for
// emit and handleEventsAPI (no Socket Mode client required).
func newTestListener() (*Listener, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	l := &Listener{
		botUserID: testBotUserID,
		out:       buf,
		status:    &bytes.Buffer{},
	}
	return l, buf
}

// parseJSONL parses the buffer as newline-delimited JSON, returning
// each line as a map. Fails the test on any parse error.
func parseJSONL(t *testing.T, buf *bytes.Buffer) []map[string]interface{} {
	t.Helper()
	var results []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		results = append(results, m)
	}
	return results
}

// --- emit tests ---

func TestEmit_ValidJSON(t *testing.T) {
	l, buf := newTestListener()
	l.emit(Event{
		Type:    testEventType,
		Channel: testChannelID,
		User:    "U456",
		Text:    "hello",
		TS:      "100.001",
	})

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	m := lines[0]
	if m["type"] != testEventType {
		t.Errorf("type = %v, want %s", m["type"], testEventType)
	}
	if m["channel"] != testChannelID {
		t.Errorf("channel = %v, want %s", m["channel"], testChannelID)
	}
	if m["user"] != "U456" {
		t.Errorf("user = %v, want U456", m["user"])
	}
	if m["text"] != "hello" {
		t.Errorf("text = %v, want hello", m["text"])
	}
	if m["ts"] != "100.001" {
		t.Errorf("ts = %v, want 100.001", m["ts"])
	}
}

func TestEmit_TrailingNewline(t *testing.T) {
	l, buf := newTestListener()
	l.emit(Event{Type: "mention", Channel: "C1"})

	raw := buf.String()
	if !strings.HasSuffix(raw, "\n") {
		t.Error("output should end with a newline")
	}
	// Exactly one newline at the end (JSONL format).
	if strings.HasSuffix(raw, "\n\n") {
		t.Error("output should have exactly one trailing newline, not two")
	}
}

func TestEmit_StripsEmptyThreadTS(t *testing.T) {
	l, buf := newTestListener()
	l.emit(Event{
		Type:     "mention",
		Channel:  "C1",
		TS:       "100.001",
		ThreadTS: "",
	})

	lines := parseJSONL(t, buf)
	if _, ok := lines[0]["thread_ts"]; ok {
		t.Error("thread_ts should be omitted when empty")
	}
}

func TestEmit_StripsSelfReferentialThreadTS(t *testing.T) {
	l, buf := newTestListener()
	l.emit(Event{
		Type:     "mention",
		Channel:  "C1",
		TS:       "100.001",
		ThreadTS: "100.001", // same as TS — top-level message, not a reply
	})

	lines := parseJSONL(t, buf)
	if _, ok := lines[0]["thread_ts"]; ok {
		t.Error("thread_ts should be omitted when equal to ts")
	}
}

func TestEmit_PreservesThreadTS(t *testing.T) {
	l, buf := newTestListener()
	l.emit(Event{
		Type:     "mention",
		Channel:  "C1",
		TS:       "200.002",
		ThreadTS: "100.001", // different from TS — this is a threaded reply
	})

	lines := parseJSONL(t, buf)
	if lines[0]["thread_ts"] != "100.001" {
		t.Errorf("thread_ts = %v, want 100.001", lines[0]["thread_ts"])
	}
}

// --- handleEventsAPI tests ---

// makeEventsAPIEvent constructs a slackevents.EventsAPIEvent wrapping the
// given inner event data pointer.
func makeEventsAPIEvent(innerData interface{}) slackevents.EventsAPIEvent {
	return slackevents.EventsAPIEvent{
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Data: innerData,
		},
	}
}

func TestHandleEventsAPI_Mention(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.AppMentionEvent{
		User:            "U999",
		Text:            "hey bot",
		Channel:         testChannelID,
		TimeStamp:       "100.001",
		ThreadTimeStamp: "90.000",
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	m := lines[0]
	if m["type"] != testEventType {
		t.Errorf("type = %v, want %s", m["type"], testEventType)
	}
	if m["channel"] != testChannelID {
		t.Errorf("channel = %v, want %s", m["channel"], testChannelID)
	}
	if m["user"] != "U999" {
		t.Errorf("user = %v, want U999", m["user"])
	}
	if m["text"] != "hey bot" {
		t.Errorf("text = %v, want 'hey bot'", m["text"])
	}
	if m["thread_ts"] != "90.000" {
		t.Errorf("thread_ts = %v, want 90.000", m["thread_ts"])
	}
}

func TestHandleEventsAPI_MentionSelfFiltered(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.AppMentionEvent{
		User:    testBotUserID, // bot's own message
		Text:    "I mentioned myself",
		Channel: testChannelID,
	}))

	if buf.Len() != 0 {
		t.Errorf("self-mention should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_DM(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      "U999",
		Text:      "hello in DM",
		Channel:   "D01DIRECTMSG", // DM channels start with D
		TimeStamp: "200.001",
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	if lines[0]["type"] != "dm" {
		t.Errorf("type = %v, want dm", lines[0]["type"])
	}
	if lines[0]["channel"] != "D01DIRECTMSG" {
		t.Errorf("channel = %v", lines[0]["channel"])
	}
}

func TestHandleEventsAPI_DMSelfFiltered(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:    testBotUserID,
		Text:    "my own DM",
		Channel: "D01DIRECTMSG",
	}))

	if buf.Len() != 0 {
		t.Errorf("self-DM should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_DMNonDMChannelIgnored(t *testing.T) {
	l, buf := newTestListener()
	// Channel messages (starting with C) should be ignored by the DM handler
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:    "U999",
		Text:    "channel message, not a DM",
		Channel: "C01OPS12345",
	}))

	if buf.Len() != 0 {
		t.Errorf("non-DM message should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_DMSubtypeIgnored(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:    "U999",
		Text:    "edited",
		Channel: "D01DIRECTMSG",
		SubType: "message_changed",
	}))

	if buf.Len() != 0 {
		t.Errorf("message with subtype should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_Reaction(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.ReactionAddedEvent{
		User:     "U999",
		Reaction: "eyes",
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
	if m["type"] != "reaction" {
		t.Errorf("type = %v, want reaction", m["type"])
	}
	if m["emoji"] != "eyes" {
		t.Errorf("emoji = %v, want eyes", m["emoji"])
	}
	if m["item_ts"] != "300.001" {
		t.Errorf("item_ts = %v, want 300.001", m["item_ts"])
	}
	if m["channel"] != testChannelID {
		t.Errorf("channel = %v, want %s", m["channel"], testChannelID)
	}
}

func TestHandleEventsAPI_ReactionSelfFiltered(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.ReactionAddedEvent{
		User:     testBotUserID,
		Reaction: "thumbsup",
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: "300.001",
		},
	}))

	if buf.Len() != 0 {
		t.Errorf("self-reaction should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_UnknownInnerEventIgnored(t *testing.T) {
	l, buf := newTestListener()
	// Pass a type that handleEventsAPI doesn't handle
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.PinAddedEvent{}))

	if buf.Len() != 0 {
		t.Errorf("unknown inner event should be ignored, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_EmptyChannelMessageIgnored(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:    "U999",
		Text:    "no channel",
		Channel: "", // empty channel
	}))

	if buf.Len() != 0 {
		t.Errorf("message with empty channel should be dropped, got: %s", buf.String())
	}
}
