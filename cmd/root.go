package cmd

import (
	"errors"
	"os"

	"github.com/prime-radiant/slackline/config"
	"github.com/prime-radiant/slackline/errs"
	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "slackline",
	Short: "Give AI agents a Slack identity",
	Long:  "A CLI tool for AI agents to send messages, read channels, and listen for events in Slack.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/slackline/config.json)")
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
	if err := rootCmd.Execute(); err != nil {
		var se *errs.SlackError
		if errors.As(err, &se) {
			errs.WriteError(os.Stderr, se.Err, se.Detail)
			return se.ExitCode()
		}
		errs.WriteError(os.Stderr, "unknown_error", err.Error())
		return 1
	}
	return 0
}
