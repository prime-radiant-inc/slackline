package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/prime-radiant-inc/slackline/config"
	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Configure slackline with existing tokens",
	Long:  "Set up slackline on a new machine using tokens provisioned by an admin.",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	// Prompt for workspace URL (optional, for display)
	fmt.Print("Workspace URL (e.g. https://myteam.slack.com): ")
	workspaceURL, _ := reader.ReadString('\n')
	workspaceURL = strings.TrimSpace(workspaceURL)

	// Prompt for bot token
	fmt.Print("Bot Token (xoxb-): ")
	botToken, _ := reader.ReadString('\n')
	botToken = strings.TrimSpace(botToken)
	if !strings.HasPrefix(botToken, "xoxb-") {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "invalid_token",
			Detail: "Bot token must start with 'xoxb-'. You may have pasted the wrong token type.",
		}
	}

	// Prompt for app token
	fmt.Print("App Token (xapp-): ")
	appToken, _ := reader.ReadString('\n')
	appToken = strings.TrimSpace(appToken)
	if !strings.HasPrefix(appToken, "xapp-") {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "invalid_token",
			Detail: "App token must start with 'xapp-'. You may have pasted the wrong token type.",
		}
	}

	// Validate bot token via auth.test
	api := slackpkg.NewClient(botToken)
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: fmt.Sprintf("Bot token validation failed: %v", err)}
	}

	// Resolve config path: --config flag → SLACKLINE_CONFIG env → DefaultPath()
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = os.Getenv("SLACKLINE_CONFIG")
	}
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}

	cfg := &config.Config{
		Version: 1,
		Workspace: config.Workspace{
			Name:   authResp.Team,
			TeamID: authResp.TeamID,
			URL:    workspaceURL,
		},
		Bot: config.Bot{
			Name:     authResp.User,
			BotToken: botToken,
			AppToken: appToken,
		},
	}

	if err := config.Save(cfg, cfgPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
	}

	fmt.Fprintf(os.Stderr, "\n✓ Config written to %s\n", cfgPath)
	fmt.Fprintf(os.Stderr, "  Bot: %s (via auth.test)\n", authResp.User)
	fmt.Fprintf(os.Stderr, "  Workspace: %s\n", authResp.Team)

	return nil
}
