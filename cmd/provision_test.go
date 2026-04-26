package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/config"
)

func writeProvisionFile(t *testing.T, dir string, cfg *config.ProvisionConfig) string {
	t.Helper()
	path := filepath.Join(dir, "provision.json")
	if err := config.SaveProvision(cfg, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProvision_NameSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tooling.tokens.rotate", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"token":"xoxe.new","refresh_token":"xoxe-newref"}`)
	})
	mux.HandleFunc("/api/apps.manifest.create", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"ok":true,
			"app_id":"A123",
			"team_id":"T456",
			"team_domain":"acme",
			"oauth_authorize_url":"https://slack.com/oauth/v2/authorize?client_id=x"
		}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	provPath := writeProvisionFile(t, tmp, &config.ProvisionConfig{
		ConfigToken:  "xoxe.old",
		RefreshToken: "xoxe-oldref",
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runProvisionWithDeps("my-bot", "", false, provPath, srv.URL+"/api/", stdout, stderr)
	if err != nil {
		t.Fatalf("runProvisionWithDeps returned error: %v\nstderr: %s", err, stderr.String())
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout.String())
	}

	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
	if got["app_id"] != "A123" {
		t.Errorf("app_id = %v", got["app_id"])
	}
	if got["team_id"] != "T456" {
		t.Errorf("team_id = %v", got["team_id"])
	}
	if got["team_domain"] != "acme" {
		t.Errorf("team_domain = %v", got["team_domain"])
	}
	if !strings.Contains(got["install_url"].(string), "A123/install-on-team") {
		t.Errorf("install_url = %v", got["install_url"])
	}
	if !strings.Contains(got["oauth_page_url"].(string), "A123/oauth") {
		t.Errorf("oauth_page_url = %v", got["oauth_page_url"])
	}
	if !strings.Contains(got["general_page_url"].(string), "A123/general") {
		t.Errorf("general_page_url = %v", got["general_page_url"])
	}
	if got["oauth_authorize_url"] != "https://slack.com/oauth/v2/authorize?client_id=x" {
		t.Errorf("oauth_authorize_url = %v", got["oauth_authorize_url"])
	}

	// provision.json should now contain the rotated tokens.
	rotated, err := config.LoadProvision(provPath)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ConfigToken != "xoxe.new" {
		t.Errorf("rotated ConfigToken = %q, want xoxe.new", rotated.ConfigToken)
	}
	if rotated.RefreshToken != "xoxe-newref" {
		t.Errorf("rotated RefreshToken = %q, want xoxe-newref", rotated.RefreshToken)
	}
}

func TestProvision_MissingProvisionFile(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "does_not_exist.json")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runProvisionWithDeps("my-bot", "", false, missing, "https://example.invalid/api/", stdout, stderr)
	if err == nil {
		t.Fatal("expected error when provision.json missing")
	}
	if !strings.Contains(err.Error(), "bootstrap") {
		t.Errorf("error should mention 'bootstrap', got: %v", err)
	}
}

func TestProvisionBootstrap_FromEnv(t *testing.T) {
	tmp := t.TempDir()
	provPath := filepath.Join(tmp, "provision.json")

	t.Setenv("SLACKLINE_CONFIG_TOKEN", "xoxe.cfg")
	t.Setenv("SLACKLINE_REFRESH_TOKEN", "xoxe-ref")

	stderr := &bytes.Buffer{}
	err := runProvisionBootstrapWithDeps(provPath, &bytes.Buffer{}, stderr)
	if err != nil {
		t.Fatalf("bootstrap failed: %v\nstderr: %s", err, stderr.String())
	}

	got, err := config.LoadProvision(provPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigToken != "xoxe.cfg" || got.RefreshToken != "xoxe-ref" {
		t.Errorf("provision.json = %+v", got)
	}
}

func TestProvisionBootstrap_OnlyOneEnvSet(t *testing.T) {
	tmp := t.TempDir()
	provPath := filepath.Join(tmp, "provision.json")

	t.Setenv("SLACKLINE_CONFIG_TOKEN", "xoxe.cfg")
	t.Setenv("SLACKLINE_REFRESH_TOKEN", "")

	stderr := &bytes.Buffer{}
	err := runProvisionBootstrapWithDeps(provPath, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("expected error when only one env var set")
	}
	if _, statErr := os.Stat(provPath); !os.IsNotExist(statErr) {
		t.Error("provision.json should not be written on validation failure")
	}
}

func TestProvisionBootstrap_FromStdin(t *testing.T) {
	tmp := t.TempDir()
	provPath := filepath.Join(tmp, "provision.json")

	t.Setenv("SLACKLINE_CONFIG_TOKEN", "")
	t.Setenv("SLACKLINE_REFRESH_TOKEN", "")

	stdin := bytes.NewBufferString("xoxe.cfg-stdin\nxoxe-ref-stdin\n")
	stderr := &bytes.Buffer{}
	err := runProvisionBootstrapWithDeps(provPath, stdin, stderr)
	if err != nil {
		t.Fatalf("bootstrap failed: %v\nstderr: %s", err, stderr.String())
	}

	got, err := config.LoadProvision(provPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigToken != "xoxe.cfg-stdin" || got.RefreshToken != "xoxe-ref-stdin" {
		t.Errorf("provision.json = %+v", got)
	}
}
