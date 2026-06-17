package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
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
	readFormat  string
)

const (
	outputFormatText = "text"
	outputFormatJSON = "json"
)

func init() {
	readCmd.Flags().StringVar(&readChannel, "channel", "", "channel name (#ops), ID (C...), or Slack URL (required)")
	readCmd.Flags().IntVar(&readLimit, "limit", 20, "maximum number of messages to return")
	readCmd.Flags().StringVar(&readThread, "thread", "", "thread timestamp to read replies from")
	readCmd.Flags().StringVar(&readSince, "since", "", "only return messages after this ISO 8601 timestamp")
	readCmd.Flags().StringVar(&readFormat, "format", outputFormatText, "output format: text or json")
	_ = readCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(readCmd)
}

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read messages from a Slack channel",
	Long:  "Read messages from a channel or thread. Default output is compact text; pass --format json for JSONL.",
	RunE:  runRead,
}

// messageOutput is the structured form used by text and JSON message output.
type messageOutput struct {
	TS       string         `json:"ts"`
	User     string         `json:"user"`
	Text     string         `json:"text"`
	ThreadTS string         `json:"thread_ts,omitempty"`
	Files    []fileMetaJSON `json:"files,omitempty"`
}

type fileMetaJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Mimetype string `json:"mimetype,omitempty"`
	Size     int    `json:"size"`
	Title    string `json:"title,omitempty"`
}

func runRead(cmd *cobra.Command, args []string) error {
	outputFormat, err := parseOutputFormat(readFormat)
	if err != nil {
		return err
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeConfigError, Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeNoToken, Detail: "No bot token configured. Run 'slackline init' to set up."}
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

	for _, m := range messages {
		threadTS := m.ThreadTimestamp
		if threadTS == m.Timestamp {
			threadTS = ""
		}
		var files []fileMetaJSON
		for _, f := range m.Files {
			files = append(files, fileMetaJSON{
				ID:       f.ID,
				Name:     f.Name,
				Mimetype: f.Mimetype,
				Size:     f.Size,
				Title:    f.Title,
			})
		}
		out := messageOutput{
			TS:       m.Timestamp,
			User:     m.User,
			Text:     m.Text,
			ThreadTS: threadTS,
			Files:    files,
		}
		if err := writeMessageOutput(cmd.OutOrStdout(), outputFormat, out); err != nil {
			return err
		}
	}
	return nil
}

func parseOutputFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", outputFormatText:
		return outputFormatText, nil
	case outputFormatJSON:
		return outputFormatJSON, nil
	default:
		return "", &errs.SlackError{Code: errs.Usage, Err: "invalid_format", Detail: "valid --format values: text, json"}
	}
}

func writeMessageOutput(w io.Writer, outputFormat string, out messageOutput) error {
	if outputFormat == outputFormatJSON {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(out)
	}
	if _, err := fmt.Fprint(w, formatMessageText(out)); err != nil {
		return err
	}
	return nil
}

func formatMessageText(out messageOutput) string {
	var b strings.Builder
	parts := []string{out.TS, out.User}
	if out.ThreadTS != "" {
		parts = append(parts, "thread="+out.ThreadTS)
	}
	b.WriteString(strings.Join(nonEmptyOutputParts(parts), " "))
	if out.Text != "" {
		b.WriteByte(' ')
		b.WriteString(singleLineOutputText(out.Text))
	}
	b.WriteByte('\n')
	for _, f := range out.Files {
		fileParts := []string{"  file", f.ID, f.Name, strconv.Itoa(f.Size), f.Mimetype}
		b.WriteString(strings.Join(nonEmptyOutputParts(fileParts), " "))
		if f.Title != "" {
			b.WriteByte(' ')
			b.WriteString(singleLineOutputText(f.Title))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func nonEmptyOutputParts(parts []string) []string {
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func singleLineOutputText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", `\n`)
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

// fetchReplies retrieves thread replies, returning up to limit messages in
// chronological order, counted from the newest. Slack returns replies
// oldest-first and paginates forward, so the newest replies live on the last
// page: we must follow the cursor to the end of the thread before truncating,
// otherwise the true tail is silently dropped (PRI-1879).
func fetchReplies(api slackpkg.SlackAPI, channelID, threadTS, oldest string, limit int) ([]goslack.Message, error) {
	var all []goslack.Message
	cursor := ""
	for {
		msgs, hasMore, nextCursor, err := api.GetConversationReplies(&goslack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Cursor:    cursor,
			Limit:     100,
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
	// Keep the newest `limit` messages (pagination collects the whole thread).
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}
