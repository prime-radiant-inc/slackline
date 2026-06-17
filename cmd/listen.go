package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/prime-radiant-inc/slackline/listen"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	"github.com/spf13/cobra"
)

var (
	listenIncludeBotSelf bool
	listenThreads        bool
	listenAllMessages    bool
	listenTypes          string
	listenFormat         string
)

func init() {
	listenCmd.Flags().BoolVar(&listenIncludeBotSelf, "include-bot-self", false, "include events authored by the bot itself (default: filtered)")
	listenCmd.Flags().BoolVar(&listenThreads, "threads", false, "(no-op since v0.2.1) bot-parent thread replies are emitted by default; kept for backward compatibility")
	listenCmd.Flags().BoolVar(&listenAllMessages, "all-messages", false, "firehose: emit every message in every channel the bot is in (implies --threads)")
	listenCmd.Flags().StringVar(&listenTypes, "type", "", "comma-separated event types to emit: mention, dm, thread_reply, channel_message, reaction (default: all). Emit-time filter — does not widen subscription; channel_message requires --all-messages")
	listenCmd.Flags().StringVar(&listenFormat, "format", outputFormatText, "output format: text or json")
	rootCmd.AddCommand(listenCmd)
}

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Listen for real-time Slack events",
	Long: `Connect via Socket Mode and stream events to stdout, one event per line.
Default output is compact text; pass --format json for the JSONL event shape.

Connection status goes to stderr: connecting/connected (websocket open),
"ready" (subscribed — events will now flow), reconnecting, disconnected.
Wait for "ready" before expecting events.

Use --type to emit only specific event types.

Default text examples:
  mention C123 U123 100.001 <@UBOT> hello
  dm D123 U123 100.002 hello
  thread_reply C123 U123 100.003 thread=100.001 parent=UBOT reply
  reaction added C123 U123 item=100.001 thumbsup
  file F123 report.pdf 12345 application/pdf Q4 Report

JSON fields with --format json:
  mention, dm, thread_reply, channel_message:
    type, channel, user, text, ts, thread_ts (if a reply), parent_user_id, files
  reaction:
    type, action ("added"|"removed"), channel, user, emoji, item_ts (the reacted-to message)

Files arrive on message-family events (Slack subtype "file_share"), never on
"mention" events. Download them with: slackline download --file <id> --out <path>.`,
	RunE: runListen,
}

var validListenTypes = map[string]bool{
	listen.EventTypeMention:        true,
	listen.EventTypeDM:             true,
	listen.EventTypeThreadReply:    true,
	listen.EventTypeChannelMessage: true,
	listen.EventTypeReaction:       true,
}

// parseListenTypes turns the comma-separated --type value into an allowlist set.
// Empty input returns (nil, nil) meaning "emit all". Unknown types and the
// channel_message-without-firehose case are usage errors.
func parseListenTypes(raw string, allMessages bool) (map[string]bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	set := map[string]bool{}
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !validListenTypes[t] {
			return nil, &errs.SlackError{Code: errs.Usage, Err: "invalid_type", Detail: fmt.Sprintf("unknown --type %q; valid: mention, dm, thread_reply, channel_message, reaction", t)}
		}
		set[t] = true
	}
	if set[listen.EventTypeChannelMessage] && !allMessages {
		return nil, &errs.SlackError{Code: errs.Usage, Err: "invalid_type", Detail: "channel_message events require --all-messages"}
	}
	return set, nil
}

func runListen(cmd *cobra.Command, args []string) error {
	types, err := parseListenTypes(listenTypes, listenAllMessages)
	if err != nil {
		return err
	}
	outputFormat, err := parseOutputFormat(listenFormat)
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

	listener := listen.NewListener(cfg.Bot.BotToken, cfg.Bot.AppToken, authResp.UserID, authResp.BotID, listen.ListenerOptions{
		IncludeBotSelf: listenIncludeBotSelf,
		Threads:        listenThreads || listenAllMessages,
		AllMessages:    listenAllMessages,
		Types:          types,
		OutputFormat:   outputFormat,
	}, os.Stdout, os.Stderr)
	if err := listener.Run(); err != nil {
		return classifyListenRunError(err)
	}
	return nil
}

func classifyListenRunError(err error) error {
	if isAuthError(err) {
		return errs.AuthError(err.Error())
	}
	return &errs.SlackError{Code: errs.SlackAPI, Err: "socket_mode_failed", Detail: err.Error()}
}
