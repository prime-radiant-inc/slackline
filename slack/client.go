package slack

import (
	"context"

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

// NewAppClient returns the app-token API surface used for Socket Mode checks.
func NewAppClient(appToken string) AppTokenAPI {
	return &realClient{
		Client: goslack.New("", goslack.OptionAppLevelToken(appToken)),
	}
}

func (c *realClient) OpenSocketMode(ctx context.Context) error {
	_, _, err := c.StartSocketModeContext(ctx)
	return err
}

// Compile-time check that realClient satisfies SlackAPI.
var (
	_ SlackAPI    = (*realClient)(nil)
	_ AppTokenAPI = (*realClient)(nil)
)
