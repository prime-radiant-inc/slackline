package slack

import (
	goslack "github.com/slack-go/slack"
)

// SlackAPI is the subset of slack-go methods used by slackline.
// All command code depends on this interface, never on *slack.Client directly.
type SlackAPI interface {
	AuthTest() (response *goslack.AuthTestResponse, err error)
	PostMessage(channelID string, options ...goslack.MsgOption) (string, string, error)
	GetConversationHistory(params *goslack.GetConversationHistoryParameters) (*goslack.GetConversationHistoryResponse, error)
	GetConversationReplies(params *goslack.GetConversationRepliesParameters) ([]goslack.Message, bool, string, error)
	GetConversations(params *goslack.GetConversationsParameters) ([]goslack.Channel, string, error)
}
