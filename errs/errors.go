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
	fmt.Fprintln(w, string(data))
}

// AuthError returns a SlackError for authentication failures.
func AuthError(slackErr string) error {
	return &SlackError{
		Code:   Auth,
		Err:    slackErr,
		Detail: "Token invalid or revoked. Run 'slackline init' to reconfigure.",
	}
}
