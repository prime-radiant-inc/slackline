package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	want := &Config{
		Version: 1,
		Workspace: Workspace{
			Name:   "test-workspace",
			TeamID: "T123",
			URL:    "https://test.slack.com",
		},
		Bot: Bot{
			Name:     "testbot",
			AppID:    "A123",
			BotToken: "xoxb-test",
			AppToken: "xapp-test",
		},
	}

	if err := Save(want, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if got.Version != want.Version {
		t.Errorf("Version = %d, want %d", got.Version, want.Version)
	}
	if got.Workspace.Name != want.Workspace.Name {
		t.Errorf("Workspace.Name = %q, want %q", got.Workspace.Name, want.Workspace.Name)
	}
	if got.Workspace.TeamID != want.Workspace.TeamID {
		t.Errorf("Workspace.TeamID = %q, want %q", got.Workspace.TeamID, want.Workspace.TeamID)
	}
	if got.Workspace.URL != want.Workspace.URL {
		t.Errorf("Workspace.URL = %q, want %q", got.Workspace.URL, want.Workspace.URL)
	}
	if got.Bot.Name != want.Bot.Name {
		t.Errorf("Bot.Name = %q, want %q", got.Bot.Name, want.Bot.Name)
	}
	if got.Bot.AppID != want.Bot.AppID {
		t.Errorf("Bot.AppID = %q, want %q", got.Bot.AppID, want.Bot.AppID)
	}
	if got.Bot.BotToken != want.Bot.BotToken {
		t.Errorf("Bot.BotToken = %q, want %q", got.Bot.BotToken, want.Bot.BotToken)
	}
	if got.Bot.AppToken != want.Bot.AppToken {
		t.Errorf("Bot.AppToken = %q, want %q", got.Bot.AppToken, want.Bot.AppToken)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{Version: 1}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestSave_RepairsExistingFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &Config{Version: 1}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	path := filepath.Join(nested, "config.json")

	cfg := &Config{Version: 1}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(nested)
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}

	perm := info.Mode().Perm()
	if perm != 0o700 {
		t.Errorf("dir permissions = %o, want 0700", perm)
	}
}

func TestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Version: 1,
		Bot: Bot{
			BotToken: "file-bot-token",
			AppToken: "file-app-token",
		},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	t.Setenv("SLACKLINE_BOT_TOKEN", "env-bot-token")
	t.Setenv("SLACKLINE_APP_TOKEN", "env-app-token")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Bot.BotToken != "env-bot-token" {
		t.Errorf("BotToken = %q, want %q", got.Bot.BotToken, "env-bot-token")
	}
	if got.Bot.AppToken != "env-app-token" {
		t.Errorf("AppToken = %q, want %q", got.Bot.AppToken, "env-app-token")
	}
}

func TestEnvOnly(t *testing.T) {
	t.Setenv("SLACKLINE_BOT_TOKEN", "env-only-bot")
	t.Setenv("SLACKLINE_APP_TOKEN", "env-only-app")

	got, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.Bot.BotToken != "env-only-bot" {
		t.Errorf("BotToken = %q, want %q", got.Bot.BotToken, "env-only-bot")
	}
	if got.Bot.AppToken != "env-only-app" {
		t.Errorf("AppToken = %q, want %q", got.Bot.AppToken, "env-only-app")
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath()
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultPath() = %q, want absolute path", path)
	}
	suffix := filepath.Join("slackline", "config.json")
	if filepath.Base(filepath.Dir(path)) != "slackline" || filepath.Base(path) != "config.json" {
		t.Errorf("DefaultPath() = %q, want path ending with %q", path, suffix)
	}
}

func TestProvisionConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provision.json")

	want := &ProvisionConfig{
		ConfigToken:  "cfg-token-123",
		RefreshToken: "refresh-token-456",
	}

	if err := SaveProvision(want, path); err != nil {
		t.Fatalf("SaveProvision: %v", err)
	}

	got, err := LoadProvision(path)
	if err != nil {
		t.Fatalf("LoadProvision: %v", err)
	}

	if got.ConfigToken != want.ConfigToken {
		t.Errorf("ConfigToken = %q, want %q", got.ConfigToken, want.ConfigToken)
	}
	if got.RefreshToken != want.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, want.RefreshToken)
	}
}

func TestSaveProvision_RepairsExistingFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provision.json")

	if err := os.WriteFile(path, []byte(`{"config_token":"old","refresh_token":"old"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &ProvisionConfig{ConfigToken: "xoxe.cfg", RefreshToken: "xoxe-ref"}
	if err := SaveProvision(cfg, path); err != nil {
		t.Fatalf("SaveProvision: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestSLACKLINE_CONFIG_EnvVar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-config.json")

	cfg := &Config{
		Version: 1,
		Bot: Bot{
			Name:     "custom-bot",
			BotToken: "xoxb-custom",
		},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load with explicit custom path (simulating SLACKLINE_CONFIG usage)
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Bot.Name != "custom-bot" {
		t.Errorf("Bot.Name = %q, want %q", got.Bot.Name, "custom-bot")
	}
	if got.Bot.BotToken != "xoxb-custom" {
		t.Errorf("Bot.BotToken = %q, want %q", got.Bot.BotToken, "xoxb-custom")
	}
}

func TestDefaultProvisionPath(t *testing.T) {
	path := DefaultProvisionPath()
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultProvisionPath() = %q, want absolute path", path)
	}
	if filepath.Base(filepath.Dir(path)) != "slackline" || filepath.Base(path) != "provision.json" {
		t.Errorf("DefaultProvisionPath() = %q, want path ending with slackline/provision.json", path)
	}
}

func TestLoadProvision_NotFound(t *testing.T) {
	_, err := LoadProvision("/nonexistent/path/provision.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadProvision_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provision.json")

	if err := os.WriteFile(path, []byte("not json {{{"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadProvision(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestSaveProvision_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provision.json")

	cfg := &ProvisionConfig{
		ConfigToken:  "cfg-token",
		RefreshToken: "refresh-token",
	}
	if err := SaveProvision(cfg, path); err != nil {
		t.Fatalf("SaveProvision: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("not valid json!!!"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestSave_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Version: 1,
		Bot: Bot{
			Name: "jsonbot",
		},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Verify valid JSON
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Verify "version" field exists
	if _, ok := raw["version"]; !ok {
		t.Error("JSON output missing 'version' field")
	}
}
