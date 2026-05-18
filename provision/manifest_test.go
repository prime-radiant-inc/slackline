package provision

import (
	"encoding/json"
	"testing"
)

func TestGenerateManifest_ContainsRequiredScopes(t *testing.T) {
	m := GenerateManifest("test-bot", "", false)
	requiredScopes := []string{
		ScopeChatWrite,
		ScopeChannelsRead,
		ScopeGroupsRead,
		ScopeChannelsHistory,
		ScopeGroupsHistory,
		ScopeAppMentionsRead,
		ScopeIMHistory,
		ScopeIMRead,
		ScopeReactionsRead,
		ScopeUsersRead,
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
	m := GenerateManifest("test-bot", "", false)
	if !m.Settings.SocketModeEnabled {
		t.Error("socket_mode_enabled should be true")
	}
}

func TestGenerateManifest_EventSubscriptions(t *testing.T) {
	m := GenerateManifest("test-bot", "", false)
	requiredEvents := []string{EventAppMention, EventMessageIM, EventReactionAdded}
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
	m := GenerateManifest("my-bot", "", false)
	if m.DisplayInfo.Name != "my-bot" {
		t.Errorf("app name = %q, want my-bot", m.DisplayInfo.Name)
	}
}

func TestGenerateManifest_ValidJSON(t *testing.T) {
	m := GenerateManifest("test", "", false)
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("marshaled manifest is empty")
	}
}

func TestGenerateManifest_Golden(t *testing.T) {
	m := GenerateManifest("my-bot", "", false)

	wantScopes := map[string]bool{
		ScopeChatWrite:       true,
		ScopeChannelsRead:    true,
		ScopeGroupsRead:      true,
		ScopeChannelsHistory: true,
		ScopeGroupsHistory:   true,
		ScopeAppMentionsRead: true,
		ScopeIMHistory:       true,
		ScopeIMRead:          true,
		ScopeReactionsRead:   true,
		ScopeReactionsWrite:  true,
		ScopeUsersRead:       true,
		ScopeFilesRead:       true,
		ScopeFilesWrite:      true,
	}
	gotScopes := map[string]bool{}
	for _, s := range m.OAuthConfig.Scopes.Bot {
		gotScopes[s] = true
	}
	if len(gotScopes) != len(wantScopes) {
		t.Errorf("scope count = %d, want %d (got=%v)", len(gotScopes), len(wantScopes), m.OAuthConfig.Scopes.Bot)
	}
	for s := range wantScopes {
		if !gotScopes[s] {
			t.Errorf("missing scope: %s", s)
		}
	}
	for s := range gotScopes {
		if !wantScopes[s] {
			t.Errorf("unexpected scope: %s", s)
		}
	}

	wantEvents := map[string]bool{
		EventAppMention:      true,
		EventMessageIM:       true,
		EventReactionAdded:   true,
		EventReactionRemoved: true,
		EventMessageChannels: true,
		EventMessageGroups:   true,
	}
	gotEvents := map[string]bool{}
	for _, e := range m.Settings.EventSubscriptions.BotEvents {
		gotEvents[e] = true
	}
	if len(gotEvents) != len(wantEvents) {
		t.Errorf("event count = %d, want %d (got=%v)", len(gotEvents), len(wantEvents), m.Settings.EventSubscriptions.BotEvents)
	}
	for e := range wantEvents {
		if !gotEvents[e] {
			t.Errorf("missing event subscription: %s", e)
		}
	}
}

func TestGenerateManifest_DescriptionDefault(t *testing.T) {
	m := GenerateManifest("my-bot", "", false)
	if m.DisplayInfo.Description != "Slackline bot identity for AI agents" {
		t.Errorf("default description = %q", m.DisplayInfo.Description)
	}
}

func TestGenerateManifest_DescriptionOverride(t *testing.T) {
	m := GenerateManifest("my-bot", "Custom desc", false)
	if m.DisplayInfo.Description != "Custom desc" {
		t.Errorf("description = %q, want 'Custom desc'", m.DisplayInfo.Description)
	}
}

func TestGenerateManifest_AlwaysOnlineDefault(t *testing.T) {
	m := GenerateManifest("my-bot", "", false)
	if m.Features.BotUser.AlwaysOnline {
		t.Error("default always_online should be false")
	}
}

func TestGenerateManifest_AlwaysOnlineOverride(t *testing.T) {
	m := GenerateManifest("my-bot", "", true)
	if !m.Features.BotUser.AlwaysOnline {
		t.Error("always_online should be true when override is true")
	}
}
