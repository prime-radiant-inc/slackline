package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Version   int       `json:"version"`
	Workspace Workspace `json:"workspace,omitempty"`
	Bot       Bot       `json:"bot"`
}

type Workspace struct {
	Name   string `json:"name,omitempty"`
	TeamID string `json:"team_id,omitempty"`
	URL    string `json:"url,omitempty"`
}

type Bot struct {
	Name     string `json:"name"`
	AppID    string `json:"app_id"`
	BotToken string `json:"bot_token"`
	AppToken string `json:"app_token"`
}

type ProvisionConfig struct {
	ConfigToken  string `json:"config_token"`
	RefreshToken string `json:"refresh_token"`
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "slackline", "config.json")
}

func DefaultProvisionPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "slackline", "provision.json")
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	cfg, err := LoadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if cfg == nil {
		cfg = &Config{Version: 1}
	}
	applyEnvOverrides(cfg)
	return cfg, nil
}

func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func LoadProvision(path string) (*ProvisionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read provision config: %w", err)
	}
	var cfg ProvisionConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse provision config: %w", err)
	}
	return &cfg, nil
}

func SaveProvision(cfg *ProvisionConfig, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create provision dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal provision config: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SLACKLINE_BOT_TOKEN"); v != "" {
		cfg.Bot.BotToken = v
	}
	if v := os.Getenv("SLACKLINE_APP_TOKEN"); v != "" {
		cfg.Bot.AppToken = v
	}
}
