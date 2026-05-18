package provision

// Slack manifest bot-event names and OAuth bot scopes. These are wire format
// for Slack's apps.manifest.create endpoint; treat them as constants in one
// place so the tests can reuse the exact same strings.
const (
	EventAppMention      = "app_mention"
	EventMessageIM       = "message.im"
	EventMessageChannels = "message.channels"
	EventMessageGroups   = "message.groups"
	EventReactionAdded   = "reaction_added"
	EventReactionRemoved = "reaction_removed"

	ScopeChatWrite       = "chat:write"
	ScopeChannelsRead    = "channels:read"
	ScopeGroupsRead      = "groups:read"
	ScopeChannelsHistory = "channels:history"
	ScopeGroupsHistory   = "groups:history"
	ScopeAppMentionsRead = "app_mentions:read"
	ScopeIMHistory       = "im:history"
	ScopeIMRead          = "im:read"
	ScopeReactionsRead   = "reactions:read"
	ScopeReactionsWrite  = "reactions:write"
	ScopeUsersRead       = "users:read"
	ScopeFilesRead       = "files:read"
	ScopeFilesWrite      = "files:write"
)

// Manifest represents a Slack app manifest for provisioning bot identities.
type Manifest struct {
	DisplayInfo DisplayInfo `json:"display_information"`
	Features    Features    `json:"features"`
	Settings    Settings    `json:"settings"`
	OAuthConfig OAuthConfig `json:"oauth_config"`
}

// Features holds app feature configuration including the bot user identity.
type Features struct {
	BotUser BotUser `json:"bot_user"`
}

// BotUser defines the bot's display name in Slack.
type BotUser struct {
	DisplayName  string `json:"display_name"`
	AlwaysOnline bool   `json:"always_online"`
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
//
// description is optional — when empty, a default is applied.
// alwaysOnline controls Bot User's always_online setting.
func GenerateManifest(appName, description string, alwaysOnline bool) *Manifest {
	if description == "" {
		description = "Slackline bot identity for AI agents"
	}
	return &Manifest{
		DisplayInfo: DisplayInfo{
			Name:        appName,
			Description: description,
		},
		Features: Features{
			BotUser: BotUser{
				DisplayName:  appName,
				AlwaysOnline: alwaysOnline,
			},
		},
		Settings: Settings{
			SocketModeEnabled: true,
			EventSubscriptions: EventSubscriptions{
				BotEvents: []string{
					EventAppMention,
					EventMessageIM,
					EventMessageChannels,
					EventMessageGroups,
					EventReactionAdded,
					EventReactionRemoved,
				},
			},
		},
		OAuthConfig: OAuthConfig{
			Scopes: Scopes{
				Bot: []string{
					ScopeChatWrite,
					ScopeChannelsRead,
					ScopeGroupsRead,
					ScopeChannelsHistory,
					ScopeGroupsHistory,
					ScopeAppMentionsRead,
					ScopeIMHistory,
					ScopeIMRead,
					ScopeReactionsRead,
					ScopeReactionsWrite,
					ScopeUsersRead,
					ScopeFilesRead,
					ScopeFilesWrite,
				},
			},
		},
	}
}
