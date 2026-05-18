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

	err := runReactAddWithAPI(api, fixtureChannelID, "100.001", fixtureEmojiThumb, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.reactionsAdded) != 1 {
		t.Fatalf("expected 1 AddReaction call, got %d", len(api.reactionsAdded))
	}
	got := api.reactionsAdded[0]
	if got.Name != fixtureEmojiThumb {
		t.Errorf("name = %q, want thumbsup", got.Name)
	}
	if got.Item.Channel != fixtureChannelID || got.Item.Timestamp != "100.001" {
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
	if out["emoji"] != fixtureEmojiThumb {
		t.Errorf("emoji = %v", out["emoji"])
	}
}

func TestReactAdd_StripsColons(t *testing.T) {
	api := &fakeSlackAPI{}
	_ = runReactAddWithAPI(api, fixtureChannelID, "100", ":party:", &bytes.Buffer{})
	if api.reactionsAdded[0].Name != "party" {
		t.Errorf("name = %q, want party (colons stripped)", api.reactionsAdded[0].Name)
	}
}

func TestReactAdd_AlreadyReactedIsIdempotent(t *testing.T) {
	api := &fakeSlackAPI{addReactionErr: errors.New("already_reacted")}
	stdout := &bytes.Buffer{}
	err := runReactAddWithAPI(api, fixtureChannelID, "100", fixtureEmojiThumb, stdout)
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
	err := runReactAddWithAPI(api, fixtureChannelID, "100", fixtureEmojiThumb, &bytes.Buffer{})
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
	err := runReactRemoveWithAPI(api, fixtureChannelID, "100.001", fixtureEmojiThumb, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.reactionsRemoved) != 1 {
		t.Fatalf("expected 1 RemoveReaction call, got %d", len(api.reactionsRemoved))
	}
	if api.reactionsRemoved[0] != (capturedReaction{Name: fixtureEmojiThumb, Item: goslack.ItemRef{Channel: fixtureChannelID, Timestamp: "100.001"}}) {
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
	err := runReactRemoveWithAPI(api, fixtureChannelID, "100", fixtureEmojiThumb, stdout)
	if err != nil {
		t.Fatalf("expected no error for no_reaction, got: %v", err)
	}
	var out map[string]interface{}
	_ = json.Unmarshal(stdout.Bytes(), &out)
	if out["no_op"] != true {
		t.Errorf("no_op should be true")
	}
}
