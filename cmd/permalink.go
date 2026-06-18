package cmd

import (
	"fmt"
	"io"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	permalinkChannel string
	permalinkTS      string
)

var permalinkCmd = &cobra.Command{
	Use:   "permalink",
	Short: "Get a Slack permalink for a message",
	RunE: func(cmd *cobra.Command, args []string) error {
		api, err := loadBotAPI()
		if err != nil {
			return err
		}
		channelID, err := resolveChannel(api, permalinkChannel)
		if err != nil {
			return err
		}
		return runPermalinkWithAPI(api, channelID, permalinkTS, cmd.OutOrStdout())
	},
}

func init() {
	permalinkCmd.Flags().StringVar(&permalinkChannel, "channel", "", "channel name (#ops), ID, or URL (required)")
	permalinkCmd.Flags().StringVar(&permalinkTS, "ts", "", "message timestamp (required)")
	_ = permalinkCmd.MarkFlagRequired("channel")
	_ = permalinkCmd.MarkFlagRequired("ts")
	rootCmd.AddCommand(permalinkCmd)
}

func runPermalinkWithAPI(api slackpkg.SlackAPI, channelID, ts string, stdout io.Writer) error {
	link, err := api.GetPermalink(&goslack.PermalinkParameters{Channel: channelID, Ts: ts})
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "permalink_failed", Detail: err.Error()}
	}
	_, _ = fmt.Fprintln(stdout, link)
	return nil
}
