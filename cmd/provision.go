package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/prime-radiant-inc/slackline/config"
	"github.com/prime-radiant-inc/slackline/errs"
	"github.com/prime-radiant-inc/slackline/provision"
	"github.com/spf13/cobra"
)

var (
	provisionDescription string
	provisionAlwaysOn    bool
)

var provisionCmd = &cobra.Command{
	Use:   "provision NAME",
	Short: "Create a Slack app via the manifest API (machine-readable JSON output)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvisionWithDeps(args[0], provisionDescription, provisionAlwaysOn,
			config.DefaultProvisionPath(), "https://slack.com/api/",
			cmd.OutOrStdout(), cmd.OutOrStderr())
	},
}

var provisionBootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Seed provision.json from env vars or stdin (one-time per machine)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvisionBootstrapWithDeps(config.DefaultProvisionPath(), os.Stdin, cmd.OutOrStderr())
	},
}

func init() {
	provisionCmd.Flags().StringVar(&provisionDescription, "description", "", "override the default app description")
	provisionCmd.Flags().BoolVar(&provisionAlwaysOn, "always-online", false, "set bot_user.always_online on the manifest")
	provisionCmd.AddCommand(provisionBootstrapCmd)
	rootCmd.AddCommand(provisionCmd)
}

// runProvisionWithDeps is the testable core of `slackline provision NAME`.
func runProvisionWithDeps(name, description string, alwaysOnline bool, provPath, apiBase string, stdout, stderr io.Writer) error {
	prov, err := config.LoadProvision(provPath)
	if err != nil {
		return &errs.SlackError{
			Code:   errs.Config,
			Err:    "no_provision_config",
			Detail: "No provision.json found. Run `slackline provision bootstrap` first to seed config and refresh tokens.",
		}
	}

	newToken, newRefresh, err := rotateProvisionConfigToken(apiBase, prov.RefreshToken)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "token_rotation_failed", Detail: err.Error()}
	}
	prov.ConfigToken = newToken
	prov.RefreshToken = newRefresh
	if err := config.SaveProvision(prov, provPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
	}

	manifest := provision.GenerateManifest(name, description, alwaysOnline)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return &errs.SlackError{Code: errs.Usage, Err: "marshal_manifest", Detail: err.Error()}
	}

	createResp, err := postProvisionManifestCreate(apiBase, prov.ConfigToken, string(manifestJSON))
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "create_app_failed", Detail: err.Error()}
	}

	// PRI-1618: Slack silently mutates app names (e.g., strips dashes). Read
	// the canonical name back from apps.manifest.export and warn if it differs
	// from what we asked for. Export failures are non-fatal — the app already
	// exists and the caller can recover.
	effectiveName, exportErr := postProvisionManifestExport(apiBase, prov.ConfigToken, createResp.AppID)
	switch {
	case exportErr != nil:
		_, _ = fmt.Fprintf(stderr, "warning: could not verify registered app name (apps.manifest.export failed: %v)\n", exportErr)
	case effectiveName != "" && effectiveName != name:
		_, _ = fmt.Fprintf(stderr, "warning: app name %q was registered by Slack as %q\n", name, effectiveName)
	}

	out := struct {
		OK                bool   `json:"ok"`
		AppID             string `json:"app_id"`
		TeamID            string `json:"team_id"`
		TeamDomain        string `json:"team_domain"`
		EffectiveName     string `json:"effective_name,omitempty"`
		InstallURL        string `json:"install_url"`
		OAuthAuthorizeURL string `json:"oauth_authorize_url"`
		OAuthPageURL      string `json:"oauth_page_url"`
		GeneralPageURL    string `json:"general_page_url"`
	}{
		OK:                true,
		AppID:             createResp.AppID,
		TeamID:            createResp.TeamID,
		TeamDomain:        createResp.TeamDomain,
		EffectiveName:     effectiveName,
		InstallURL:        fmt.Sprintf("https://api.slack.com/apps/%s/install-on-team", createResp.AppID),
		OAuthAuthorizeURL: ensureTeamScopedAuthorizeURL(createResp.OAuthAuthorizeURL, createResp.TeamID),
		OAuthPageURL:      fmt.Sprintf("https://api.slack.com/apps/%s/oauth", createResp.AppID),
		GeneralPageURL:    fmt.Sprintf("https://api.slack.com/apps/%s/general", createResp.AppID),
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// ensureTeamScopedAuthorizeURL augments an oauth_authorize_url with the query
// params Slack requires for a successful install (PRI-1619). The URL Slack
// returns from apps.manifest.create omits `team=` and `install_redirect=`,
// which makes it bounce with "Something went wrong when authorizing". Existing
// params on the URL are preserved.
func ensureTeamScopedAuthorizeURL(raw, teamID string) string {
	if raw == "" || teamID == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Get("team") == "" {
		q.Set("team", teamID)
	}
	if q.Get("install_redirect") == "" {
		q.Set("install_redirect", "install-on-team")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// runProvisionBootstrapWithDeps is the testable core of `slackline provision bootstrap`.
func runProvisionBootstrapWithDeps(provPath string, stdin io.Reader, stderr io.Writer) error {
	cfgTok := os.Getenv("SLACKLINE_CONFIG_TOKEN")
	refTok := os.Getenv("SLACKLINE_REFRESH_TOKEN")

	switch {
	case cfgTok != "" && refTok != "":
		// Both env vars set — non-interactive path.
	case cfgTok != "" || refTok != "":
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    "missing_token",
			Detail: "Set both SLACKLINE_CONFIG_TOKEN and SLACKLINE_REFRESH_TOKEN, or unset both to be prompted via stdin.",
		}
	default:
		// Interactive path — prompt via stdin.
		_, _ = fmt.Fprintln(stderr, "No config token found. Generate one at https://api.slack.com/apps")
		_, _ = fmt.Fprintln(stderr, "  → scroll to \"Your App Configuration Tokens\" → Generate Token.")
		_, _ = fmt.Fprintln(stderr, "")
		reader := bufio.NewReader(stdin)
		_, _ = fmt.Fprint(stderr, "Paste your config token: ")
		line, _ := reader.ReadString('\n')
		cfgTok = strings.TrimSpace(line)
		if cfgTok == "" {
			return &errs.SlackError{Code: errs.Usage, Err: "empty_config_token", Detail: "config token cannot be empty"}
		}
		_, _ = fmt.Fprint(stderr, "Paste your refresh token: ")
		line, _ = reader.ReadString('\n')
		refTok = strings.TrimSpace(line)
		if refTok == "" {
			return &errs.SlackError{Code: errs.Usage, Err: "empty_refresh_token", Detail: "refresh token cannot be empty"}
		}
	}

	prov := &config.ProvisionConfig{ConfigToken: cfgTok, RefreshToken: refTok}
	if err := config.SaveProvision(prov, provPath); err != nil {
		return &errs.SlackError{Code: errs.Config, Err: "save_failed", Detail: err.Error()}
	}
	_, _ = fmt.Fprintln(stderr, "✓ provision.json written.")
	return nil
}

// rotateProvisionConfigToken calls tooling.tokens.rotate with apiBase as a prefix.
// apiBase must end with "/" (e.g., "https://slack.com/api/").
func rotateProvisionConfigToken(apiBase, refreshToken string) (string, string, error) {
	resp, err := http.PostForm(apiBase+"tooling.tokens.rotate", url.Values{
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return "", "", fmt.Errorf("rotate request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		OK           bool   `json:"ok"`
		Error        string `json:"error,omitempty"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode rotate: %w", err)
	}
	if !out.OK {
		return "", "", fmt.Errorf("rotate: %s", out.Error)
	}
	return out.Token, out.RefreshToken, nil
}

type provisionManifestCreateResponse struct {
	OK                bool   `json:"ok"`
	Error             string `json:"error,omitempty"`
	AppID             string `json:"app_id"`
	TeamID            string `json:"team_id"`
	TeamDomain        string `json:"team_domain"`
	OAuthAuthorizeURL string `json:"oauth_authorize_url"`
}

// postProvisionManifestCreate calls apps.manifest.create with apiBase as a prefix.
// apiBase must end with "/" (e.g., "https://slack.com/api/").
func postProvisionManifestCreate(apiBase, configToken, manifestJSON string) (*provisionManifestCreateResponse, error) {
	resp, err := http.PostForm(apiBase+"apps.manifest.create", url.Values{
		"token":    {configToken},
		"manifest": {manifestJSON},
	})
	if err != nil {
		return nil, fmt.Errorf("manifest create POST failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out provisionManifestCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode manifest.create: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("apps.manifest.create: %s", out.Error)
	}
	return &out, nil
}

// postProvisionManifestExport calls apps.manifest.export and returns the
// canonical app name (display_information.name) for the given app_id. Used to
// detect when Slack has silently mutated the requested app name (PRI-1618).
func postProvisionManifestExport(apiBase, configToken, appID string) (string, error) {
	resp, err := http.PostForm(apiBase+"apps.manifest.export", url.Values{
		"token":  {configToken},
		"app_id": {appID},
	})
	if err != nil {
		return "", fmt.Errorf("manifest export POST failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error,omitempty"`
		Manifest struct {
			DisplayInformation struct {
				Name string `json:"name"`
			} `json:"display_information"`
		} `json:"manifest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode manifest.export: %w", err)
	}
	if !out.OK {
		return "", fmt.Errorf("apps.manifest.export: %s", out.Error)
	}
	return out.Manifest.DisplayInformation.Name, nil
}
