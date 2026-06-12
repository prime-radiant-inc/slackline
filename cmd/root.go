package cmd

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/prime-radiant-inc/slackline/config"
	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "slackline",
	Short: "Give AI agents a Slack identity",
	Long:  "A CLI tool for AI agents to send messages, read channels, and listen for events in Slack.",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/slackline/config.json)")
	configureUsageErrors(rootCmd)
}

// configureUsageErrors makes cobra's own usage failures surface as slackline's
// JSON error envelope with exit code errs.Usage, instead of cobra's default
// human-readable usage text. Flag-parse failures (unknown or malformed flags)
// are converted here via SetFlagErrorFunc; the remaining usage failures
// (missing required flag, unknown subcommand, wrong argument count) come back
// from Command.Execute as plain errors and are classified by isCobraUsageError.
func configureUsageErrors(c *cobra.Command) {
	c.SilenceErrors = true
	c.SilenceUsage = true
	c.SetFlagErrorFunc(usageFlagError)
}

// usageFlagError wraps a cobra flag-parse error as a usage SlackError so it maps
// to exit code errs.Usage with the standard JSON envelope.
func usageFlagError(_ *cobra.Command, err error) error {
	return &errs.SlackError{Code: errs.Usage, Err: errs.CodeUsageError, Detail: err.Error()}
}

// isCobraUsageError reports whether err is one of cobra's own usage failures
// that arrive as plain (non-SlackError) errors from Command.Execute: a missing
// required flag, an unknown subcommand, or an argument-count violation. Cobra
// (v1.10.2) builds these with fmt.Errorf and exposes no typed error or
// sentinel, so matching their message shape is the only available mechanism.
// Flag-parse errors do not reach here; usageFlagError converts those first.
// Keep these patterns aligned with cobra's command.go (ValidateRequiredFlags)
// and args.go (the arg-count validators) on upgrade.
func isCobraUsageError(err error) bool {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "required flag(s) "):
		return true
	case strings.HasPrefix(msg, "unknown command "):
		return true
	case strings.Contains(msg, "arg(s), received "), strings.Contains(msg, "arg(s), only received "):
		return true
	default:
		return false
	}
}

// SetVersion stores the build-time version string on the root command.
func SetVersion(v string) {
	rootCmd.Version = v
}

func loadConfig() (*config.Config, string, error) {
	path := cfgFile
	if path == "" {
		path = os.Getenv("SLACKLINE_CONFIG")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, "", err
	}
	if path == "" {
		path = config.DefaultPath()
	}
	return cfg, path, nil
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	return executeWith(rootCmd, os.Stderr)
}

// executeWith runs root and maps any error to slackline's exit-code contract:
// a *errs.SlackError keeps its own code; anything else is an unexpected runtime
// failure (exit 1).
func executeWith(root *cobra.Command, stderr io.Writer) int {
	err := root.Execute()
	if err == nil {
		return errs.Success
	}
	var se *errs.SlackError
	if errors.As(err, &se) {
		errs.WriteError(stderr, se.Err, se.Detail)
		return se.ExitCode()
	}
	if isCobraUsageError(err) {
		errs.WriteError(stderr, errs.CodeUsageError, err.Error())
		return errs.Usage
	}
	errs.WriteError(stderr, "unknown_error", err.Error())
	return 1
}
