package slack

import (
	"errors"

	goslack "github.com/slack-go/slack"
)

// UploadFiles is the multi-file batched upload wrapper.
// Task 3 fills in the body.
func (c *realClient) UploadFiles(channelID, threadTS, initialComment string, files []FileUpload) ([]goslack.FileSummary, error) {
	return nil, errors.New("not implemented")
}
