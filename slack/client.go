package slack

import (
	goslack "github.com/slack-go/slack"
)

// NewClient returns a SlackAPI backed by a real slack-go client.
func NewClient(botToken string) SlackAPI {
	return goslack.New(botToken)
}
