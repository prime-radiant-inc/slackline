package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/prime-radiant/slackline/errs"
	"github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	channelsAll  bool
	channelsJSON bool
)

var channelsCmd = &cobra.Command{
	Use:   "channels",
	Short: "List Slack channels visible to the bot",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _, err := loadConfig()
		if err != nil {
			return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
		}
		if cfg.Bot.BotToken == "" {
			return &errs.SlackError{Code: errs.Config, Err: "missing_token", Detail: "No bot token configured. Run 'slackline init' first."}
		}

		client := slack.New(cfg.Bot.BotToken)

		var all []slack.Channel
		cursor := ""
		for {
			params := &slack.GetConversationsParameters{
				Cursor:          cursor,
				ExcludeArchived: true,
				Limit:           200,
				Types:           []string{"public_channel", "private_channel"},
			}
			channels, nextCursor, err := client.GetConversations(params)
			if err != nil {
				if slackErr, ok := err.(slack.SlackErrorResponse); ok && isAuthError(errors.New(slackErr.Err)) {
					return errs.AuthError(slackErr.Err)
				}
				return &errs.SlackError{Code: errs.SlackAPI, Err: "channels_failed", Detail: err.Error()}
			}
			all = append(all, channels...)
			if nextCursor == "" {
				break
			}
			cursor = nextCursor
		}

		// Filter to member channels unless --all is set.
		if !channelsAll {
			filtered := all[:0]
			for _, ch := range all {
				if ch.IsMember {
					filtered = append(filtered, ch)
				}
			}
			all = filtered
		}

		if channelsJSON {
			return writeChannelsJSON(all)
		}
		return writeChannelsTable(all)
	},
}

type channelJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Topic   string `json:"topic"`
	Purpose string `json:"purpose"`
}

func writeChannelsJSON(channels []slack.Channel) error {
	out := make([]channelJSON, len(channels))
	for i, ch := range channels {
		out[i] = channelJSON{
			ID:      ch.ID,
			Name:    ch.Name,
			Topic:   ch.Topic.Value,
			Purpose: ch.Purpose.Value,
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeChannelsTable(channels []slack.Channel) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPURPOSE")
	for _, ch := range channels {
		purpose := ch.Purpose.Value
		if len(purpose) > 60 {
			purpose = purpose[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t#%s\t%s\n", ch.ID, ch.Name, purpose)
	}
	return w.Flush()
}

func init() {
	channelsCmd.Flags().BoolVar(&channelsAll, "all", false, "Show all channels, not just ones the bot has joined")
	channelsCmd.Flags().BoolVar(&channelsJSON, "json", false, "Output as JSON array")
	rootCmd.AddCommand(channelsCmd)
}
