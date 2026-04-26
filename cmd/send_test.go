package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
)

func TestSend_TextOnlyUsesPostMessage(t *testing.T) {
	api := &fakeSlackAPI{}
	stdout := &bytes.Buffer{}
	err := runSendWithAPI(api, "C123", "hello", "", nil, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.lastUploadFilesCall != nil {
		t.Error("text-only send should NOT use UploadFiles")
	}
}

func TestSend_WithSingleAttach(t *testing.T) {
	api := &fakeSlackAPI{
		uploadFilesResp: []goslack.FileSummary{{ID: "F1"}},
	}
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.txt")
	_ = os.WriteFile(a, []byte("abc"), 0o600)

	stdout := &bytes.Buffer{}
	err := runSendWithAPI(api, "C123", "see this", "", []string{a}, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	got := api.lastUploadFilesCall
	if got.channelID != "C123" {
		t.Errorf("channel = %q", got.channelID)
	}
	if got.initialComment != "see this" {
		t.Errorf("initial_comment = %q", got.initialComment)
	}
	if len(got.files) != 1 || got.files[0].Path != a {
		t.Errorf("files = %+v", got.files)
	}
	var out map[string]interface{}
	_ = json.Unmarshal(stdout.Bytes(), &out)
	files, _ := out["files"].([]interface{})
	if len(files) != 1 {
		t.Errorf("output files = %+v", files)
	}
}

func TestSend_AttachWithoutMessage(t *testing.T) {
	api := &fakeSlackAPI{
		uploadFilesResp: []goslack.FileSummary{{ID: "F1"}},
	}
	tmp := t.TempDir()
	p := filepath.Join(tmp, "f")
	_ = os.WriteFile(p, []byte("x"), 0o600)
	if err := runSendWithAPI(api, "C123", "", "", []string{p}, &bytes.Buffer{}); err != nil {
		t.Fatalf("send without message failed: %v", err)
	}
	if api.lastUploadFilesCall.initialComment != "" {
		t.Errorf("initial_comment should be empty, got %q", api.lastUploadFilesCall.initialComment)
	}
}

func TestSend_AttachInThread(t *testing.T) {
	api := &fakeSlackAPI{
		uploadFilesResp: []goslack.FileSummary{{ID: "F1"}},
	}
	tmp := t.TempDir()
	p := filepath.Join(tmp, "f")
	_ = os.WriteFile(p, []byte("x"), 0o600)
	if err := runSendWithAPI(api, "C123", "", "1000.000", []string{p}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if api.lastUploadFilesCall.threadTS != "1000.000" {
		t.Errorf("thread_ts = %q", api.lastUploadFilesCall.threadTS)
	}
}

func TestSend_AttachMultipleFiles(t *testing.T) {
	api := &fakeSlackAPI{
		uploadFilesResp: []goslack.FileSummary{{ID: "F1"}, {ID: "F2"}},
	}
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a")
	b := filepath.Join(tmp, "b")
	_ = os.WriteFile(a, []byte("a"), 0o600)
	_ = os.WriteFile(b, []byte("b"), 0o600)
	if err := runSendWithAPI(api, "C123", "two files", "", []string{a, b}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(api.lastUploadFilesCall.files) != 2 {
		t.Errorf("files = %+v", api.lastUploadFilesCall.files)
	}
}

func TestSend_AttachMissingFile(t *testing.T) {
	api := &fakeSlackAPI{}
	err := runSendWithAPI(api, "C123", "", "", []string{"/nonexistent/file.txt"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSend_AttachExceedsSizeCap(t *testing.T) {
	api := &fakeSlackAPI{}
	tmp := t.TempDir()
	p := filepath.Join(tmp, "big")
	_ = os.WriteFile(p, bytes.Repeat([]byte("x"), 200), 0o600)
	t.Setenv("SLACKLINE_MAX_UPLOAD_BYTES", "100")
	err := runSendWithAPI(api, "C123", "", "", []string{p}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected size-cap error")
	}
}

var _ slackpkg.SlackAPI = (*fakeSlackAPI)(nil)
