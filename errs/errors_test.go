package errs

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteError_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, "channel_not_found", "Could not find channel #nonexistent")
	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nbody: %s", err, buf.String())
	}
	if got["error"] != "channel_not_found" {
		t.Errorf("error = %q, want %q", got["error"], "channel_not_found")
	}
	if got["detail"] != "Could not find channel #nonexistent" {
		t.Errorf("detail = %q, want %q", got["detail"], "Could not find channel #nonexistent")
	}
}

func TestWriteError_TrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, "test", "test detail")
	out := buf.String()
	if out[len(out)-1] != '\n' {
		t.Error("output should end with newline")
	}
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"Success", Success, 0},
		{"SlackAPI", SlackAPI, 1},
		{"Auth", Auth, 2},
		{"Config", Config, 3},
		{"Usage", Usage, 4},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestSlackError_ImplementsError(t *testing.T) {
	err := &SlackError{Code: SlackAPI, Err: "channel_not_found", Detail: "not found"}
	if err.Error() != "channel_not_found: not found" {
		t.Errorf("Error() = %q, want %q", err.Error(), "channel_not_found: not found")
	}
	if err.ExitCode() != SlackAPI {
		t.Errorf("ExitCode() = %d, want %d", err.ExitCode(), SlackAPI)
	}
}

func TestAuthError(t *testing.T) {
	err := AuthError("token_revoked")
	se, ok := err.(*SlackError)
	if !ok {
		t.Fatal("AuthError should return *SlackError")
	}
	if se.ExitCode() != Auth {
		t.Errorf("ExitCode() = %d, want %d", se.ExitCode(), Auth)
	}
	if se.Detail != "Token invalid or revoked. Run 'slackline init' to reconfigure." {
		t.Errorf("unexpected detail: %s", se.Detail)
	}
}
