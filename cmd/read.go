package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	readChannel string
	readLimit   int
	readThread  string
	readSince   string
)

func init() {
	readCmd.Flags().StringVar(&readChannel, "channel", "", "channel name (#ops), ID (C...), or Slack URL (required)")
	readCmd.Flags().IntVar(&readLimit, "limit", 20, "maximum number of messages to return")
	readCmd.Flags().StringVar(&readThread, "thread", "", "thread timestamp to read replies from")
	readCmd.Flags().StringVar(&readSince, "since", "", "only return messages after this ISO 8601 timestamp")
	_ = readCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(readCmd)
}

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read messages from a Slack channel",
	Long:  "Read messages from a channel or thread. Output is JSONL (one message per line).",
	RunE:  runRead,
}

// messageOutput is the JSONL output format for each message.
type messageOutput struct {
	TS       string `json:"ts"`
	User     string `json:"user"`
	Text     string `json:"text"`
	ThreadTS string `json:"thread_ts,omitempty"`
}

func runRead(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "No bot token configured. Run 'slackline init' to set up."}
	}

	api := slackpkg.NewClient(cfg.Bot.BotToken)
	resolver := slackpkg.NewResolver(api)
	channelID, err := resolver.Resolve(readChannel)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "channel_not_found", Detail: err.Error()}
	}

	var oldest string
	if readSince != "" {
		t, err := time.Parse(time.RFC3339, readSince)
		if err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "invalid_since", Detail: fmt.Sprintf("Failed to parse --since as ISO 8601: %v", err)}
		}
		oldest = fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
	}

	var messages []goslack.Message
	if readThread != "" {
		messages, err = fetchReplies(api, channelID, readThread, oldest, readLimit)
	} else {
		messages, err = fetchHistory(api, channelID, oldest, readLimit)
	}
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "read_failed", Detail: err.Error()}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	for _, m := range messages {
		threadTS := m.ThreadTimestamp
		if threadTS == m.Timestamp {
			threadTS = ""
		}
		out := messageOutput{
			TS:       m.Timestamp,
			User:     m.User,
			Text:     m.Text,
			ThreadTS: threadTS,
		}
		if err := enc.Encode(out); err != nil {
			return err
		}
	}
	return nil
}

// fetchHistory retrieves channel messages with pagination, returning up to
// limit messages in chronological order (oldest first).
func fetchHistory(api slackpkg.SlackAPI, channelID, oldest string, limit int) ([]goslack.Message, error) {
	var all []goslack.Message
	cursor := ""
	for {
		remaining := limit - len(all)
		if remaining <= 0 {
			break
		}
		pageSize := remaining
		if pageSize > 100 {
			pageSize = 100
		}
		resp, err := api.GetConversationHistory(&goslack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Cursor:    cursor,
			Limit:     pageSize,
			Oldest:    oldest,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Messages...)
		if !resp.HasMore || resp.ResponseMetaData.NextCursor == "" {
			break
		}
		cursor = resp.ResponseMetaData.NextCursor
	}
	// Slack returns reverse-chronological; reverse to chronological.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	// Truncate to limit after reversing (pagination may overshoot).
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// fetchReplies retrieves thread replies with pagination, returning up to
// limit messages in chronological order. Slack returns replies in
// chronological order already.
func fetchReplies(api slackpkg.SlackAPI, channelID, threadTS, oldest string, limit int) ([]goslack.Message, error) {
	var all []goslack.Message
	cursor := ""
	for {
		remaining := limit - len(all)
		if remaining <= 0 {
			break
		}
		pageSize := remaining
		if pageSize > 100 {
			pageSize = 100
		}
		msgs, hasMore, nextCursor, err := api.GetConversationReplies(&goslack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Cursor:    cursor,
			Limit:     pageSize,
			Oldest:    oldest,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, msgs...)
		if !hasMore || nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}
