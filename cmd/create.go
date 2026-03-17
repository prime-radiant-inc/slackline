package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/prime-radiant-inc/slackline/config"
	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/prime-radiant-inc/slackline/provision"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	"github.com/spf13/cobra"
)

var (
	createName string
	createInit bool
)

func init() {
	createCmd.Flags().StringVar(&createName, "name", "", "bot name (required for app creation, not needed with --init alone)")
	createCmd.Flags().BoolVar(&createInit, "init", false, "first-time bootstrap — prompt for config token")
	// NOTE: --name is NOT marked required because --init can be used standalone
	// to bootstrap config tokens without creating an app. Validation is in runCreate.
	rootCmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Slack bot app (admin)",
	Long:  "Create a Slack app via the manifest API. Requires an App Configuration Token.",
	RunE:  runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)
	provPath := config.DefaultProvisionPath()

	// Load or bootstrap provision config
	provCfg, err := config.LoadProvision(provPath)
	if err != nil || createInit {
		// First-time bootstrap
		fmt.Fprintln(os.Stderr, "No config token found. You need to generate one:")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  1. Go to https://api.slack.com/apps")
		fmt.Fprintln(os.Stderr, "  2. Scroll to \"Your App Configuration Tokens\"")
		fmt.Fprintln(os.Stderr, "  3. Click \"Generate Token\" for your workspace")
		fmt.Fprintln(os.Stderr, "")

		fmt.Print("Paste your config token: ")
		configToken, _ := reader.ReadString('\n')
		configToken = strings.TrimSpace(configToken)
		if configToken == "" {
			return &errs.SlackError{Code: errs.Usage, Err: "invalid_token", Detail: "Config token cannot be empty"}
		}

		fmt.Print("Paste your refresh token: ")
		refreshToken, _ := reader.ReadString('\n')
		refreshToken = strings.TrimSpace(refreshToken)
		if refreshToken == "" {
			return &errs.SlackError{Code: errs.Usage, Err: "invalid_token", Detail: "Refresh token cannot be empty"}
		}

		provCfg = &config.ProvisionConfig{
			ConfigToken:  configToken,
			RefreshToken: refreshToken,
		}
		if err := config.SaveProvision(provCfg, provPath); err != nil {
			return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
		}
		fmt.Fprintln(os.Stderr, "\n✓ Config token stored.")
	}

	// If --init was the only goal (no --name), we're done after storing tokens
	if createName == "" {
		return nil
	}

	// Rotate config token (they expire after 12 hours)
	fmt.Fprint(os.Stderr, "Refreshing config token... ")
	newConfig, newRefresh, err := provision.RotateConfigToken("", provCfg.RefreshToken)
	if err != nil {
		if strings.Contains(err.Error(), "token_expired") {
			fmt.Fprintln(os.Stderr, "✗")
			return &errs.SlackError{
				Code:   errs.Auth,
				Err:    "config_token_expired",
				Detail: "Refresh token expired. Re-generate at https://api.slack.com/apps and run 'slackline create --init'.",
			}
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "token_rotation_failed", Detail: err.Error()}
	}
	provCfg.ConfigToken = newConfig
	provCfg.RefreshToken = newRefresh
	if err := config.SaveProvision(provCfg, provPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: fmt.Sprintf("Failed to save rotated tokens: %v", err)}
	}
	fmt.Fprintln(os.Stderr, "✓")

	// Create app via manifest API
	manifest := provision.GenerateManifest(createName)
	manifestJSON, _ := json.Marshal(manifest)

	fmt.Fprintf(os.Stderr, "Creating Slack app %q... ", createName)
	appID, err := createAppViaManifest("", provCfg.ConfigToken, string(manifestJSON))
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗")
		return &errs.SlackError{Code: errs.SlackAPI, Err: "create_app_failed", Detail: err.Error()}
	}
	fmt.Fprintf(os.Stderr, "✓ (app_id: %s)\n", appID)

	// Guide the admin through installation
	fmt.Fprintf(os.Stderr, "\nStep 1: Install the app\n")
	fmt.Fprintf(os.Stderr, "  → https://api.slack.com/apps/%s/install-on-team\n", appID)
	fmt.Fprintf(os.Stderr, "  Click \"Allow\", then press Enter.\n")
	_, _ = reader.ReadString('\n')

	fmt.Fprintf(os.Stderr, "\nStep 2: Paste Bot Token (xoxb-)\n")
	fmt.Fprintf(os.Stderr, "  → https://api.slack.com/apps/%s/oauth\n", appID)
	fmt.Print("  Token: ")
	botToken, _ := reader.ReadString('\n')
	botToken = strings.TrimSpace(botToken)
	if !strings.HasPrefix(botToken, "xoxb-") {
		return &errs.SlackError{Code: errs.Usage, Err: "invalid_token", Detail: "Bot token must start with 'xoxb-'"}
	}

	fmt.Fprintf(os.Stderr, "\nStep 3: Paste App Token (xapp-)\n")
	fmt.Fprintf(os.Stderr, "  → https://api.slack.com/apps/%s/general\n", appID)
	fmt.Fprintln(os.Stderr, "  Click \"Generate Token\", add connections:write scope.")
	fmt.Print("  Token: ")
	appToken, _ := reader.ReadString('\n')
	appToken = strings.TrimSpace(appToken)
	if !strings.HasPrefix(appToken, "xapp-") {
		return &errs.SlackError{Code: errs.Usage, Err: "invalid_token", Detail: "App token must start with 'xapp-'"}
	}

	// Validate bot token
	api := slackpkg.NewClient(botToken)
	authResp, err := api.AuthTest()
	if err != nil {
		if isAuthError(err) {
			return errs.AuthError(err.Error())
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "auth_test_failed", Detail: fmt.Sprintf("Bot token validation failed: %v", err)}
	}

	// Write config
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
		},
		Bot: config.Bot{
			Name:     createName,
			AppID:    appID,
			BotToken: botToken,
			AppToken: appToken,
		},
	}

	if err := config.Save(cfg, cfgPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
	}

	fmt.Fprintf(os.Stderr, "\n✓ %s ready. Config written to %s\n", createName, cfgPath)
	return nil
}

// createAppViaManifest calls the Slack apps.manifest.create API.
// apiBase can be overridden for testing (e.g., httptest server URL); use "" for default.
func createAppViaManifest(apiBase, configToken, manifestJSON string) (string, error) {
	if apiBase == "" {
		apiBase = "https://slack.com"
	}
	resp, err := http.PostForm(apiBase+"/api/apps.manifest.create", url.Values{
		"token":    {configToken},
		"manifest": {manifestJSON},
	})
	if err != nil {
		return "", fmt.Errorf("manifest create request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
		AppID string `json:"app_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse manifest response: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.AppID, nil
}
