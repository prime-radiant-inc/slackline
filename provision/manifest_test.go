package provision

import (
	"encoding/json"
	"testing"
)

func TestGenerateManifest_ContainsRequiredScopes(t *testing.T) {
	m := GenerateManifest("test-bot")
	requiredScopes := []string{
		"chat:write",
		"channels:read",
		"groups:read",
		"channels:history",
		"groups:history",
		"app_mentions:read",
		"im:history",
		"im:read",
		"reactions:read",
		"users:read",
	}
	for _, scope := range requiredScopes {
		found := false
		for _, s := range m.OAuthConfig.Scopes.Bot {
			if s == scope {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("manifest missing required bot scope: %s", scope)
		}
	}
}

func TestGenerateManifest_SocketModeEnabled(t *testing.T) {
	m := GenerateManifest("test-bot")
	if !m.Settings.SocketModeEnabled {
		t.Error("socket_mode_enabled should be true")
	}
}

func TestGenerateManifest_EventSubscriptions(t *testing.T) {
	m := GenerateManifest("test-bot")
	requiredEvents := []string{"app_mention", "message.im", "reaction_added"}
	for _, event := range requiredEvents {
		found := false
		for _, e := range m.Settings.EventSubscriptions.BotEvents {
			if e == event {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("manifest missing event subscription: %s", event)
		}
	}
}

func TestGenerateManifest_AppName(t *testing.T) {
	m := GenerateManifest("my-bot")
	if m.DisplayInfo.Name != "my-bot" {
		t.Errorf("app name = %q, want my-bot", m.DisplayInfo.Name)
	}
}

func TestGenerateManifest_ValidJSON(t *testing.T) {
	m := GenerateManifest("test")
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("marshaled manifest is empty")
	}
}
