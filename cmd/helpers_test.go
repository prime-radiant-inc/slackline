package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"

	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
)

const (
	fixtureMessageTS  = "123.456"
	fixtureChannelID  = "C123"
	fixtureUserID     = "U123"
	fixtureEmojiThumb = "thumbsup"
)

// uploadFilesCall records arguments from a single UploadFiles invocation.
type uploadFilesCall struct {
	channelID      string
	threadTS       string
	initialComment string
	files          []slackpkg.FileUpload
}

// capturedReaction records the arguments from a single AddReaction/RemoveReaction call.
type capturedReaction struct {
	Name string
	Item goslack.ItemRef
}

// fakeSlackAPI implements slackpkg.SlackAPI for testing cmd helpers.
type fakeSlackAPI struct {
	historyResp *goslack.GetConversationHistoryResponse
	historyErr  error
	// capturedHistoryParams records the last GetConversationHistory call.
	capturedHistoryParams *goslack.GetConversationHistoryParameters

	repliesMessages []goslack.Message
	repliesHasMore  bool
	repliesCursor   string
	repliesErr      error
	// repliesPages, when set, makes GetConversationReplies serve one page per
	// call, advancing by the cursor it returns. Overrides repliesMessages.
	repliesPages [][]goslack.Message
	// capturedRepliesParams records the last GetConversationReplies call.
	capturedRepliesParams *goslack.GetConversationRepliesParameters

	reactionsAdded    []capturedReaction
	reactionsRemoved  []capturedReaction
	addReactionErr    error
	removeReactionErr error

	getFileInfoFile *goslack.File
	getFileInfoErr  error

	getFileBytes []byte
	getFileErr   error

	lastUploadFilesCall *uploadFilesCall
	uploadFilesResp     []goslack.FileSummary
	uploadFilesErr      error
}

func (f *fakeSlackAPI) AuthTest() (*goslack.AuthTestResponse, error) {
	return &goslack.AuthTestResponse{}, nil
}

func (f *fakeSlackAPI) PostMessage(channelID string, options ...goslack.MsgOption) (string, string, error) {
	return "", "", nil
}

func (f *fakeSlackAPI) GetConversationHistory(params *goslack.GetConversationHistoryParameters) (*goslack.GetConversationHistoryResponse, error) {
	f.capturedHistoryParams = params
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.historyResp, nil
}

func (f *fakeSlackAPI) GetConversationReplies(params *goslack.GetConversationRepliesParameters) ([]goslack.Message, bool, string, error) {
	f.capturedRepliesParams = params
	if f.repliesErr != nil {
		return nil, false, "", f.repliesErr
	}
	if f.repliesPages != nil {
		idx := 0
		if params.Cursor != "" {
			idx, _ = strconv.Atoi(params.Cursor)
		}
		page := f.repliesPages[idx]
		hasMore := idx+1 < len(f.repliesPages)
		nextCursor := ""
		if hasMore {
			nextCursor = strconv.Itoa(idx + 1)
		}
		return page, hasMore, nextCursor, nil
	}
	return f.repliesMessages, f.repliesHasMore, f.repliesCursor, nil
}

func (f *fakeSlackAPI) GetConversations(params *goslack.GetConversationsParameters) ([]goslack.Channel, string, error) {
	return nil, "", nil
}

func (f *fakeSlackAPI) AddReaction(name string, item goslack.ItemRef) error {
	f.reactionsAdded = append(f.reactionsAdded, capturedReaction{Name: name, Item: item})
	return f.addReactionErr
}

func (f *fakeSlackAPI) RemoveReaction(name string, item goslack.ItemRef) error {
	f.reactionsRemoved = append(f.reactionsRemoved, capturedReaction{Name: name, Item: item})
	return f.removeReactionErr
}

func (f *fakeSlackAPI) GetFileInfo(fileID string, count, page int) (*goslack.File, []goslack.Comment, *goslack.Paging, error) {
	if f.getFileInfoErr != nil {
		return nil, nil, nil, f.getFileInfoErr
	}
	return f.getFileInfoFile, nil, nil, nil
}

func (f *fakeSlackAPI) GetFile(downloadURL string, writer io.Writer) error {
	if f.getFileErr != nil {
		return f.getFileErr
	}
	if f.getFileBytes != nil {
		_, err := writer.Write(f.getFileBytes)
		return err
	}
	return nil
}

func (f *fakeSlackAPI) UploadFiles(channelID, threadTS, initialComment string, files []slackpkg.FileUpload) ([]goslack.FileSummary, error) {
	f.lastUploadFilesCall = &uploadFilesCall{
		channelID:      channelID,
		threadTS:       threadTS,
		initialComment: initialComment,
		files:          files,
	}
	if f.uploadFilesErr != nil {
		return nil, f.uploadFilesErr
	}
	return f.uploadFilesResp, nil
}

// Compile-time check that fakeSlackAPI satisfies slackpkg.SlackAPI.
var _ slackpkg.SlackAPI = (*fakeSlackAPI)(nil)

// Common test timestamp constants.
const (
	ts1 = "1.0"
	ts2 = "2.0"
	ts3 = "3.0"
)

// --- isAuthError tests ---

func TestIsAuthError_TokenRevoked(t *testing.T) {
	if !isAuthError(errors.New("token_revoked")) {
		t.Error("expected isAuthError to return true for token_revoked")
	}
}

func TestIsAuthError_InvalidAuth(t *testing.T) {
	if !isAuthError(errors.New("invalid_auth")) {
		t.Error("expected isAuthError to return true for invalid_auth")
	}
}

func TestIsAuthError_NotAuthed(t *testing.T) {
	if !isAuthError(errors.New("not_authed")) {
		t.Error("expected isAuthError to return true for not_authed")
	}
}

func TestIsAuthError_AccountInactive(t *testing.T) {
	if !isAuthError(errors.New("account_inactive")) {
		t.Error("expected isAuthError to return true for account_inactive")
	}
}

func TestIsAuthError_NormalError(t *testing.T) {
	if isAuthError(errors.New("channel_not_found")) {
		t.Error("expected isAuthError to return false for channel_not_found")
	}
}

func TestIsAuthError_Nil(t *testing.T) {
	if isAuthError(nil) {
		t.Error("expected isAuthError to return false for nil")
	}
}

// --- maskToken tests ---

func TestMaskToken_Empty(t *testing.T) {
	got := maskToken("")
	if got != "(none)" {
		t.Errorf("maskToken(\"\") = %q, want %q", got, "(none)")
	}
}

func TestMaskToken_Short(t *testing.T) {
	// 5 chars: first 4 + "-..."
	got := maskToken("xoxb-")
	if got != "xoxb-..." {
		t.Errorf("maskToken(\"xoxb-\") = %q, want %q", got, "xoxb-...")
	}
}

func TestMaskToken_VeryShort(t *testing.T) {
	// 2 chars: first 2 (capped at len) + "-..."
	got := maskToken("ab")
	if got != "ab-..." {
		t.Errorf("maskToken(\"ab\") = %q, want %q", got, "ab-...")
	}
}

func TestMaskToken_Normal(t *testing.T) {
	// 20 chars: first 5 + "..." + last 4
	token := "xoxb-1234567890abcdef"
	got := maskToken(token)
	want := "xoxb-...cdef"
	if got != want {
		t.Errorf("maskToken(%q) = %q, want %q", token, got, want)
	}
}

func TestMaskToken_ExactlyTen(t *testing.T) {
	// 10 chars: first 5 + "..." + last 4 (overlap boundary)
	token := "0123456789"
	got := maskToken(token)
	want := "01234...6789"
	if got != want {
		t.Errorf("maskToken(%q) = %q, want %q", token, got, want)
	}
}

// --- fetchHistory tests ---

func makeMessage(ts, user, text string) goslack.Message {
	m := goslack.Message{}
	m.Timestamp = ts
	m.User = user
	m.Text = text
	return m
}

func TestFetchHistory_ChronologicalOrder(t *testing.T) {
	// Slack returns reverse-chronological: newest first.
	// fetchHistory should reverse to chronological: oldest first.
	api := &fakeSlackAPI{
		historyResp: &goslack.GetConversationHistoryResponse{
			Messages: []goslack.Message{
				makeMessage(ts3, "U1", "third"),
				makeMessage(ts2, "U1", "second"),
				makeMessage(ts1, "U1", "first"),
			},
		},
	}

	msgs, err := fetchHistory(api, fixtureChannelID, "", 10)
	if err != nil {
		t.Fatalf("fetchHistory returned error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("fetchHistory returned %d messages, want 3", len(msgs))
	}
	if msgs[0].Timestamp != ts1 {
		t.Errorf("msgs[0].Timestamp = %q, want %q", msgs[0].Timestamp, ts1)
	}
	if msgs[1].Timestamp != ts2 {
		t.Errorf("msgs[1].Timestamp = %q, want %q", msgs[1].Timestamp, ts2)
	}
	if msgs[2].Timestamp != ts3 {
		t.Errorf("msgs[2].Timestamp = %q, want %q", msgs[2].Timestamp, ts3)
	}
}

func TestFetchHistory_RespectsLimit(t *testing.T) {
	api := &fakeSlackAPI{
		historyResp: &goslack.GetConversationHistoryResponse{
			Messages: []goslack.Message{
				makeMessage("5.0", "U1", "five"),
				makeMessage("4.0", "U1", "four"),
				makeMessage(ts3, "U1", "three"),
				makeMessage(ts2, "U1", "two"),
				makeMessage(ts1, "U1", "one"),
			},
		},
	}

	msgs, err := fetchHistory(api, fixtureChannelID, "", 5)
	if err != nil {
		t.Fatalf("fetchHistory returned error: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("fetchHistory returned %d messages, want 5", len(msgs))
	}

	// Also test that limit truncates.
	msgs, err = fetchHistory(api, fixtureChannelID, "", 3)
	if err != nil {
		t.Fatalf("fetchHistory returned error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("fetchHistory returned %d messages, want 3", len(msgs))
	}
	// After reversal and truncation to 3, we should get the 3 newest in chronological order.
	if msgs[0].Timestamp != ts3 {
		t.Errorf("msgs[0].Timestamp = %q, want %q", msgs[0].Timestamp, ts3)
	}
}

func TestFetchHistory_WithOldest(t *testing.T) {
	api := &fakeSlackAPI{
		historyResp: &goslack.GetConversationHistoryResponse{
			Messages: []goslack.Message{
				makeMessage(ts2, "U1", "second"),
			},
		},
	}

	_, err := fetchHistory(api, fixtureChannelID, "1.5", 10)
	if err != nil {
		t.Fatalf("fetchHistory returned error: %v", err)
	}
	if api.capturedHistoryParams == nil {
		t.Fatal("expected capturedHistoryParams to be set")
	}
	if api.capturedHistoryParams.Oldest != "1.5" {
		t.Errorf("Oldest param = %q, want %q", api.capturedHistoryParams.Oldest, "1.5")
	}
}

// --- fetchReplies tests ---

func TestFetchReplies_ReturnsMessages(t *testing.T) {
	api := &fakeSlackAPI{
		repliesMessages: []goslack.Message{
			makeMessage(ts1, "U1", "parent"),
			makeMessage("1.1", "U2", "reply one"),
			makeMessage("1.2", "U3", "reply two"),
		},
	}

	msgs, err := fetchReplies(api, fixtureChannelID, ts1, "", 10)
	if err != nil {
		t.Fatalf("fetchReplies returned error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("fetchReplies returned %d messages, want 3", len(msgs))
	}
	if msgs[0].Timestamp != ts1 {
		t.Errorf("msgs[0].Timestamp = %q, want %q", msgs[0].Timestamp, ts1)
	}
	if msgs[2].Timestamp != "1.2" {
		t.Errorf("msgs[2].Timestamp = %q, want %q", msgs[2].Timestamp, "1.2")
	}
}

func TestFetchReplies_RespectsLimit(t *testing.T) {
	api := &fakeSlackAPI{
		repliesMessages: []goslack.Message{
			makeMessage(ts1, "U1", "parent"),
			makeMessage("1.1", "U2", "reply one"),
			makeMessage("1.2", "U3", "reply two"),
			makeMessage("1.3", "U4", "reply three"),
		},
	}

	msgs, err := fetchReplies(api, fixtureChannelID, ts1, "", 2)
	if err != nil {
		t.Fatalf("fetchReplies returned error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("fetchReplies returned %d messages, want 2", len(msgs))
	}
	// --limit counts from the newest: the 2 newest replies, not the oldest 2.
	if msgs[0].Timestamp != "1.2" {
		t.Errorf("msgs[0].Timestamp = %q, want %q", msgs[0].Timestamp, "1.2")
	}
	if msgs[1].Timestamp != "1.3" {
		t.Errorf("msgs[1].Timestamp = %q, want %q", msgs[1].Timestamp, "1.3")
	}
}

// TestFetchReplies_PaginatesToTail guards PRI-1879: conversations.replies
// returns messages oldest-first and paginates forward, so the newest replies
// live on the last page. fetchReplies must follow the cursor to the end and
// return the true tail, not stop after the first page.
func TestFetchReplies_PaginatesToTail(t *testing.T) {
	api := &fakeSlackAPI{
		repliesPages: [][]goslack.Message{
			{
				makeMessage(ts1, "U1", "parent"),
				makeMessage("1.1", "U2", "reply one"),
			},
			{
				makeMessage("1.2", "U3", "reply two"),
				makeMessage("1.3", "U4", "newest reply"),
			},
		},
	}

	// limit (3) is smaller than the thread total (4) and the newest reply is on
	// the last page: the early-stop bug would truncate after page 1 and miss it.
	msgs, err := fetchReplies(api, fixtureChannelID, ts1, "", 3)
	if err != nil {
		t.Fatalf("fetchReplies returned error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("fetchReplies returned %d messages, want 3", len(msgs))
	}
	if got := msgs[len(msgs)-1].Timestamp; got != "1.3" {
		t.Errorf("newest message ts = %q, want %q", got, "1.3")
	}
	if got := msgs[0].Timestamp; got != "1.1" {
		t.Errorf("oldest returned ts = %q, want %q (newest 3)", got, "1.1")
	}
	// Pages must be requested at full size, not shrunk toward --limit, or
	// pagination would stop short of the tail again.
	if got := api.capturedRepliesParams.Limit; got != 100 {
		t.Errorf("page Limit = %d, want 100", got)
	}
}

// --- messageOutput JSONL tests ---

func TestMessageOutput_JSONL(t *testing.T) {
	out := messageOutput{
		TS:   fixtureMessageTS,
		User: fixtureUserID,
		Text: "hello world",
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded["ts"] != fixtureMessageTS {
		t.Errorf("ts = %v, want %q", decoded["ts"], fixtureMessageTS)
	}
	if decoded["user"] != fixtureUserID {
		t.Errorf("user = %v, want %q", decoded["user"], fixtureUserID)
	}
	if decoded["text"] != "hello world" {
		t.Errorf("text = %v, want %q", decoded["text"], "hello world")
	}
	// thread_ts should be omitted when empty.
	if _, ok := decoded["thread_ts"]; ok {
		t.Error("thread_ts should be omitted when empty")
	}
}

func TestMessageOutput_JSONL_WithThreadTS(t *testing.T) {
	out := messageOutput{
		TS:       fixtureMessageTS,
		User:     fixtureUserID,
		Text:     "reply",
		ThreadTS: "100.000",
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded["thread_ts"] != "100.000" {
		t.Errorf("thread_ts = %v, want %q", decoded["thread_ts"], "100.000")
	}
}

// --- writeMessage tests ---

func TestWriteMessage_JSONL(t *testing.T) {
	var buf bytes.Buffer
	msg := goslack.Message{}
	msg.Timestamp = ts1
	msg.User = "U1"
	msg.Text = "hello"

	writeMessage(&buf, msg)

	var decoded map[string]string
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded["ts"] != ts1 {
		t.Errorf("ts = %q, want %q", decoded["ts"], ts1)
	}
	if decoded["user"] != "U1" {
		t.Errorf("user = %q, want %q", decoded["user"], "U1")
	}
	if decoded["text"] != "hello" {
		t.Errorf("text = %q, want %q", decoded["text"], "hello")
	}
	if _, ok := decoded["thread_ts"]; ok {
		t.Error("thread_ts should be absent when empty")
	}
}

func TestWriteMessage_WithThreadTS(t *testing.T) {
	var buf bytes.Buffer
	msg := goslack.Message{}
	msg.Timestamp = ts1
	msg.User = "U1"
	msg.Text = "reply"
	msg.ThreadTimestamp = "0.5"

	writeMessage(&buf, msg)

	var decoded map[string]string
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded["thread_ts"] != "0.5" {
		t.Errorf("thread_ts = %q, want %q", decoded["thread_ts"], "0.5")
	}
}
