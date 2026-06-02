package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/config"
)

const (
	fixtureOldConfigToken  = "xoxe.old"
	fixtureOldRefreshToken = "xoxe-oldref"
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
	mux.HandleFunc("/api/apps.manifest.export", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"manifest":{"display_information":{"name":"mybot"}}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	provPath := writeProvisionFile(t, tmp, &config.ProvisionConfig{
		ConfigToken:  fixtureOldConfigToken,
		RefreshToken: fixtureOldRefreshToken,
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runProvisionWithDeps("mybot", "", false, provPath, srv.URL+"/api/", stdout, stderr)
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
	if got["effective_name"] != "mybot" {
		t.Errorf("effective_name = %v, want mybot", got["effective_name"])
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
	// oauth_authorize_url must be team-scoped — see PRI-1619.
	authorize := got["oauth_authorize_url"].(string)
	if !strings.Contains(authorize, "team=T456") {
		t.Errorf("oauth_authorize_url missing team=T456: %v", authorize)
	}
	if !strings.Contains(authorize, "install_redirect=install-on-team") {
		t.Errorf("oauth_authorize_url missing install_redirect=install-on-team: %v", authorize)
	}
	// No warning when requested name matches the registered name.
	if strings.Contains(stderr.String(), "warning") {
		t.Errorf("unexpected warning on stderr: %s", stderr.String())
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

// TestProvision_NameMutatedBySlack covers PRI-1618: Slack silently strips dashes
// from app names. The requested name "jesse-claude" becomes "jesseclaude" once
// stored by Slack; provision should detect this via apps.manifest.export and
// warn on stderr + emit `effective_name` in the JSON.
func TestProvision_NameMutatedBySlack(t *testing.T) {
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
	mux.HandleFunc("/api/apps.manifest.export", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"manifest":{"display_information":{"name":"jesseclaude"}}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	provPath := writeProvisionFile(t, tmp, &config.ProvisionConfig{
		ConfigToken:  fixtureOldConfigToken,
		RefreshToken: fixtureOldRefreshToken,
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := runProvisionWithDeps("jesse-claude", "", false, provPath, srv.URL+"/api/", stdout, stderr); err != nil {
		t.Fatalf("runProvisionWithDeps returned error: %v\nstderr: %s", err, stderr.String())
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if got["effective_name"] != "jesseclaude" {
		t.Errorf("effective_name = %v, want jesseclaude", got["effective_name"])
	}

	stderrText := stderr.String()
	if !strings.Contains(strings.ToLower(stderrText), "warning") {
		t.Errorf("expected warning on stderr, got: %s", stderrText)
	}
	if !strings.Contains(stderrText, "jesse-claude") {
		t.Errorf("warning should quote requested name; stderr: %s", stderrText)
	}
	if !strings.Contains(stderrText, "jesseclaude") {
		t.Errorf("warning should quote effective name; stderr: %s", stderrText)
	}
}

// TestProvision_ExportFailureDoesNotFail covers a partial-failure path: if
// apps.manifest.export fails after a successful create, we should still emit
// the JSON for the created app (callers can recover) but warn loudly.
func TestProvision_ExportFailureDoesNotFail(t *testing.T) {
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
	mux.HandleFunc("/api/apps.manifest.export", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":false,"error":"app_not_found"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	provPath := writeProvisionFile(t, tmp, &config.ProvisionConfig{
		ConfigToken:  fixtureOldConfigToken,
		RefreshToken: fixtureOldRefreshToken,
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := runProvisionWithDeps("my-bot", "", false, provPath, srv.URL+"/api/", stdout, stderr); err != nil {
		t.Fatalf("runProvisionWithDeps returned error: %v\nstderr: %s", err, stderr.String())
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if got["app_id"] != "A123" {
		t.Errorf("app_id = %v", got["app_id"])
	}
	// effective_name should be absent when verification failed.
	if _, ok := got["effective_name"]; ok {
		t.Errorf("effective_name should be omitted on export failure; got %v", got["effective_name"])
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "warning") {
		t.Errorf("expected export-failure warning on stderr, got: %s", stderr.String())
	}
}

// --- ensureTeamScopedAuthorizeURL tests (PRI-1619) ---

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u.Query()
}

func TestEnsureTeamScopedAuthorizeURL_AddsMissingParams(t *testing.T) {
	got := ensureTeamScopedAuthorizeURL("https://slack.com/oauth/v2/authorize?client_id=x", "T123")
	q := parseQuery(t, got)
	if q.Get("client_id") != "x" {
		t.Errorf("client_id lost: %q", got)
	}
	if q.Get("team") != "T123" {
		t.Errorf("team = %q, want T123 (full url: %q)", q.Get("team"), got)
	}
	if q.Get("install_redirect") != "install-on-team" {
		t.Errorf("install_redirect = %q, want install-on-team (full url: %q)", q.Get("install_redirect"), got)
	}
}

func TestEnsureTeamScopedAuthorizeURL_PreservesExistingTeam(t *testing.T) {
	got := ensureTeamScopedAuthorizeURL("https://slack.com/oauth/v2/authorize?client_id=x&team=Texisting", "T123")
	q := parseQuery(t, got)
	if q.Get("team") != "Texisting" {
		t.Errorf("existing team overwritten: %q", got)
	}
	if q.Get("install_redirect") != "install-on-team" {
		t.Errorf("install_redirect not added: %q", got)
	}
}

func TestEnsureTeamScopedAuthorizeURL_PreservesExistingInstallRedirect(t *testing.T) {
	got := ensureTeamScopedAuthorizeURL("https://slack.com/oauth/v2/authorize?client_id=x&install_redirect=other", "T123")
	q := parseQuery(t, got)
	if q.Get("install_redirect") != "other" {
		t.Errorf("existing install_redirect overwritten: %q", got)
	}
	if q.Get("team") != "T123" {
		t.Errorf("team not added: %q", got)
	}
}

func TestEnsureTeamScopedAuthorizeURL_EmptyTeamLeavesURLAlone(t *testing.T) {
	in := "https://slack.com/oauth/v2/authorize?client_id=x"
	got := ensureTeamScopedAuthorizeURL(in, "")
	if got != in {
		t.Errorf("empty teamID should not modify URL; got %q", got)
	}
}

func TestEnsureTeamScopedAuthorizeURL_EmptyURLReturnsEmpty(t *testing.T) {
	got := ensureTeamScopedAuthorizeURL("", "T123")
	if got != "" {
		t.Errorf("empty URL should stay empty; got %q", got)
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

	restore := stubReadSecretLine(
		t,
		"xoxe.cfg-stdin",
		"xoxe-ref-stdin",
	)
	defer restore()

	stdin := bytes.NewBufferString("")
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

func TestProvisionBootstrap_RejectsNonTTYSecretInput(t *testing.T) {
	tmp := t.TempDir()
	provPath := filepath.Join(tmp, "provision.json")

	t.Setenv("SLACKLINE_CONFIG_TOKEN", "")
	t.Setenv("SLACKLINE_REFRESH_TOKEN", "")

	stdin := bytes.NewBufferString("xoxe.cfg-stdin\nxoxe-ref-stdin\n")
	stderr := &bytes.Buffer{}
	err := runProvisionBootstrapWithDeps(provPath, stdin, stderr)
	if err == nil {
		t.Fatal("expected non-TTY secret input to fail")
	}
	if !strings.Contains(err.Error(), "non_tty_secret_input") {
		t.Fatalf("error = %v, want non_tty_secret_input", err)
	}
	if _, statErr := os.Stat(provPath); !os.IsNotExist(statErr) {
		t.Error("provision.json should not be written when secret input is not a TTY")
	}
}

func TestCreate_RemovedReturnsMigrationError(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runCreateRemoved(stdout, stderr)
	if err == nil {
		t.Fatal("expected error from removed `slackline create`")
	}
	if !strings.Contains(err.Error(), "provision") {
		t.Errorf("error should mention 'provision' to guide migration, got: %v", err)
	}
}
