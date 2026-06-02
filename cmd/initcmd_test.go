package cmd

import (
	"bytes"
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
)

func TestReadEnvInputs_NeitherSet(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "")
	t.Setenv("SLACKLINE_APP_TOKEN", "")
	t.Setenv("SLACKLINE_WORKSPACE_URL", "")

	inputs, err := readEnvInputs()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if inputs != nil {
		t.Fatalf("expected nil inputs (interactive mode), got %+v", inputs)
	}
}

func TestReadEnvInputs_BothSet(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxb-valid-token")
	t.Setenv("SLACKLINE_APP_TOKEN", "xapp-valid-token")
	t.Setenv("SLACKLINE_WORKSPACE_URL", "https://myteam.slack.com")

	inputs, err := readEnvInputs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inputs == nil {
		t.Fatal("expected inputs, got nil")
	}
	if inputs.botToken != "xoxb-valid-token" {
		t.Errorf("botToken = %q, want %q", inputs.botToken, "xoxb-valid-token")
	}
	if inputs.appToken != "xapp-valid-token" {
		t.Errorf("appToken = %q, want %q", inputs.appToken, "xapp-valid-token")
	}
	if inputs.workspaceURL != "https://myteam.slack.com" {
		t.Errorf("workspaceURL = %q, want %q", inputs.workspaceURL, "https://myteam.slack.com")
	}
}

func TestReadEnvInputs_OnlyBotSet(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxb-valid-token")
	t.Setenv("SLACKLINE_APP_TOKEN", "")

	_, err := readEnvInputs()
	if err == nil {
		t.Fatal("expected error when only bot token is set")
	}
	se, ok := err.(*errs.SlackError)
	if !ok {
		t.Fatalf("expected *errs.SlackError, got %T", err)
	}
	if se.Code != errs.Usage {
		t.Errorf("exit code = %d, want %d (Usage)", se.Code, errs.Usage)
	}
}

func TestReadEnvInputs_OnlyAppSet(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "")
	t.Setenv("SLACKLINE_APP_TOKEN", "xapp-valid-token")

	_, err := readEnvInputs()
	if err == nil {
		t.Fatal("expected error when only app token is set")
	}
	se, ok := err.(*errs.SlackError)
	if !ok {
		t.Fatalf("expected *errs.SlackError, got %T", err)
	}
	if se.Code != errs.Usage {
		t.Errorf("exit code = %d, want %d (Usage)", se.Code, errs.Usage)
	}
}

func TestReadEnvInputs_BadBotPrefix(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxp-wrong-type")
	t.Setenv("SLACKLINE_APP_TOKEN", "xapp-valid-token")

	_, err := readEnvInputs()
	if err == nil {
		t.Fatal("expected error for wrong bot token prefix")
	}
	se, ok := err.(*errs.SlackError)
	if !ok {
		t.Fatalf("expected *errs.SlackError, got %T", err)
	}
	if se.Code != errs.Usage {
		t.Errorf("exit code = %d, want %d (Usage)", se.Code, errs.Usage)
	}
}

func TestReadEnvInputs_BadAppPrefix(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "xoxb-valid-token")
	t.Setenv("SLACKLINE_APP_TOKEN", "xoxb-wrong-type")

	_, err := readEnvInputs()
	if err == nil {
		t.Fatal("expected error for wrong app token prefix")
	}
	se, ok := err.(*errs.SlackError)
	if !ok {
		t.Fatalf("expected *errs.SlackError, got %T", err)
	}
	if se.Code != errs.Usage {
		t.Errorf("exit code = %d, want %d (Usage)", se.Code, errs.Usage)
	}
}

func TestReadInteractiveInitInputsUsesSecretReader(t *testing.T) {
	restore := stubReadSecretLine(t, "xoxb-secret", "xapp-secret")
	defer restore()

	inputs, err := readInteractiveInitInputs(
		bytes.NewBufferString("https://myteam.slack.com\n"),
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("readInteractiveInitInputs: %v", err)
	}
	if inputs.workspaceURL != "https://myteam.slack.com" {
		t.Errorf("workspaceURL = %q", inputs.workspaceURL)
	}
	if inputs.botToken != "xoxb-secret" {
		t.Errorf("botToken = %q", inputs.botToken)
	}
	if inputs.appToken != "xapp-secret" {
		t.Errorf("appToken = %q", inputs.appToken)
	}
}

func TestReadSecretLineRejectsNonTTYInput(t *testing.T) {
	_, err := defaultReadSecretLine(bytes.NewBufferString("xoxb-secret\n"), "Bot Token: ", &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected non-TTY secret input to fail")
	}
	if se, ok := err.(*errs.SlackError); !ok || se.Err != "non_tty_secret_input" {
		t.Fatalf("err = %#v, want non_tty_secret_input SlackError", err)
	}
}
