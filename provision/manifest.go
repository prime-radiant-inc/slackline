package provision

// Manifest represents a Slack app manifest for provisioning bot identities.
type Manifest struct {
	DisplayInfo DisplayInfo `json:"display_information"`
	Settings    Settings    `json:"settings"`
	OAuthConfig OAuthConfig `json:"oauth_config"`
}

// DisplayInfo holds the app's display name and description.
type DisplayInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Settings holds app-level configuration like socket mode and event subscriptions.
type Settings struct {
	SocketModeEnabled  bool               `json:"socket_mode_enabled"`
	EventSubscriptions EventSubscriptions `json:"event_subscriptions"`
}

// EventSubscriptions lists the bot events the app subscribes to.
type EventSubscriptions struct {
	BotEvents []string `json:"bot_events"`
}

// OAuthConfig holds the OAuth scopes for the app.
type OAuthConfig struct {
	Scopes Scopes `json:"scopes"`
}

// Scopes holds the bot token scopes.
type Scopes struct {
	Bot []string `json:"bot"`
}

// GenerateManifest creates a Slack app manifest with the required scopes and
// event subscriptions for a slackline bot identity.
func GenerateManifest(appName string) *Manifest {
	return &Manifest{
		DisplayInfo: DisplayInfo{
			Name:        appName,
			Description: "Slackline bot identity for AI agents",
		},
		Settings: Settings{
			SocketModeEnabled: true,
			EventSubscriptions: EventSubscriptions{
				BotEvents: []string{
					"app_mention",
					"message.im",
					"reaction_added",
				},
			},
		},
		OAuthConfig: OAuthConfig{
			Scopes: Scopes{
				Bot: []string{
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
				},
			},
		},
	}
}
