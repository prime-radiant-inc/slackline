package errs

import (
	"bytes"
	"testing"
)

func TestWriteError_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, "channel_not_found", "Could not find channel #nonexistent")
	want := "error: channel_not_found: Could not find channel #nonexistent\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
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

func TestTimeoutExitCode(t *testing.T) {
	if Timeout != 5 {
		t.Errorf("Timeout = %d, want 5", Timeout)
	}
	// Guard the rest of the taxonomy so the codes stay stable.
	if Success != 0 || SlackAPI != 1 || Auth != 2 || Config != 3 || Usage != 4 {
		t.Error("exit-code taxonomy 0-4 changed unexpectedly")
	}
}
