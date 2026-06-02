package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prime-radiant-inc/slackline/errs"
	"golang.org/x/term"
)

var readSecretLine = defaultReadSecretLine

func defaultReadSecretLine(stdin io.Reader, prompt string, output io.Writer) (string, error) {
	file, ok := stdin.(*os.File)
	if !ok {
		return "", nonTTYSecretInputError()
	}
	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		return "", nonTTYSecretInputError()
	}
	_, _ = fmt.Fprint(output, prompt)
	data, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(output)
	if err != nil {
		return "", &errs.SlackError{Code: errs.Usage, Err: "read_secret_failed", Detail: err.Error()}
	}
	return strings.TrimSpace(string(data)), nil
}

func nonTTYSecretInputError() error {
	return &errs.SlackError{
		Code:   errs.Usage,
		Err:    "non_tty_secret_input",
		Detail: "interactive token prompts require a terminal so input is not echoed; use environment variables for non-interactive setup",
	}
}
