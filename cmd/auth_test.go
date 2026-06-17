package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
	goslack "github.com/slack-go/slack"
)

func TestAuthWhoamiWithAPI_PrintsIdentity(t *testing.T) {
	api := &fakeSlackAPI{
		authResp: &goslack.AuthTestResponse{
			Team:   "Prime Radiant",
			TeamID: "T123",
			User:   "slackline-bot",
			UserID: "U123",
			BotID:  "B123",
			URL:    "https://prime.slack.com/",
		},
	}
	var out bytes.Buffer

	if err := runAuthWhoamiWithAPI(api, &out); err != nil {
		t.Fatalf("runAuthWhoamiWithAPI failed: %v", err)
	}

	want := strings.Join([]string{
		"Bot:       slackline-bot (U123)",
		"Bot ID:    B123",
		"Workspace: Prime Radiant (T123)",
		"URL:       https://prime.slack.com/",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestAuthWhoamiWithAPI_AuthFailureUsesAuthError(t *testing.T) {
	api := &fakeSlackAPI{authErr: errors.New("invalid_auth")}

	err := runAuthWhoamiWithAPI(api, &bytes.Buffer{})

	var se *errs.SlackError
	if !errors.As(err, &se) {
		t.Fatalf("err = %T, want SlackError", err)
	}
	if se.Code != errs.Auth {
		t.Fatalf("code = %d, want %d", se.Code, errs.Auth)
	}
}
