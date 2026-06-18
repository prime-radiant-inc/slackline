package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
)

func readUsers() []goslack.User {
	u := goslack.User{}
	u.ID = "U1"
	u.Name = "drew"
	u.Profile.DisplayName = "Drew"
	u.RealName = "Drew Smith"
	return []goslack.User{u}
}

func TestResolveNames_EnrichesTextAndAuthor(t *testing.T) {
	api := &fakeSlackAPI{users: readUsers()}
	dir := slackpkg.NewUserDirectory(api)
	msgs := []messageOutput{{TS: ts1, User: "U1", Text: "hi <@U1> there"}}

	got := resolveNames(dir, msgs, &bytes.Buffer{})
	if got[0].Text != "hi <@U1|drew> there" {
		t.Fatalf("text = %q, want enriched mention", got[0].Text)
	}
	if got[0].UserName != "drew" {
		t.Fatalf("user_name = %q, want drew", got[0].UserName)
	}
}

func TestResolveNames_NilDirIsNoop(t *testing.T) {
	msgs := []messageOutput{{TS: ts1, User: "U1", Text: "hi <@U1>"}}
	got := resolveNames(nil, msgs, &bytes.Buffer{})
	if got[0].Text != "hi <@U1>" || got[0].UserName != "" {
		t.Fatalf("nil dir should not modify messages: %+v", got[0])
	}
}

func TestResolveNames_LoadErrorWarnsAndKeepsRaw(t *testing.T) {
	api := &fakeSlackAPI{usersErr: errors.New("missing_scope")}
	dir := slackpkg.NewUserDirectory(api)
	msgs := []messageOutput{{TS: ts1, User: "U1", Text: "hi <@U1>"}}

	var warn bytes.Buffer
	got := resolveNames(dir, msgs, &warn)
	if got[0].Text != "hi <@U1>" || got[0].UserName != "" {
		t.Fatalf("resolution failure should keep raw output: %+v", got[0])
	}
	if !strings.Contains(warn.String(), "could not resolve") {
		t.Fatalf("expected resolution warning, got %q", warn.String())
	}
}

func TestFormatMessageText_WithUserName(t *testing.T) {
	out := messageOutput{TS: fixtureMessageTS, User: "U1", UserName: "drew", Text: "hello"}
	got := formatMessageText(out)
	want := "123.456 U1|drew hello\n"
	if got != want {
		t.Fatalf("formatMessageText = %q, want %q", got, want)
	}
}

func TestMessageOutput_JSONL_UserName(t *testing.T) {
	out := messageOutput{TS: fixtureMessageTS, User: "U1", UserName: "drew", Text: "hi"}
	var buf bytes.Buffer
	if err := writeMessageOutput(&buf, outputFormatJSON, out); err != nil {
		t.Fatalf("writeMessageOutput failed: %v", err)
	}
	if !strings.Contains(buf.String(), `"user_name":"drew"`) {
		t.Fatalf("json missing user_name: %s", buf.String())
	}
}
