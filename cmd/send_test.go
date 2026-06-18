package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
)

func TestSend_TextOnlyUsesPostMessage(t *testing.T) {
	api := &fakeSlackAPI{postMessageChannel: fixtureChannelID, postMessageTS: fixtureMessageTS}
	stdout := &bytes.Buffer{}
	err := runSendWithAPI(api, fixtureChannelID, "hello", "", nil, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.lastUploadFilesCall != nil {
		t.Error("text-only send should NOT use UploadFiles")
	}
	if stdout.String() != fixtureChannelID+" "+fixtureMessageTS+"\n" {
		t.Fatalf("text output = %q", stdout.String())
	}
}

func TestSend_JSONFormat(t *testing.T) {
	api := &fakeSlackAPI{postMessageChannel: fixtureChannelID, postMessageTS: fixtureMessageTS}
	stdout := &bytes.Buffer{}
	err := runSendWithAPIFormat(api, fixtureChannelID, "hello", "", nil, outputFormatJSON, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out["channel"] != fixtureChannelID {
		t.Errorf("channel = %v", out["channel"])
	}
	if out["ts"] != fixtureMessageTS {
		t.Errorf("ts = %v", out["ts"])
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
	err := runSendWithAPIFormat(api, fixtureChannelID, "see this", "", []string{a}, outputFormatJSON, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	got := api.lastUploadFilesCall
	if got.channelID != fixtureChannelID {
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

func TestSend_WithAttachTextOutputIncludesShareTimestamp(t *testing.T) {
	api := &fakeSlackAPI{
		uploadFilesResp: []goslack.FileSummary{{ID: "F1"}},
		getFileInfoFile: &goslack.File{
			ID: "F1",
			Shares: goslack.Share{
				Public: map[string][]goslack.ShareFileInfo{
					fixtureChannelID: {{Ts: fixtureMessageTS}},
				},
			},
		},
	}
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.txt")
	_ = os.WriteFile(a, []byte("abc"), 0o600)
	stdout := &bytes.Buffer{}

	err := runSendWithAPI(api, fixtureChannelID, "see this", "", []string{a}, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	want := fixtureChannelID + " " + fixtureMessageTS + "\n"
	if stdout.String() != want {
		t.Fatalf("text output = %q, want %q", stdout.String(), want)
	}
}

func TestSend_AttachWithoutMessage(t *testing.T) {
	api := &fakeSlackAPI{
		uploadFilesResp: []goslack.FileSummary{{ID: "F1"}},
	}
	tmp := t.TempDir()
	p := filepath.Join(tmp, "f")
	_ = os.WriteFile(p, []byte("x"), 0o600)
	if err := runSendWithAPI(api, fixtureChannelID, "", "", []string{p}, &bytes.Buffer{}); err != nil {
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
	if err := runSendWithAPI(api, fixtureChannelID, "", "1000.000", []string{p}, &bytes.Buffer{}); err != nil {
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
	if err := runSendWithAPI(api, fixtureChannelID, "two files", "", []string{a, b}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(api.lastUploadFilesCall.files) != 2 {
		t.Errorf("files = %+v", api.lastUploadFilesCall.files)
	}
}

func TestSend_AttachMissingFile(t *testing.T) {
	api := &fakeSlackAPI{}
	err := runSendWithAPI(api, fixtureChannelID, "", "", []string{"/nonexistent/file.txt"}, &bytes.Buffer{})
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
	err := runSendWithAPI(api, fixtureChannelID, "", "", []string{p}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected size-cap error")
	}
}

// --- linkifyMessage (Issue #1) ---

func TestLinkifyMessage_ResolvesHandle(t *testing.T) {
	api := &fakeSlackAPI{users: drewUser()}
	var warn bytes.Buffer
	got, err := linkifyMessage(api, "heads up @drew", false, &warn)
	if err != nil {
		t.Fatalf("linkifyMessage error: %v", err)
	}
	if got != "heads up <@U1>" {
		t.Fatalf("linkifyMessage = %q, want %q", got, "heads up <@U1>")
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestLinkifyMessage_DisabledLeavesLiteral(t *testing.T) {
	api := &fakeSlackAPI{users: drewUser()}
	var warn bytes.Buffer
	got, err := linkifyMessage(api, "heads up @drew", true, &warn)
	if err != nil {
		t.Fatalf("linkifyMessage error: %v", err)
	}
	if got != "heads up @drew" {
		t.Fatalf("disabled linkify changed text: %q", got)
	}
}

func TestLinkifyMessage_UnresolvedWarns(t *testing.T) {
	api := &fakeSlackAPI{users: drewUser()}
	var warn bytes.Buffer
	got, err := linkifyMessage(api, "ping @nobody", false, &warn)
	if err != nil {
		t.Fatalf("linkifyMessage error: %v", err)
	}
	if got != "ping @nobody" {
		t.Fatalf("unresolved mention should stay literal, got %q", got)
	}
	if !strings.Contains(warn.String(), "nobody") {
		t.Fatalf("expected warning mentioning 'nobody', got %q", warn.String())
	}
}

func TestLinkifyMessage_EmailUntouchedNoWarn(t *testing.T) {
	api := &fakeSlackAPI{users: drewUser()}
	var warn bytes.Buffer
	got, err := linkifyMessage(api, "reach me at drew@example.com", false, &warn)
	if err != nil {
		t.Fatalf("linkifyMessage error: %v", err)
	}
	if got != "reach me at drew@example.com" {
		t.Fatalf("email should be untouched, got %q", got)
	}
	if warn.Len() != 0 {
		t.Fatalf("email should not warn, got %q", warn.String())
	}
}

func TestLinkifyMessage_APIErrorFails(t *testing.T) {
	api := &fakeSlackAPI{usersErr: errors.New("missing_scope")}
	var warn bytes.Buffer
	_, err := linkifyMessage(api, "ping @drew", false, &warn)
	if err == nil {
		t.Fatal("expected error when user resolution fails with a mention present")
	}
}

func TestLinkifyMessage_AuthErrorMapped(t *testing.T) {
	api := &fakeSlackAPI{usersErr: errors.New("token_revoked")}
	var warn bytes.Buffer
	_, err := linkifyMessage(api, "ping @drew", false, &warn)
	if !isAuthError(errors.New("token_revoked")) {
		t.Fatal("precondition: token_revoked should be an auth error")
	}
	var se *errs.SlackError
	if !errors.As(err, &se) || se.Code != errs.Auth {
		t.Fatalf("expected auth-coded SlackError, got %v", err)
	}
}

var _ slackpkg.SlackAPI = (*fakeSlackAPI)(nil)
