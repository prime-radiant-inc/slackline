package cmd

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/spf13/cobra"
)

// newUsageTestRoot builds a command tree wired with slackline's usage-error
// handling. The "do" child declares a required flag and takes exactly one
// positional arg, mirroring how real subcommands state their requirements, so
// the tests drive the same cobra failure paths the CLI hits. "boom" returns a
// plain runtime error; "slackfail" returns a typed *errs.SlackError.
func newUsageTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "tool"}
	configureUsageErrors(root)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	do := &cobra.Command{
		Use:  "do",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}
	do.Flags().String("channel", "", "target channel")
	_ = do.MarkFlagRequired("channel")
	root.AddCommand(do)

	root.AddCommand(&cobra.Command{
		Use:  "boom",
		RunE: func(_ *cobra.Command, _ []string) error { return errors.New("kaboom") },
	})

	root.AddCommand(&cobra.Command{
		Use: "slackfail",
		RunE: func(_ *cobra.Command, _ []string) error {
			return &errs.SlackError{Code: errs.Auth, Err: errs.CodeAuthTestFailed, Detail: "token revoked"}
		},
	})

	return root
}

type errorLine struct {
	code   string
	detail string
}

// runExec drives executeWith against args and returns the exit code and the
// decoded text error line (nil when stderr was empty).
func runExec(t *testing.T, args ...string) (int, *errorLine) {
	t.Helper()
	root := newUsageTestRoot()
	root.SetArgs(args)
	var stderr bytes.Buffer
	code := executeWith(root, &stderr)
	trimmed := bytes.TrimSpace(stderr.Bytes())
	if len(trimmed) == 0 {
		return code, nil
	}
	line := string(trimmed)
	const prefix = "error: "
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("stderr is not a text error line: %q", stderr.String())
	}
	rest := strings.TrimPrefix(line, prefix)
	codePart, detail, ok := strings.Cut(rest, ": ")
	if !ok {
		t.Fatalf("stderr missing code/detail separator: %q", stderr.String())
	}
	return code, &errorLine{code: codePart, detail: detail}
}

func assertUsage(t *testing.T, code int, line *errorLine) {
	t.Helper()
	if code != errs.Usage {
		t.Errorf("exit code = %d, want %d (errs.Usage)", code, errs.Usage)
	}
	if line.code != errs.CodeUsageError {
		t.Errorf("error token = %q, want %q", line.code, errs.CodeUsageError)
	}
	if line.detail == "" {
		t.Error("detail should carry cobra's message, got empty")
	}
}

func TestExecute_MissingRequiredFlagExitsUsage(t *testing.T) {
	code, env := runExec(t, "do", "arg1") // --channel required, omitted
	assertUsage(t, code, env)
}

func TestExecute_UnknownFlagExitsUsage(t *testing.T) {
	code, env := runExec(t, "do", "--channel", "c", "arg1", "--bogus")
	assertUsage(t, code, env)
}

func TestExecute_UnknownCommandExitsUsage(t *testing.T) {
	code, env := runExec(t, "nope")
	assertUsage(t, code, env)
}

func TestExecute_TooManyArgsExitsUsage(t *testing.T) {
	code, env := runExec(t, "do", "--channel", "c", "a", "b")
	assertUsage(t, code, env)
}

func TestExecute_MissingArgsExitsUsage(t *testing.T) {
	code, env := runExec(t, "do", "--channel", "c") // ExactArgs(1), zero given
	assertUsage(t, code, env)
}

func TestExecute_RuntimeErrorExitsOneUnchanged(t *testing.T) {
	code, line := runExec(t, "boom")
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (runtime failure unchanged)", code)
	}
	if line.code != "unknown_error" {
		t.Errorf("error token = %q, want %q", line.code, "unknown_error")
	}
	if line.detail != "kaboom" {
		t.Errorf("detail = %q, want %q", line.detail, "kaboom")
	}
}

func TestExecute_SlackErrorKeepsItsCode(t *testing.T) {
	code, line := runExec(t, "slackfail")
	if code != errs.Auth {
		t.Errorf("exit code = %d, want %d (errs.Auth)", code, errs.Auth)
	}
	if line.code != errs.CodeAuthTestFailed {
		t.Errorf("error token = %q, want %q", line.code, errs.CodeAuthTestFailed)
	}
}

func TestExecute_SuccessExitsZero(t *testing.T) {
	code, line := runExec(t, "do", "--channel", "c", "arg1")
	if code != errs.Success {
		t.Errorf("exit code = %d, want 0", code)
	}
	if line != nil {
		t.Errorf("success should write no error line, got %v", line)
	}
}
