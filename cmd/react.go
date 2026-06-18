package cmd

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	reactChannel string
	reactTS      string
	reactEmoji   string
	reactFormat  string
)

var reactCmd = &cobra.Command{
	Use:   "react",
	Short: "Add or remove emoji reactions on a message",
}

var reactAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a reaction to a message",
	RunE: func(cmd *cobra.Command, args []string) error {
		api, err := loadBotAPI()
		if err != nil {
			return err
		}
		channelID, err := resolveChannel(api, reactChannel)
		if err != nil {
			return err
		}
		outputFormat, err := parseOutputFormat(reactFormat)
		if err != nil {
			return err
		}
		return runReactAddWithAPIFormat(api, channelID, reactTS, reactEmoji, outputFormat, cmd.OutOrStdout())
	},
}

var reactRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a reaction from a message",
	RunE: func(cmd *cobra.Command, args []string) error {
		api, err := loadBotAPI()
		if err != nil {
			return err
		}
		channelID, err := resolveChannel(api, reactChannel)
		if err != nil {
			return err
		}
		outputFormat, err := parseOutputFormat(reactFormat)
		if err != nil {
			return err
		}
		return runReactRemoveWithAPIFormat(api, channelID, reactTS, reactEmoji, outputFormat, cmd.OutOrStdout())
	},
}

func init() {
	reactCmd.PersistentFlags().StringVar(&reactFormat, "format", outputFormatText, "output format: text or json")
	for _, sub := range []*cobra.Command{reactAddCmd, reactRemoveCmd} {
		sub.Flags().StringVar(&reactChannel, "channel", "", "channel name (#ops), ID, or URL (required)")
		sub.Flags().StringVar(&reactTS, "ts", "", "message timestamp (required)")
		sub.Flags().StringVar(&reactEmoji, "emoji", "", "emoji name without colons (required)")
		_ = sub.MarkFlagRequired("channel")
		_ = sub.MarkFlagRequired("ts")
		_ = sub.MarkFlagRequired("emoji")
		reactCmd.AddCommand(sub)
	}
	rootCmd.AddCommand(reactCmd)
}

// loadBotAPI loads config and returns a bot-token SlackAPI.
func loadBotAPI() (slackpkg.SlackAPI, error) {
	cfg, _, err := loadConfig()
	if err != nil {
		return nil, &errs.SlackError{Code: errs.Config, Err: errs.CodeConfigError, Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return nil, &errs.SlackError{Code: errs.Config, Err: errs.CodeNoToken, Detail: "Run 'slackline init' first."}
	}
	return slackpkg.NewClient(cfg.Bot.BotToken), nil
}

// resolveChannel converts a flag value (#name | C123 | URL) to a channel ID.
func resolveChannel(api slackpkg.SlackAPI, flagValue string) (string, error) {
	resolver := slackpkg.NewResolver(api)
	id, err := resolver.Resolve(flagValue)
	if err != nil {
		return "", &errs.SlackError{Code: errs.SlackAPI, Err: "channel_not_found", Detail: err.Error()}
	}
	return id, nil
}

// stripEmojiColons returns "thumbsup" for inputs like ":thumbsup:" or " :thumbsup: ".
func stripEmojiColons(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, ":")
	s = strings.TrimSuffix(s, ":")
	return s
}

// runReactAddWithAPI is the testable core of `react add`.
func runReactAddWithAPI(api slackpkg.SlackAPI, channelID, ts, emoji string, stdout io.Writer) error {
	return runReactAddWithAPIFormat(api, channelID, ts, emoji, outputFormatText, stdout)
}

func runReactAddWithAPIFormat(api slackpkg.SlackAPI, channelID, ts, emoji, outputFormat string, stdout io.Writer) error {
	emoji = stripEmojiColons(emoji)
	err := api.AddReaction(emoji, goslack.ItemRef{Channel: channelID, Timestamp: ts})
	noOp, reactErr := classifyReactionErr(err, "already_reacted", "react_add_failed")
	if reactErr != nil {
		return reactErr
	}
	return writeReactOutput(stdout, outputFormat, channelID, ts, emoji, "added", noOp)
}

// runReactRemoveWithAPI is the testable core of `react remove`.
func runReactRemoveWithAPI(api slackpkg.SlackAPI, channelID, ts, emoji string, stdout io.Writer) error {
	return runReactRemoveWithAPIFormat(api, channelID, ts, emoji, outputFormatText, stdout)
}

func runReactRemoveWithAPIFormat(api slackpkg.SlackAPI, channelID, ts, emoji, outputFormat string, stdout io.Writer) error {
	emoji = stripEmojiColons(emoji)
	err := api.RemoveReaction(emoji, goslack.ItemRef{Channel: channelID, Timestamp: ts})
	noOp, reactErr := classifyReactionErr(err, "no_reaction", "react_remove_failed")
	if reactErr != nil {
		return reactErr
	}
	return writeReactOutput(stdout, outputFormat, channelID, ts, emoji, "removed", noOp)
}

// classifyReactionErr maps a Slack reaction error to (noOp, error).
// idempotentCode is the Slack error string that should be treated as a no-op success.
// failCode is the errs.SlackError code for all other failures.
func classifyReactionErr(err error, idempotentCode, failCode string) (noOp bool, classified error) {
	if err == nil {
		return false, nil
	}
	if strings.Contains(err.Error(), idempotentCode) {
		return true, nil
	}
	if isAuthError(err) {
		return false, errs.AuthError(err.Error())
	}
	return false, &errs.SlackError{Code: errs.SlackAPI, Err: failCode, Detail: err.Error()}
}

func writeReactOutput(stdout io.Writer, outputFormat, channelID, ts, emoji, action string, noOp bool) error {
	if outputFormat != outputFormatJSON {
		return nil
	}
	out := struct {
		OK      bool   `json:"ok"`
		NoOp    bool   `json:"no_op,omitempty"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
		Emoji   string `json:"emoji"`
		Action  string `json:"action"`
	}{OK: true, NoOp: noOp, Channel: channelID, TS: ts, Emoji: emoji, Action: action}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
