package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	askChannel string
	askMessage string
	askTimeout int
	askPoll    int
)

var askCmd = &cobra.Command{
	Use:   "ask",
	Short: "Send a message and wait for a reply",
	Long:  "Sends a message to a channel and polls the thread for replies from other users. Exits 0 when a reply is received, exits 1 on timeout.",
	RunE:  runAsk,
}

func init() {
	askCmd.Flags().StringVar(&askChannel, "channel", "", "channel name (#name), ID (C...), or Slack URL (required)")
	askCmd.Flags().StringVar(&askMessage, "message", "", "message text (reads from stdin if omitted)")
	askCmd.Flags().IntVar(&askTimeout, "timeout", 300, "seconds to wait for a reply before timing out")
	askCmd.Flags().IntVar(&askPoll, "poll", 10, "seconds between poll attempts")
	_ = askCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(askCmd)
}

func runAsk(cmd *cobra.Command, args []string) error {
	text := askMessage
	if text == "" {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "stdin_error", Detail: err.Error()}
		}
		if fi.Mode()&os.ModeCharDevice != 0 {
			return &errs.SlackError{Code: errs.Usage, Err: "no_message", Detail: "Provide --message or pipe text to stdin."}
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "stdin_error", Detail: err.Error()}
		}
		text = strings.TrimRight(string(data), "\n")
	}
	if text == "" {
		return &errs.SlackError{Code: errs.Usage, Err: "empty_message", Detail: "Message cannot be empty."}
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "missing_token", Detail: "No bot token configured. Run 'slackline init'."}
	}

	api := slackpkg.NewClient(cfg.Bot.BotToken)

	// Get bot user ID for self-filtering.
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: err.Error()}
	}
	botUserID := authResp.UserID

	resolver := slackpkg.NewResolver(api)
	channelID, err := resolver.Resolve(askChannel)
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "channel_resolve_error", Detail: err.Error()}
	}

	_, ts, err := api.PostMessage(channelID, goslack.MsgOptionText(text, false))
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "send_failed", Detail: err.Error()}
	}

	// Poll for replies.
	deadline := time.Now().Add(time.Duration(askTimeout) * time.Second)
	pollInterval := time.Duration(askPoll) * time.Second

	for {
		time.Sleep(pollInterval)
		if time.Now().After(deadline) {
			return &errs.SlackError{Code: errs.SlackAPI, Err: "timeout", Detail: fmt.Sprintf("No reply received within %d seconds.", askTimeout)}
		}

		msgs, _, _, err := api.GetConversationReplies(&goslack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: ts,
		})
		if err != nil {
			if isAuthError(err) {
				return errs.AuthError(err.Error())
			}
			return &errs.SlackError{Code: errs.SlackAPI, Err: "poll_failed", Detail: err.Error()}
		}

		var replies []goslack.Message
		for _, m := range msgs {
			// Skip parent message.
			if m.Timestamp == ts {
				continue
			}
			// Skip bot's own messages.
			if m.User == botUserID {
				continue
			}
			replies = append(replies, m)
		}

		if len(replies) > 0 {
			for _, m := range replies {
				writeMessage(os.Stdout, m)
			}
			return nil
		}
	}
}

// isAuthError returns true if the error indicates an authentication failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not_authed") ||
		strings.Contains(msg, "invalid_auth") ||
		strings.Contains(msg, "token_revoked") ||
		strings.Contains(msg, "account_inactive")
}

// writeMessage writes a message as a compact JSONL line to w.
func writeMessage(w io.Writer, m goslack.Message) {
	obj := map[string]string{
		"ts":   m.Timestamp,
		"user": m.User,
		"text": m.Text,
	}
	if m.ThreadTimestamp != "" {
		obj["thread_ts"] = m.ThreadTimestamp
	}
	data, _ := json.Marshal(obj)
	_, _ = fmt.Fprintln(w, string(data))
}
