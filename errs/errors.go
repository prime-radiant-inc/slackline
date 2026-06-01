package errs

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	Success  = 0
	SlackAPI = 1
	Auth     = 2
	Config   = 3
	Usage    = 4
	Timeout  = 5
)

// Error code strings used in SlackError.Err. These are wire format — they
// appear in machine-readable JSON output and are keyed on by callers/tests.
const (
	CodeConfigError    = "config_error"
	CodeNoToken        = "no_token"
	CodeMissingToken   = "missing_token"
	CodeInvalidToken   = "invalid_token"
	CodeAuthTestFailed = "auth_test_failed"
	CodeSaveFailed     = "save_failed"
)

// SlackError represents an error with an associated exit code.
type SlackError struct {
	Code   int
	Err    string
	Detail string
}

func (e *SlackError) Error() string {
	return fmt.Sprintf("%s: %s", e.Err, e.Detail)
}

func (e *SlackError) ExitCode() int {
	return e.Code
}

// WriteError writes a JSON error object to w with a trailing newline.
func WriteError(w io.Writer, errCode string, detail string) {
	obj := struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}{Error: errCode, Detail: detail}
	data, _ := json.Marshal(obj)
	_, _ = fmt.Fprintln(w, string(data))
}

// AuthError returns a SlackError for authentication failures.
func AuthError(slackErr string) error {
	return &SlackError{
		Code:   Auth,
		Err:    slackErr,
		Detail: "Token invalid or revoked. Run 'slackline init' to reconfigure.",
	}
}
