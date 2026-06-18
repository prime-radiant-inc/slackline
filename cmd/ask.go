package cmd

import (
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
	askFormat  string
)

var askCmd = &cobra.Command{
	Use:   "ask",
	Short: "Send a message and wait for a reply",
	Long:  "Sends a message to a channel and polls the thread for replies from other users. Exits 0 when a reply is received, exits 5 on timeout (1/2/3 for API/auth/config errors).",
	RunE:  runAsk,
}

func init() {
	askCmd.Flags().StringVar(&askChannel, "channel", "", "channel name (#name), ID (C...), or Slack URL (required)")
	askCmd.Flags().StringVar(&askMessage, "message", "", "message text (reads from stdin if omitted)")
	askCmd.Flags().IntVar(&askTimeout, "timeout", 300, "seconds to wait for a reply before timing out")
	askCmd.Flags().IntVar(&askPoll, "poll", 10, "seconds between poll attempts")
	askCmd.Flags().StringVar(&askFormat, "format", outputFormatText, "output format: text or json")
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
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeConfigError, Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeMissingToken, Detail: "No bot token configured. Run 'slackline init'."}
	}

	api := slackpkg.NewClient(cfg.Bot.BotToken)

	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: errs.CodeAuthTestFailed, Detail: err.Error()}
	}

	resolver := slackpkg.NewResolver(api)
	channelID, err := resolver.Resolve(askChannel)
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "channel_resolve_error", Detail: err.Error()}
	}

	outputFormat, err := parseOutputFormat(askFormat)
	if err != nil {
		return err
	}
	return runAskWithAPIFormat(api, channelID, authResp.UserID, text, askTimeout, askPoll, outputFormat, time.Now, time.Sleep, cmd.OutOrStdout())
}

// runAskWithAPI posts text to channelID, then polls the thread until a reply
// from another user arrives (exit 0) or the deadline passes (Timeout). now/sleep
// are injected for deterministic tests; production passes time.Now/time.Sleep.
func runAskWithAPI(api slackpkg.SlackAPI, channelID, botUserID, text string, timeoutSec, pollSec int, now func() time.Time, sleep func(time.Duration), out io.Writer) error {
	return runAskWithAPIFormat(api, channelID, botUserID, text, timeoutSec, pollSec, outputFormatText, now, sleep, out)
}

func runAskWithAPIFormat(api slackpkg.SlackAPI, channelID, botUserID, text string, timeoutSec, pollSec int, outputFormat string, now func() time.Time, sleep func(time.Duration), out io.Writer) error {
	_, ts, err := api.PostMessage(channelID, goslack.MsgOptionText(text, false))
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "send_failed", Detail: err.Error()}
	}

	deadline := now().Add(time.Duration(timeoutSec) * time.Second)
	pollInterval := time.Duration(pollSec) * time.Second

	for {
		sleep(pollInterval)
		if now().After(deadline) {
			return &errs.SlackError{Code: errs.Timeout, Err: "timeout", Detail: fmt.Sprintf("No reply received within %d seconds.", timeoutSec)}
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
				if err := writeAskMessage(out, outputFormat, m); err != nil {
					return err
				}
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

func writeAskMessage(w io.Writer, outputFormat string, m goslack.Message) error {
	threadTS := m.ThreadTimestamp
	if threadTS == m.Timestamp {
		threadTS = ""
	}
	return writeMessageOutput(w, outputFormat, messageOutput{
		TS:       m.Timestamp,
		User:     m.User,
		Text:     m.Text,
		ThreadTS: threadTS,
	})
}

// writeMessage writes a message as a compact JSONL line to w.
func writeMessage(w io.Writer, m goslack.Message) {
	_ = writeAskMessage(w, outputFormatJSON, m)
}
