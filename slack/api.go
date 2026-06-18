package slack

import (
	"context"
	"io"

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

	// Reactions (Task 6).
	AddReaction(name string, item goslack.ItemRef) error
	RemoveReaction(name string, item goslack.ItemRef) error

	// Files (Task 8 download).
	GetFileInfo(fileID string, count, page int) (*goslack.File, []goslack.Comment, *goslack.Paging, error)
	GetFile(downloadURL string, writer io.Writer) error

	// UploadFiles batches N files into a single Slack message via the
	// files.getUploadURLExternal + files.completeUploadExternal flow.
	// Implementation lives in slack/files.go (Task 3 fills it in).
	UploadFiles(channelID, threadTS, initialComment string, files []FileUpload) ([]goslack.FileSummary, error)
}

// AppTokenAPI is the Socket Mode preflight surface used by auth status.
type AppTokenAPI interface {
	OpenSocketMode(ctx context.Context) error
}

// FileUpload describes a single local file destined for a batched multi-file upload.
type FileUpload struct {
	Path  string
	Title string // optional; defaults to filename
}
