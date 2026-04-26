package listen

import (
	"encoding/json"
	"testing"
)

func TestMentionEvent_JSON(t *testing.T) {
	e := Event{
		Type: "mention", Channel: "C01TESTCHAN", User: "U0123",
		Text: "hey @test-bot check the logs", TS: "1769756026.624319",
		ThreadTS: "1769756026.624319",
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["type"] != "mention" {
		t.Errorf("type = %v, want mention", got["type"])
	}
	if got["channel"] != "C01TESTCHAN" {
		t.Errorf("channel = %v", got["channel"])
	}
	if got["text"] != "hey @test-bot check the logs" {
		t.Errorf("text = %v", got["text"])
	}
}

func TestDMEvent_JSON(t *testing.T) {
	e := Event{Type: "dm", Channel: "D01TESTDM00", User: "U0456", Text: "can you review this PR?", TS: "1769756030.111111"}
	data, _ := json.Marshal(e)
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["type"] != "dm" {
		t.Errorf("type = %v, want dm", got["type"])
	}
	if _, ok := got["thread_ts"]; ok {
		t.Error("thread_ts should be omitted when empty")
	}
}

func TestReactionAddedEvent_JSON(t *testing.T) {
	e := Event{Type: "reaction_added", Channel: "C01TESTCHAN", User: "U0123", Emoji: "eyes", ItemTS: "1769756026.624319"}
	data, _ := json.Marshal(e)
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["type"] != "reaction_added" {
		t.Errorf("type = %v, want reaction_added", got["type"])
	}
	if got["emoji"] != "eyes" {
		t.Errorf("emoji = %v, want eyes", got["emoji"])
	}
	if got["item_ts"] != "1769756026.624319" {
		t.Errorf("item_ts = %v", got["item_ts"])
	}
	if _, ok := got["text"]; ok {
		t.Error("reaction_added event should not have text")
	}
}

func TestReactionRemovedEvent_JSON(t *testing.T) {
	e := Event{Type: "reaction_removed", Channel: "C01TESTCHAN", User: "U0123", Emoji: "thumbsup", ItemTS: "1769756026.624319"}
	data, _ := json.Marshal(e)
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["type"] != "reaction_removed" {
		t.Errorf("type = %v, want reaction_removed", got["type"])
	}
	if got["emoji"] != "thumbsup" {
		t.Errorf("emoji = %v, want thumbsup", got["emoji"])
	}
	if got["item_ts"] != "1769756026.624319" {
		t.Errorf("item_ts = %v", got["item_ts"])
	}
	if _, ok := got["text"]; ok {
		t.Error("reaction_removed event should not have text")
	}
}

func TestEvent_OmitsEmptyFields(t *testing.T) {
	e := Event{Type: "mention", Channel: "C123", User: "U123", Text: "hello", TS: "123.456"}
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
