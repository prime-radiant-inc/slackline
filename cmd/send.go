package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
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
	sendAttach  []string
)

const defaultMaxUploadBytes = int64(100 * 1024 * 1024)

func init() {
	sendCmd.Flags().StringVar(&sendChannel, "channel", "", "channel name (#ops), ID (C...), or Slack URL (required)")
	sendCmd.Flags().StringVar(&sendMessage, "message", "", "message text (reads stdin if omitted; optional when --attach is used)")
	sendCmd.Flags().StringVar(&sendThread, "thread", "", "thread timestamp to reply to")
	sendCmd.Flags().StringArrayVar(&sendAttach, "attach", nil, "attach a file by path (repeatable)")
	_ = sendCmd.MarkFlagRequired("channel")
	rootCmd.AddCommand(sendCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message (and optionally one or more files) to a Slack channel",
	Long:  "Send a message to a channel. Message can be passed via --message, piped via stdin, or omitted entirely when one or more --attach flags are present.",
	RunE: func(cmd *cobra.Command, args []string) error {
		text := sendMessage
		if text == "" && len(sendAttach) == 0 {
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) != 0 {
				return &errs.SlackError{Code: errs.Usage, Err: "no_message", Detail: "Provide --message, pipe text to stdin, or pass at least one --attach"}
			}
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return &errs.SlackError{Code: errs.Usage, Err: "stdin_read_error", Detail: err.Error()}
			}
			text = strings.TrimRight(string(data), "\n")
		}

		cfg, _, err := loadConfig()
		if err != nil {
			return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
		}
		if cfg.Bot.BotToken == "" {
			return &errs.SlackError{Code: errs.Config, Err: "no_token", Detail: "Run 'slackline init' to set up."}
		}
		api := slackpkg.NewClient(cfg.Bot.BotToken)
		channelID, err := resolveChannel(api, sendChannel)
		if err != nil {
			return err
		}

		return runSendWithAPI(api, channelID, text, sendThread, sendAttach, cmd.OutOrStdout())
	},
}

// runSendWithAPI is the testable core. attachPaths == nil → text-only path.
func runSendWithAPI(api slackpkg.SlackAPI, channelID, text, threadTS string, attachPaths []string, stdout io.Writer) error {
	if len(attachPaths) == 0 {
		if text == "" {
			return &errs.SlackError{Code: errs.Usage, Err: "empty_message", Detail: "Message cannot be empty when no --attach is provided"}
		}
		opts := []goslack.MsgOption{goslack.MsgOptionText(text, false)}
		if threadTS != "" {
			opts = append(opts, goslack.MsgOptionTS(threadTS))
		}
		respChan, ts, err := api.PostMessage(channelID, opts...)
		if err != nil {
			if isAuthError(err) {
				return errs.AuthError(err.Error())
			}
			return &errs.SlackError{Code: errs.SlackAPI, Err: "send_failed", Detail: err.Error()}
		}
		return writeSendJSON(stdout, respChan, ts, threadTS, nil)
	}

	if err := validateAttachments(attachPaths); err != nil {
		return err
	}

	uploads := make([]slackpkg.FileUpload, len(attachPaths))
	for i, p := range attachPaths {
		uploads[i] = slackpkg.FileUpload{Path: p}
	}
	results, err := api.UploadFiles(channelID, threadTS, text, uploads)
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "upload_failed", Detail: err.Error()}
	}
	files := make([]map[string]string, len(results))
	for i, r := range results {
		files[i] = map[string]string{"id": r.ID, "title": r.Title}
	}
	return writeSendJSON(stdout, channelID, "", threadTS, files)
}

func validateAttachments(paths []string) error {
	cap := defaultMaxUploadBytes
	if v := os.Getenv("SLACKLINE_MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cap = n
		}
	}
	var total int64
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "attach_not_found", Detail: fmt.Sprintf("%s: %v", p, err)}
		}
		if !st.Mode().IsRegular() {
			return &errs.SlackError{Code: errs.Usage, Err: "attach_not_regular", Detail: fmt.Sprintf("%s is not a regular file", p)}
		}
		total += st.Size()
	}
	if total > cap {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "upload_size_exceeded",
			Detail: fmt.Sprintf("combined upload size %d exceeds cap %d (override with SLACKLINE_MAX_UPLOAD_BYTES)", total, cap),
		}
	}
	return nil
}

func writeSendJSON(stdout io.Writer, channelID, ts, threadTS string, files []map[string]string) error {
	out := struct {
		OK       bool                `json:"ok"`
		Channel  string              `json:"channel"`
		TS       string              `json:"ts,omitempty"`
		ThreadTS string              `json:"thread_ts,omitempty"`
		Files    []map[string]string `json:"files,omitempty"`
	}{OK: true, Channel: channelID, TS: ts, ThreadTS: threadTS, Files: files}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
