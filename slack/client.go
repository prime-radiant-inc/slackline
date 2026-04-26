package slack

import (
	goslack "github.com/slack-go/slack"
)

// realClient wraps goslack.Client and satisfies SlackAPI.
// Methods defined on goslack.Client that already match the interface are
// promoted via embedding; methods that need higher-level orchestration
// (e.g. UploadFiles) are implemented in separate files.
type realClient struct {
	*goslack.Client
	apiBase string
	token   string
}

// NewClient returns a SlackAPI backed by a real slack-go client.
func NewClient(botToken string) SlackAPI {
	return &realClient{
		Client:  goslack.New(botToken),
		apiBase: "https://slack.com/api/",
		token:   botToken,
	}
}

// Compile-time check that realClient satisfies SlackAPI.
var _ SlackAPI = (*realClient)(nil)
