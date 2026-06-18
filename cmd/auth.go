package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/prime-radiant-inc/slackline/config"
	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
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

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show the authenticated bot identity",
	RunE:  runAuthWhoami,
}

const unknownDisplay = "(unknown)"

func init() {
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authWhoamiCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	cfg, cfgPath, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeConfigError, Detail: err.Error()}
	}

	var botAPI slackpkg.SlackAPI
	if cfg.Bot.BotToken != "" {
		botAPI = slackpkg.NewClient(cfg.Bot.BotToken)
	}
	var appAPI slackpkg.AppTokenAPI
	if cfg.Bot.AppToken != "" && strings.HasPrefix(cfg.Bot.AppToken, "xapp-") {
		appAPI = slackpkg.NewAppClient(cfg.Bot.AppToken)
	}

	return runAuthStatusWithAPIs(cfg, cfgPath, botAPI, appAPI, cmd.OutOrStdout())
}

func runAuthStatusWithAPIs(cfg *config.Config, cfgPath string, botAPI slackpkg.SlackAPI, appAPI slackpkg.AppTokenAPI, out io.Writer) error {
	// Validate bot token
	botStatus := "(not configured)"
	botName := unknownDisplay
	workspace := unknownDisplay
	teamID := ""
	if cfg.Bot.BotToken != "" && botAPI != nil {
		resp, authErr := botAPI.AuthTest()
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

	appStatus := "(not configured)"
	if cfg.Bot.AppToken != "" {
		if strings.HasPrefix(cfg.Bot.AppToken, "xapp-") {
			if appAPI == nil {
				appStatus = "(invalid: app token validator unavailable)"
			} else if err := appAPI.OpenSocketMode(context.Background()); err != nil {
				appStatus = fmt.Sprintf("(invalid: %s)", err.Error())
			} else {
				appStatus = "(valid)"
			}
		} else {
			appStatus = "(invalid: expected xapp- prefix)"
		}
	}

	// Format workspace display
	workspaceDisplay := workspace
	if teamID != "" {
		workspaceDisplay = fmt.Sprintf("%s (%s)", workspace, teamID)
	}

	_, _ = fmt.Fprintf(out, "Bot:       %s\n", botName)
	_, _ = fmt.Fprintf(out, "Workspace: %s\n", workspaceDisplay)
	_, _ = fmt.Fprintf(out, "Bot Token: %s %s\n", maskToken(cfg.Bot.BotToken), botStatus)
	_, _ = fmt.Fprintf(out, "App Token: %s %s\n", maskToken(cfg.Bot.AppToken), appStatus)
	_, _ = fmt.Fprintf(out, "Config:    %s\n", cfgPath)

	return nil
}

func runAuthWhoami(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeConfigError, Detail: err.Error()}
	}
	if cfg.Bot.BotToken == "" {
		return &errs.SlackError{Code: errs.Config, Err: errs.CodeNoToken, Detail: "No bot token configured. Run 'slackline init' to set up."}
	}
	return runAuthWhoamiWithAPI(slackpkg.NewClient(cfg.Bot.BotToken), cmd.OutOrStdout())
}

func runAuthWhoamiWithAPI(api slackpkg.SlackAPI, out io.Writer) error {
	resp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: errs.CodeAuthTestFailed, Detail: err.Error()}
	}
	writeAuthIdentity(out, resp)
	return nil
}

func writeAuthIdentity(out io.Writer, resp *goslack.AuthTestResponse) {
	_, _ = fmt.Fprintf(out, "Bot:       %s\n", authDisplay(resp.User, resp.UserID))
	_, _ = fmt.Fprintf(out, "Bot ID:    %s\n", emptyUnknown(resp.BotID))
	_, _ = fmt.Fprintf(out, "Workspace: %s\n", authDisplay(resp.Team, resp.TeamID))
	_, _ = fmt.Fprintf(out, "URL:       %s\n", emptyUnknown(resp.URL))
}

func authDisplay(name, id string) string {
	name = emptyUnknown(name)
	if id == "" {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

func emptyUnknown(value string) string {
	if value == "" {
		return unknownDisplay
	}
	return value
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
