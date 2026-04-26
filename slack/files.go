package slack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	goslack "github.com/slack-go/slack"
)

// UploadFiles uploads N local files via Slack's external-upload flow and
// shares them as a single message in channelID. If threadTS is non-empty,
// the message is posted as a thread reply. initialComment, when non-empty,
// becomes the message body.
func (c *realClient) UploadFiles(channelID, threadTS, initialComment string, files []FileUpload) ([]goslack.FileSummary, error) {
	type uploadResult struct {
		fileID string
		title  string
	}
	results := make([]uploadResult, 0, len(files))

	for _, f := range files {
		size, err := fileSize(f.Path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", f.Path, err)
		}
		urlInfo, err := getUploadURLExternal(c.apiBase, c.token, filepath.Base(f.Path), size)
		if err != nil {
			return nil, err
		}
		if err := postFileBytes(urlInfo.UploadURL, f.Path); err != nil {
			return nil, err
		}
		title := f.Title
		if title == "" {
			title = filepath.Base(f.Path)
		}
		results = append(results, uploadResult{fileID: urlInfo.FileID, title: title})
	}

	type fileItem struct {
		ID    string `json:"id"`
		Title string `json:"title,omitempty"`
	}
	items := make([]fileItem, len(results))
	for i, r := range results {
		items[i] = fileItem{ID: r.fileID, Title: r.title}
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal files list: %w", err)
	}

	form := url.Values{
		"token":      {c.token},
		"files":      {string(itemsJSON)},
		"channel_id": {channelID},
	}
	if threadTS != "" {
		form.Set("thread_ts", threadTS)
	}
	if initialComment != "" {
		form.Set("initial_comment", initialComment)
	}

	resp, err := http.PostForm(c.apiBase+"files.completeUploadExternal", form)
	if err != nil {
		return nil, fmt.Errorf("completeUploadExternal POST failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var completeResp struct {
		OK    bool                  `json:"ok"`
		Error string                `json:"error,omitempty"`
		Files []goslack.FileSummary `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completeResp); err != nil {
		return nil, fmt.Errorf("decode completeUploadExternal: %w", err)
	}
	if !completeResp.OK {
		return nil, fmt.Errorf("completeUploadExternal: %s", completeResp.Error)
	}
	return completeResp.Files, nil
}

type uploadURLInfo struct {
	UploadURL string `json:"upload_url"`
	FileID    string `json:"file_id"`
}

func getUploadURLExternal(apiBase, token, filename string, size int64) (*uploadURLInfo, error) {
	resp, err := http.PostForm(apiBase+"files.getUploadURLExternal", url.Values{
		"token":    {token},
		"filename": {filename},
		"length":   {strconv.FormatInt(size, 10)},
	})
	if err != nil {
		return nil, fmt.Errorf("getUploadURLExternal POST failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
		uploadURLInfo
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode getUploadURLExternal: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("getUploadURLExternal: %s", out.Error)
	}
	return &out.uploadURLInfo, nil
}

func postFileBytes(uploadURL, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	// Slack's external-upload signed URL accepts POST (not PUT, despite earlier
	// internal docs); using PUT returns SignatureDoesNotMatch from S3.
	req, err := http.NewRequest(http.MethodPost, uploadURL, f)
	if err != nil {
		return fmt.Errorf("build upload request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload POST failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("upload returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func fileSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}
