package cmd

import (
	"fmt"
	"strings"

	"github.com/prime-radiant/slackline/errs"
	"github.com/prime-radiant/slackline/slack"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication commands",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE:  runAuthStatus,
}

func init() {
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	cfg, cfgPath, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "config_error", Detail: err.Error()}
	}

	// Validate bot token
	botStatus := "(not configured)"
	botName := "(unknown)"
	workspace := "(unknown)"
	teamID := ""
	if cfg.Bot.BotToken != "" {
		client := slack.NewClient(cfg.Bot.BotToken)
		resp, authErr := client.AuthTest()
		if authErr != nil {
			botStatus = fmt.Sprintf("(invalid: %s)", authErr.Error())
		} else {
			botStatus = "(valid)"
			if resp.User != "" {
				botName = resp.User
			}
			if resp.Team != "" {
				workspace = resp.Team
			}
			if resp.TeamID != "" {
				teamID = resp.TeamID
			}
		}
	}

	// App tokens (xapp-) are only valid for Socket Mode — auth.test doesn't accept them.
	// Just check the prefix to confirm the right token type was configured.
	appStatus := "(not configured)"
	if cfg.Bot.AppToken != "" {
		if strings.HasPrefix(cfg.Bot.AppToken, "xapp-") {
			appStatus = "(configured)"
		} else {
			appStatus = "(invalid: expected xapp- prefix)"
		}
	}

	// Format workspace display
	workspaceDisplay := workspace
	if teamID != "" {
		workspaceDisplay = fmt.Sprintf("%s (%s)", workspace, teamID)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Bot:       %s\n", botName)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Workspace: %s\n", workspaceDisplay)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Bot Token: %s %s\n", maskToken(cfg.Bot.BotToken), botStatus)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "App Token: %s %s\n", maskToken(cfg.Bot.AppToken), appStatus)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Config:    %s\n", cfgPath)

	return nil
}

// maskToken redacts a token for display, preserving enough to identify it.
func maskToken(token string) string {
	if token == "" {
		return "(none)"
	}
	// Need at least 10 chars so first-5 and last-4 don't overlap.
	if len(token) < 10 {
		end := 4
		if len(token) < end {
			end = len(token)
		}
		return token[:end] + "-..."
	}
	return token[:5] + "..." + token[len(token)-4:]
}
