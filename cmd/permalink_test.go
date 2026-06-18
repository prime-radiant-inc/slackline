package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
)

func TestPermalink_PrintsURL(t *testing.T) {
	api := &fakeSlackAPI{permalinkURL: "https://example.slack.com/archives/C123/p123456000000"}
	var out bytes.Buffer

	err := runPermalinkWithAPI(api, fixtureChannelID, fixtureMessageTS, &out)
	if err != nil {
		t.Fatalf("runPermalinkWithAPI failed: %v", err)
	}

	if api.capturedPermalinkParams == nil {
		t.Fatal("GetPermalink was not called")
	}
	if api.capturedPermalinkParams.Channel != fixtureChannelID {
		t.Errorf("channel = %q, want %q", api.capturedPermalinkParams.Channel, fixtureChannelID)
	}
	if api.capturedPermalinkParams.Ts != fixtureMessageTS {
		t.Errorf("ts = %q, want %q", api.capturedPermalinkParams.Ts, fixtureMessageTS)
	}
	if out.String() != "https://example.slack.com/archives/C123/p123456000000\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPermalink_AuthFailureUsesAuthError(t *testing.T) {
	api := &fakeSlackAPI{permalinkErr: errors.New("invalid_auth")}

	err := runPermalinkWithAPI(api, fixtureChannelID, fixtureMessageTS, &bytes.Buffer{})

	var se *errs.SlackError
	if !errors.As(err, &se) {
		t.Fatalf("err = %T, want SlackError", err)
	}
	if se.Code != errs.Auth {
		t.Fatalf("code = %d, want %d", se.Code, errs.Auth)
	}
}

func TestPermalink_APIFailureUsesPermalinkError(t *testing.T) {
	api := &fakeSlackAPI{permalinkErr: errors.New("message_not_found")}

	err := runPermalinkWithAPI(api, fixtureChannelID, fixtureMessageTS, &bytes.Buffer{})

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "permalink_failed") {
		t.Fatalf("error should use permalink_failed, got: %v", err)
	}
	if !strings.Contains(err.Error(), "message_not_found") {
		t.Fatalf("error should include Slack reason, got: %v", err)
	}
}
