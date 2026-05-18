package cmd

import (
	"os"

	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/prime-radiant-inc/slackline/listen"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	"github.com/spf13/cobra"
)

var (
	listenIncludeBotSelf bool
	listenThreads        bool
	listenAllMessages    bool
)

func init() {
	listenCmd.Flags().BoolVar(&listenIncludeBotSelf, "include-bot-self", false, "include events authored by the bot itself (default: filtered)")
	listenCmd.Flags().BoolVar(&listenThreads, "threads", false, "(no-op since v0.2.1) bot-parent thread replies are emitted by default; kept for backward compatibility")
	listenCmd.Flags().BoolVar(&listenAllMessages, "all-messages", false, "firehose: emit every message in every channel the bot is in (implies --threads)")
	rootCmd.AddCommand(listenCmd)
}

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Listen for real-time Slack events",
	Long:  "Connect via Socket Mode and stream @mentions, DMs, and reactions as JSONL to stdout.",
	RunE:  runListen,
}

func runListen(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeConfigError, Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeNoToken, Detail: "No bot token configured. Run 'slackline init' to set up."}
	}
	if cfg.Bot.AppToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_app_token", Detail: "No app token configured. Socket Mode requires an app token (xapp-)."}
	}

	// Get bot user ID for self-filtering
	api := slackpkg.NewClient(cfg.Bot.BotToken)
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: errs.CodeAuthTestFailed, Detail: err.Error()}
	}

	listener := listen.NewListener(cfg.Bot.BotToken, cfg.Bot.AppToken, authResp.UserID, listen.ListenerOptions{
		IncludeBotSelf: listenIncludeBotSelf,
		Threads:        listenThreads || listenAllMessages,
		AllMessages:    listenAllMessages,
	}, os.Stdout, os.Stderr)
	return listener.Run()
}
