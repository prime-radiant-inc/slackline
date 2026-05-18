package listen

import (
	"encoding/json"
	"testing"
)

const (
	fixtureChannelID = "C01TESTCHAN"
	fixtureDMID      = "D01TESTDM00"
	fixtureMessageTS = "1769756026.624319"
	fixtureEmojiEyes = "eyes"
	fixtureUserID    = "U0123"
)

func TestMentionEvent_JSON(t *testing.T) {
	e := Event{
		Type: EventTypeMention, Channel: fixtureChannelID, User: fixtureUserID,
		Text: "hey @test-bot check the logs", TS: fixtureMessageTS,
		ThreadTS: fixtureMessageTS,
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["type"] != EventTypeMention {
		t.Errorf("type = %v, want %s", got["type"], EventTypeMention)
	}
	if got["channel"] != fixtureChannelID {
		t.Errorf("channel = %v", got["channel"])
	}
	if got["text"] != "hey @test-bot check the logs" {
		t.Errorf("text = %v", got["text"])
	}
}

func TestDMEvent_JSON(t *testing.T) {
	e := Event{Type: EventTypeDM, Channel: fixtureDMID, User: "U0456", Text: "can you review this PR?", TS: "1769756030.111111"}
	data, _ := json.Marshal(e)
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["type"] != EventTypeDM {
		t.Errorf("type = %v, want %s", got["type"], EventTypeDM)
	}
	if _, ok := got["thread_ts"]; ok {
		t.Error("thread_ts should be omitted when empty")
	}
}

func TestReactionAddedEvent_JSON(t *testing.T) {
	e := Event{Type: EventTypeReactionAdded, Channel: fixtureChannelID, User: fixtureUserID, Emoji: fixtureEmojiEyes, ItemTS: fixtureMessageTS}
	data, _ := json.Marshal(e)
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["type"] != EventTypeReactionAdded {
		t.Errorf("type = %v, want %s", got["type"], EventTypeReactionAdded)
	}
	if got["emoji"] != fixtureEmojiEyes {
		t.Errorf("emoji = %v, want %s", got["emoji"], fixtureEmojiEyes)
	}
	if got["item_ts"] != fixtureMessageTS {
		t.Errorf("item_ts = %v", got["item_ts"])
	}
	if _, ok := got["text"]; ok {
		t.Error("reaction_added event should not have text")
	}
}

func TestReactionRemovedEvent_JSON(t *testing.T) {
	e := Event{Type: EventTypeReactionRemoved, Channel: fixtureChannelID, User: fixtureUserID, Emoji: "thumbsup", ItemTS: fixtureMessageTS}
	data, _ := json.Marshal(e)
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["type"] != EventTypeReactionRemoved {
		t.Errorf("type = %v, want %s", got["type"], EventTypeReactionRemoved)
	}
	if got["emoji"] != "thumbsup" {
		t.Errorf("emoji = %v, want thumbsup", got["emoji"])
	}
	if got["item_ts"] != fixtureMessageTS {
		t.Errorf("item_ts = %v", got["item_ts"])
	}
	if _, ok := got["text"]; ok {
		t.Error("reaction_removed event should not have text")
	}
}

func TestEvent_OmitsEmptyFields(t *testing.T) {
	const helloText = "hello"
	e := Event{Type: EventTypeMention, Channel: "C123", User: "U123", Text: helloText, TS: "123.456"}
	data, _ := json.Marshal(e)
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"thread_ts", "emoji", "item_ts"} {
		if _, ok := got[key]; ok {
			t.Errorf("field %q should be omitted when empty", key)
		}
	}
}
