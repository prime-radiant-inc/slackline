package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	sendChannel string
	sendMessage string
	sendThread  string
)

func init() {
	sendCmd.Flags().StringVar(&sendChannel, "channel", "", "channel name (#ops), ID (C...), or Slack URL (required)")
	sendCmd.Flags().StringVar(&sendMessage, "message", "", "message text (reads stdin if omitted)")
	sendCmd.Flags().StringVar(&sendThread, "thread", "", "thread timestamp to reply to")
	_ = sendCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(sendCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to a Slack channel",
	Long:  "Send a message to a channel. Message can be passed via --message or piped via stdin.",
	RunE:  runSend,
}

func runSend(cmd *cobra.Command, args []string) error {
	text := sendMessage
	if text == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return &errs.SlackError{Code: errs.Usage, Err: "no_message", Detail: "Provide --message or pipe message via stdin"}
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "stdin_read_error", Detail: fmt.Sprintf("Failed to read stdin: %v", err)}
		}
		text = strings.TrimRight(string(data), "\n")
	}
	if text == "" {
		return &errs.SlackError{Code: errs.Usage, Err: "empty_message", Detail: "Message cannot be empty"}
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "No bot token configured. Run 'slackline init' to set up."}
	}

	api := slackpkg.NewClient(cfg.Bot.BotToken)
	resolver := slackpkg.NewResolver(api)
	channelID, err := resolver.Resolve(sendChannel)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "channel_not_found", Detail: err.Error()}
	}

	opts := []goslack.MsgOption{goslack.MsgOptionText(text, false)}
	if sendThread != "" {
		opts = append(opts, goslack.MsgOptionTS(sendThread))
	}

	respChannel, ts, err := api.PostMessage(channelID, opts...)
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "send_failed", Detail: err.Error()}
	}

	out := struct {
		OK       bool   `json:"ok"`
		Channel  string `json:"channel"`
		TS       string `json:"ts"`
		ThreadTS string `json:"thread_ts,omitempty"`
	}{OK: true, Channel: respChannel, TS: ts, ThreadTS: sendThread}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
