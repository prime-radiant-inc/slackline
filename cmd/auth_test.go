package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/config"
	"github.com/prime-radiant-inc/slackline/errs"
	goslack "github.com/slack-go/slack"
)

type fakeAppTokenAPI struct {
	calls int
	err   error
}

func (f *fakeAppTokenAPI) OpenSocketMode(ctx context.Context) error {
	f.calls++
	return f.err
}

func TestAuthStatusWithAPIs_ValidatesAppToken(t *testing.T) {
	cfg := &config.Config{
		Bot: config.Bot{
			BotToken: "xoxb-1234567890",
			AppToken: "xapp-1234567890",
		},
	}
	botAPI := &fakeSlackAPI{
		authResp: &goslack.AuthTestResponse{
			Team:   "Prime Radiant",
			TeamID: "T123",
			User:   "slackline-bot",
		},
	}
	appAPI := &fakeAppTokenAPI{}
	var out bytes.Buffer

	if err := runAuthStatusWithAPIs(cfg, "/tmp/config.json", botAPI, appAPI, &out); err != nil {
		t.Fatalf("runAuthStatusWithAPIs failed: %v", err)
	}

	if appAPI.calls != 1 {
		t.Fatalf("OpenSocketMode calls = %d, want 1", appAPI.calls)
	}
	if !strings.Contains(out.String(), "App Token: xapp-...7890 (valid)") {
		t.Fatalf("output should mark app token valid, got:\n%s", out.String())
	}
}

func TestAuthStatusWithAPIs_InvalidAppTokenReportsReason(t *testing.T) {
	cfg := &config.Config{
		Bot: config.Bot{
			BotToken: "xoxb-1234567890",
			AppToken: "xapp-1234567890",
		},
	}
	appAPI := &fakeAppTokenAPI{err: errors.New("invalid_auth")}
	var out bytes.Buffer

	if err := runAuthStatusWithAPIs(cfg, "/tmp/config.json", &fakeSlackAPI{}, appAPI, &out); err != nil {
		t.Fatalf("runAuthStatusWithAPIs failed: %v", err)
	}

	if !strings.Contains(out.String(), "App Token: xapp-...7890 (invalid: invalid_auth)") {
		t.Fatalf("output should mark app token invalid, got:\n%s", out.String())
	}
}

func TestAuthStatusWithAPIs_BadAppTokenPrefixDoesNotValidate(t *testing.T) {
	cfg := &config.Config{
		Bot: config.Bot{
			AppToken: "xoxb-not-an-app-token",
		},
	}
	appAPI := &fakeAppTokenAPI{}
	var out bytes.Buffer

	if err := runAuthStatusWithAPIs(cfg, "/tmp/config.json", &fakeSlackAPI{}, appAPI, &out); err != nil {
		t.Fatalf("runAuthStatusWithAPIs failed: %v", err)
	}

	if appAPI.calls != 0 {
		t.Fatalf("OpenSocketMode calls = %d, want 0", appAPI.calls)
	}
	if !strings.Contains(out.String(), "App Token: xoxb-...oken (invalid: expected xapp- prefix)") {
		t.Fatalf("output should reject app token prefix, got:\n%s", out.String())
	}
}

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
