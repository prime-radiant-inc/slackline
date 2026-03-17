package provision

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const defaultSlackAPIBase = "https://slack.com"

// RotateConfigToken exchanges a refresh token for a new configuration token
// and refresh token via the Slack tooling.tokens.rotate API.
func RotateConfigToken(apiBase, refreshToken string) (string, string, error) {
	if apiBase == "" {
		apiBase = defaultSlackAPIBase
	}

	resp, err := http.PostForm(apiBase+"/api/tooling.tokens.rotate", url.Values{
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return "", "", fmt.Errorf("token rotation request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK           bool   `json:"ok"`
		Error        string `json:"error,omitempty"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("parse rotation response: %w", err)
	}
	if !result.OK {
		return "", "", fmt.Errorf("token rotation failed: %s", result.Error)
	}
	return result.Token, result.RefreshToken, nil
}
