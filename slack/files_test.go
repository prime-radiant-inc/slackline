package slack

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
)

func TestUploadFiles_TwoFiles(t *testing.T) {
	var (
		getURLCalls    int
		putCalls       int
		completeCalls  int
		completeParams url.Values
		uploadedBytes  = map[string]string{}
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/files.getUploadURLExternal", func(w http.ResponseWriter, r *http.Request) {
		getURLCalls++
		_ = r.ParseForm()
		filename := r.Form.Get("filename")
		fileID := "F_" + strings.ReplaceAll(filename, ".", "_")
		uploadURL := "http://" + r.Host + "/upload?id=" + fileID
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"upload_url":"`+uploadURL+`","file_id":"`+fileID+`"}`)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		putCalls++
		body, _ := io.ReadAll(r.Body)
		uploadedBytes[r.URL.Query().Get("id")] = string(body)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/files.completeUploadExternal", func(w http.ResponseWriter, r *http.Request) {
		completeCalls++
		_ = r.ParseForm()
		completeParams = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"files":[{"id":"F_a_txt","title":"a.txt"},{"id":"F_b_txt","title":"b.txt"}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	pathA := filepath.Join(tmp, "a.txt")
	pathB := filepath.Join(tmp, "b.txt")
	if err := os.WriteFile(pathA, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte("bravo"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := newRealClientForTest(srv.URL+"/api/", "xoxb-test")

	files := []FileUpload{{Path: pathA, Title: "a.txt"}, {Path: pathB, Title: "b.txt"}}
	got, err := c.UploadFiles("C123", "100.000", "hello", files)
	if err != nil {
		t.Fatalf("UploadFiles returned error: %v", err)
	}
	if getURLCalls != 2 {
		t.Errorf("getUploadURLExternal calls = %d, want 2", getURLCalls)
	}
	if putCalls != 2 {
		t.Errorf("PUT upload calls = %d, want 2", putCalls)
	}
	if completeCalls != 1 {
		t.Errorf("completeUploadExternal calls = %d, want 1", completeCalls)
	}
	if completeParams.Get("channel_id") != "C123" {
		t.Errorf("channel_id = %q", completeParams.Get("channel_id"))
	}
	if completeParams.Get("thread_ts") != "100.000" {
		t.Errorf("thread_ts = %q", completeParams.Get("thread_ts"))
	}
	if completeParams.Get("initial_comment") != "hello" {
		t.Errorf("initial_comment = %q", completeParams.Get("initial_comment"))
	}
	var filesArg []map[string]string
	if err := json.Unmarshal([]byte(completeParams.Get("files")), &filesArg); err != nil {
		t.Fatalf("files param not valid JSON: %v", err)
	}
	if len(filesArg) != 2 {
		t.Fatalf("files param length = %d, want 2", len(filesArg))
	}
	if uploadedBytes["F_a_txt"] != "alpha" {
		t.Errorf("uploaded bytes for a.txt = %q", uploadedBytes["F_a_txt"])
	}
	if uploadedBytes["F_b_txt"] != "bravo" {
		t.Errorf("uploaded bytes for b.txt = %q", uploadedBytes["F_b_txt"])
	}
	if len(got) != 2 {
		t.Errorf("returned summaries = %d, want 2", len(got))
	}
}

func TestUploadFiles_NoThread(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/files.getUploadURLExternal", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"upload_url":"http://`+r.Host+`/u","file_id":"F1"}`)
	})
	mux.HandleFunc("/u", func(w http.ResponseWriter, r *http.Request) {})
	var thread string
	mux.HandleFunc("/api/files.completeUploadExternal", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		thread = r.Form.Get("thread_ts")
		_, _ = io.WriteString(w, `{"ok":true,"files":[{"id":"F1"}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "x")
	_ = os.WriteFile(p, []byte("x"), 0o600)

	c := newRealClientForTest(srv.URL+"/api/", "xoxb-test")
	if _, err := c.UploadFiles("C1", "", "", []FileUpload{{Path: p}}); err != nil {
		t.Fatal(err)
	}
	if thread != "" {
		t.Errorf("thread_ts = %q, want empty", thread)
	}
}

func TestUploadFiles_GetURLError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/files.getUploadURLExternal", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":false,"error":"file_too_big"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "x")
	_ = os.WriteFile(p, []byte("x"), 0o600)

	c := newRealClientForTest(srv.URL+"/api/", "xoxb-test")
	_, err := c.UploadFiles("C1", "", "", []FileUpload{{Path: p}})
	if err == nil {
		t.Fatal("expected error from getUploadURLExternal failure")
	}
	if !strings.Contains(err.Error(), "file_too_big") {
		t.Errorf("error should mention file_too_big, got: %v", err)
	}
}

// newRealClientForTest constructs a realClient with custom apiBase/token values
// pointed at an httptest server.
func newRealClientForTest(apiBase, token string) *realClient {
	return &realClient{
		Client:  goslack.New(token),
		apiBase: apiBase,
		token:   token,
	}
}
