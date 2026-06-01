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
)

func init() {
	listenCmd.Flags().BoolVar(&listenIncludeBotSelf, "include-bot-self", false, "include events authored by the bot itself (default: filtered)")
	listenCmd.Flags().BoolVar(&listenThreads, "threads", false, "(no-op since v0.2.1) bot-parent thread replies are emitted by default; kept for backward compatibility")
	listenCmd.Flags().BoolVar(&listenAllMessages, "all-messages", false, "firehose: emit every message in every channel the bot is in (implies --threads)")
	listenCmd.Flags().StringVar(&listenTypes, "type", "", "comma-separated event types to emit: mention, dm, thread_reply, channel_message, reaction (default: all). Emit-time filter — does not widen subscription; channel_message requires --all-messages")
	rootCmd.AddCommand(listenCmd)
}

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Listen for real-time Slack events",
	Long:  "Connect via Socket Mode and stream events as JSONL to stdout. Use --type to emit only specific event types (mention, dm, thread_reply, channel_message, reaction).",
	RunE:  runListen,
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
		Types:          types,
	}, os.Stdout, os.Stderr)
	return listener.Run()
}
