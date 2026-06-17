package listen

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const (
	testBotUserID     = "UBOT123"
	testBotID         = "B123"
	testChannelID     = "C123"
	testOtherUserID   = "U999"
	testEventTS       = "100.001"
	testReplyTS       = "200.001"
	testThreadTS      = "100.000"
	testItemTS        = "300.001"
	testEmojiThumbsup = "thumbsup"
	testSelfText      = "I mentioned myself"
	testFileID        = "F123"
	testReportName    = "report.pdf"
)

// newTestListener creates a Listener with only the fields needed for
// emit and handleEventsAPI (no Socket Mode client required).
func newTestListener() (*Listener, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	l := &Listener{
		botUserID:    testBotUserID,
		botID:        testBotID,
		out:          buf,
		status:       &bytes.Buffer{},
		outputFormat: OutputFormatJSON,
	}
	return l, buf
}

func newTestTextListener() (*Listener, *bytes.Buffer) {
	l, buf := newTestListener()
	l.outputFormat = OutputFormatText
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

func TestNewListener_DefaultOutputFormatText(t *testing.T) {
	l := NewListener("", "", testBotUserID, testBotID, ListenerOptions{}, &bytes.Buffer{}, &bytes.Buffer{})

	if l.outputFormat != OutputFormatText {
		t.Fatalf("outputFormat = %q, want %q", l.outputFormat, OutputFormatText)
	}
}

func TestEmit_Text(t *testing.T) {
	l, buf := newTestTextListener()
	l.emit(Event{
		Type:    EventTypeMention,
		Channel: testChannelID,
		User:    "U456",
		Text:    "hello",
		TS:      testEventTS,
	})

	want := "mention C123 U456 100.001 hello\n"
	if buf.String() != want {
		t.Fatalf("text output = %q, want %q", buf.String(), want)
	}
}

func TestEmit_TextReaction(t *testing.T) {
	l, buf := newTestTextListener()
	l.emit(Event{
		Type:    EventTypeReaction,
		Action:  ReactionActionAdded,
		Channel: testChannelID,
		User:    "U456",
		Emoji:   testEmojiThumbsup,
		ItemTS:  testItemTS,
	})

	want := "reaction added C123 U456 item=300.001 thumbsup\n"
	if buf.String() != want {
		t.Fatalf("text output = %q, want %q", buf.String(), want)
	}
}

func TestEmit_JSONFormat(t *testing.T) {
	l, buf := newTestListener()
	l.emit(Event{
		Type:    EventTypeMention,
		Channel: testChannelID,
		User:    "U456",
		Text:    "hello",
		TS:      testEventTS,
	})

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	m := lines[0]
	if m["type"] != EventTypeMention {
		t.Errorf("type = %v, want %s", m["type"], EventTypeMention)
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
	if m["ts"] != testEventTS {
		t.Errorf("ts = %v, want 100.001", m["ts"])
	}
}

func TestEmit_TrailingNewline(t *testing.T) {
	l, buf := newTestListener()
	l.emit(Event{Type: EventTypeMention, Channel: "C1"})

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
		Type:     EventTypeMention,
		Channel:  "C1",
		TS:       testEventTS,
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
		Type:     EventTypeMention,
		Channel:  "C1",
		TS:       testEventTS,
		ThreadTS: testEventTS, // same as TS — top-level message, not a reply
	})

	lines := parseJSONL(t, buf)
	if _, ok := lines[0]["thread_ts"]; ok {
		t.Error("thread_ts should be omitted when equal to ts")
	}
}

func TestEmit_PreservesThreadTS(t *testing.T) {
	l, buf := newTestListener()
	l.emit(Event{
		Type:     EventTypeMention,
		Channel:  "C1",
		TS:       "200.002",
		ThreadTS: testEventTS, // different from TS — this is a threaded reply
	})

	lines := parseJSONL(t, buf)
	if lines[0]["thread_ts"] != testEventTS {
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
		User:            testOtherUserID,
		Text:            "hey bot",
		Channel:         testChannelID,
		TimeStamp:       testEventTS,
		ThreadTimeStamp: "90.000",
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	m := lines[0]
	if m["type"] != EventTypeMention {
		t.Errorf("type = %v, want %s", m["type"], EventTypeMention)
	}
	if m["channel"] != testChannelID {
		t.Errorf("channel = %v, want %s", m["channel"], testChannelID)
	}
	if m["user"] != testOtherUserID {
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
		Text:    testSelfText,
		Channel: testChannelID,
	}))

	if buf.Len() != 0 {
		t.Errorf("self-mention should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_MentionSelfFilteredByBotID(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.AppMentionEvent{
		BotID:   testBotID,
		Text:    testSelfText,
		Channel: testChannelID,
	}))

	if buf.Len() != 0 {
		t.Errorf("self-mention with bot_id should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_DM(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      testOtherUserID,
		Text:      "hello in DM",
		Channel:   fixtureDMID, // DM channels start with D
		TimeStamp: testReplyTS,
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	if lines[0]["type"] != EventTypeDM {
		t.Errorf("type = %v, want dm", lines[0]["type"])
	}
	if lines[0]["channel"] != fixtureDMID {
		t.Errorf("channel = %v", lines[0]["channel"])
	}
}

func TestHandleEventsAPI_DMSelfFiltered(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:    testBotUserID,
		Text:    "my own DM",
		Channel: fixtureDMID,
	}))

	if buf.Len() != 0 {
		t.Errorf("self-DM should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_FileShareDMSelfFilteredByBotID(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		BotID:   testBotID,
		Text:    "my own file",
		Channel: fixtureDMID,
		SubType: messageSubtypeFileShare,
		Message: &goslack.Msg{
			Files: []goslack.File{{ID: testFileID, Name: testReportName}},
		},
	}))

	if buf.Len() != 0 {
		t.Errorf("self file_share with bot_id should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_DMNonDMChannelIgnored(t *testing.T) {
	l, buf := newTestListener()
	// Channel messages (starting with C) should be ignored by the DM handler
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:    testOtherUserID,
		Text:    "channel message, not a DM",
		Channel: fixtureChannelID,
	}))

	if buf.Len() != 0 {
		t.Errorf("non-DM message should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_DMSubtypeIgnored(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:    testOtherUserID,
		Text:    "edited",
		Channel: fixtureDMID,
		SubType: "message_changed",
	}))

	if buf.Len() != 0 {
		t.Errorf("message with subtype should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_Reaction(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.ReactionAddedEvent{
		User:     testOtherUserID,
		Reaction: fixtureEmojiEyes,
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: testItemTS,
		},
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	m := lines[0]
	if m["type"] != EventTypeReaction {
		t.Errorf("type = %v, want reaction", m["type"])
	}
	if m["action"] != ReactionActionAdded {
		t.Errorf("action = %v, want added", m["action"])
	}
	if m["emoji"] != fixtureEmojiEyes {
		t.Errorf("emoji = %v, want eyes", m["emoji"])
	}
	if m["item_ts"] != testItemTS {
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
		Reaction: testEmojiThumbsup,
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: testItemTS,
		},
	}))

	if buf.Len() != 0 {
		t.Errorf("self-reaction should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_ReactionRemoved(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.ReactionRemovedEvent{
		User:     testOtherUserID,
		Reaction: testEmojiThumbsup,
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: testItemTS,
		},
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	m := lines[0]
	if m["type"] != EventTypeReaction {
		t.Errorf("type = %v, want reaction", m["type"])
	}
	if m["action"] != ReactionActionRemoved {
		t.Errorf("action = %v, want removed", m["action"])
	}
	if m["emoji"] != testEmojiThumbsup {
		t.Errorf("emoji = %v, want thumbsup", m["emoji"])
	}
	if m["item_ts"] != testItemTS {
		t.Errorf("item_ts = %v", m["item_ts"])
	}
}

func TestHandleEventsAPI_ReactionRemovedSelfFiltered(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.ReactionRemovedEvent{
		User:     testBotUserID,
		Reaction: testEmojiThumbsup,
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: testItemTS,
		},
	}))

	if buf.Len() != 0 {
		t.Errorf("self reaction_removed should be dropped, got: %s", buf.String())
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
		User:    testOtherUserID,
		Text:    "no channel",
		Channel: "", // empty channel
	}))

	if buf.Len() != 0 {
		t.Errorf("message with empty channel should be dropped, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_IncludeBotSelf_Mention(t *testing.T) {
	l, buf := newTestListener()
	l.includeBotSelf = true
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.AppMentionEvent{
		User:    testBotUserID,
		Text:    testSelfText,
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
		Reaction: testEmojiThumbsup,
		Item: slackevents.Item{
			Channel:   testChannelID,
			Timestamp: testItemTS,
		},
	}))
	if buf.Len() == 0 {
		t.Error("self reaction should pass with --include-bot-self")
	}
}

func TestHandleEventsAPI_DM_WithFiles(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      testOtherUserID,
		Text:      "see this",
		Channel:   fixtureDMID,
		TimeStamp: testReplyTS,
		// Files live on MessageEvent.Message (*goslack.Msg), not on MessageEvent directly.
		Message: &goslack.Msg{
			Files: []goslack.File{
				{ID: testFileID, Name: testReportName, Mimetype: "application/pdf", Size: 12345, Title: "Q4 Report"},
				{ID: "F456", Name: "extra.png", Mimetype: "image/png", Size: 6789},
			},
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
	if first["id"] != testFileID {
		t.Errorf("files[0].id = %v", first["id"])
	}
	if first["name"] != testReportName {
		t.Errorf("files[0].name = %v", first["name"])
	}
	if first["mimetype"] != "application/pdf" {
		t.Errorf("files[0].mimetype = %v", first["mimetype"])
	}
}

// AppMentionEvent (v0.19.0) has no Files field; mention events never carry files.
// This test confirms the files key is absent on mention output.
func TestHandleEventsAPI_Mention_NoFilesOmitted(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.AppMentionEvent{
		User:      testOtherUserID,
		Text:      "look at this",
		Channel:   testChannelID,
		TimeStamp: testEventTS,
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	if _, ok := lines[0]["files"]; ok {
		t.Error("files key should be absent on mention events (AppMentionEvent has no Files field)")
	}
}

func TestHandleEventsAPI_DM_NoFilesOmitsArray(t *testing.T) {
	l, buf := newTestListener()
	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      testOtherUserID,
		Text:      "no files",
		Channel:   fixtureDMID,
		TimeStamp: testReplyTS,
	}))

	lines := parseJSONL(t, buf)
	if _, ok := lines[0]["files"]; ok {
		t.Error("files key should be omitted when there are no attachments")
	}
}

func TestHandleEventsAPI_ThreadsMode_BotParentReply(t *testing.T) {
	l, buf := newTestListener()
	l.threads = true

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:            testOtherUserID,
		Text:            "thanks bot",
		Channel:         fixtureChannelID,
		TimeStamp:       testReplyTS,
		ThreadTimeStamp: testThreadTS,
		Message: &goslack.Msg{
			ParentUserId: testBotUserID,
		},
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	if lines[0]["type"] != EventTypeThreadReply {
		t.Errorf("type = %v, want thread_reply", lines[0]["type"])
	}
	if lines[0]["thread_ts"] != testThreadTS {
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
		User:            testOtherUserID,
		Text:            "talking among themselves",
		Channel:         fixtureChannelID,
		TimeStamp:       testReplyTS,
		ThreadTimeStamp: testThreadTS,
		Message: &goslack.Msg{
			ParentUserId: "U_OTHER",
		},
	}))

	if buf.Len() != 0 {
		t.Errorf("thread reply with non-bot parent should be dropped in --threads mode, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_ThreadsMode_NonThreadDropped(t *testing.T) {
	l, buf := newTestListener()
	l.threads = true

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      testOtherUserID,
		Text:      "top-level channel msg",
		Channel:   fixtureChannelID,
		TimeStamp: testReplyTS,
	}))

	if buf.Len() != 0 {
		t.Error("non-thread channel message should be dropped in --threads mode without --all-messages")
	}
}

func TestHandleEventsAPI_AllMessagesMode_ChannelMessage(t *testing.T) {
	l, buf := newTestListener()
	l.allMessages = true

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      testOtherUserID,
		Text:      "hi everyone",
		Channel:   fixtureChannelID,
		TimeStamp: testReplyTS,
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	if lines[0]["type"] != EventTypeChannelMessage {
		t.Errorf("type = %v, want channel_message", lines[0]["type"])
	}
}

func TestHandleEventsAPI_DefaultMode_BotParentThreadReplyEmitted(t *testing.T) {
	l, buf := newTestListener()
	// Default — no --threads, no --all-messages.

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:            testOtherUserID,
		Text:            "thanks bot",
		Channel:         fixtureChannelID,
		TimeStamp:       testReplyTS,
		ThreadTimeStamp: testThreadTS,
		Message: &goslack.Msg{
			ParentUserId: testBotUserID,
		},
	}))

	lines := parseJSONL(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1", len(lines))
	}
	if lines[0]["type"] != EventTypeThreadReply {
		t.Errorf("type = %v, want thread_reply", lines[0]["type"])
	}
	if lines[0]["thread_ts"] != testThreadTS {
		t.Errorf("thread_ts = %v", lines[0]["thread_ts"])
	}
	if lines[0]["parent_user_id"] != testBotUserID {
		t.Errorf("parent_user_id = %v", lines[0]["parent_user_id"])
	}
}

func TestHandleEventsAPI_DefaultMode_NonBotParentThreadReplyDropped(t *testing.T) {
	l, buf := newTestListener()

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:            testOtherUserID,
		Text:            "talking among themselves",
		Channel:         fixtureChannelID,
		TimeStamp:       testReplyTS,
		ThreadTimeStamp: testThreadTS,
		Message: &goslack.Msg{
			ParentUserId: "U_OTHER",
		},
	}))

	if buf.Len() != 0 {
		t.Errorf("thread reply with non-bot parent should be dropped in default mode, got: %s", buf.String())
	}
}

func TestHandleEventsAPI_DefaultMode_TopLevelChannelMessageDropped(t *testing.T) {
	l, buf := newTestListener()

	l.handleEventsAPI(makeEventsAPIEvent(&slackevents.MessageEvent{
		User:      testOtherUserID,
		Text:      "top-level channel msg",
		Channel:   fixtureChannelID,
		TimeStamp: testReplyTS,
	}))

	if buf.Len() != 0 {
		t.Errorf("top-level channel message should be dropped in default mode, got: %s", buf.String())
	}
}

func TestEmit_TypeFilter(t *testing.T) {
	buf := &bytes.Buffer{}
	l := &Listener{
		botUserID:    testBotUserID,
		out:          buf,
		status:       &bytes.Buffer{},
		types:        map[string]bool{EventTypeMention: true},
		outputFormat: OutputFormatJSON,
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

func TestHandleEvent_HelloEmitsReady(t *testing.T) {
	statusBuf := &bytes.Buffer{}
	l := &Listener{out: &bytes.Buffer{}, status: statusBuf}
	l.handleEvent(socketmode.Event{Type: socketmode.EventTypeHello})
	if !strings.Contains(statusBuf.String(), "ready") {
		t.Errorf("status = %q, want it to contain \"ready\"", statusBuf.String())
	}
}
