package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
)

const (
	testFileContent = "hello"
	fixtureFileName = "a.txt"
)

func TestDownload_ToPath(t *testing.T) {
	api := &fakeSlackAPI{
		getFileInfoFile: &goslack.File{ID: "F123", Name: "report.pdf", Mimetype: "application/pdf", Size: 5, URLPrivate: "https://files.slack.com/F123"},
		getFileBytes:    []byte(testFileContent),
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "report.pdf")
	stderr := &bytes.Buffer{}
	if err := runDownloadWithAPI(api, "F123", out, false, 100*1024*1024, stderr); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != testFileContent {
		t.Errorf("file contents = %q, want %q", got, testFileContent)
	}
	var summary map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
		t.Fatalf("stderr not valid JSON: %v\n%s", err, stderr.String())
	}
	if summary["ok"] != true {
		t.Errorf("ok = %v", summary["ok"])
	}
	if summary["path"] != out {
		t.Errorf("path = %v, want %s", summary["path"], out)
	}
}

func TestDownload_ToStdout(t *testing.T) {
	api := &fakeSlackAPI{
		getFileInfoFile: &goslack.File{ID: "F1", Name: fixtureFileName, Size: 5, URLPrivate: "https://files.slack.com/F1"},
		getFileBytes:    []byte(testFileContent),
	}
	stdout := &bytes.Buffer{}
	if err := runDownloadWithAPIWriter(api, "F1", "-", false, 100*1024*1024, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if stdout.String() != testFileContent {
		t.Errorf("stdout = %q, want %q", stdout.String(), testFileContent)
	}
}

func TestDownload_ExistingFileNoForce(t *testing.T) {
	api := &fakeSlackAPI{
		getFileInfoFile: &goslack.File{ID: "F1", Name: fixtureFileName, Size: 5, URLPrivate: "x"},
		getFileBytes:    []byte(testFileContent),
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "x.txt")
	_ = os.WriteFile(out, []byte("existing"), 0o600)

	err := runDownloadWithAPI(api, "F1", out, false, 100*1024*1024, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when target exists and --force is false")
	}
	got, _ := os.ReadFile(out)
	if string(got) != "existing" {
		t.Errorf("file should be untouched, got %q", got)
	}
}

func TestDownload_ExistingFileForce(t *testing.T) {
	api := &fakeSlackAPI{
		getFileInfoFile: &goslack.File{ID: "F1", Name: fixtureFileName, Size: 5, URLPrivate: "x"},
		getFileBytes:    []byte("new"), //nolint:goconst // unique value used only in force-overwrite test
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "x.txt")
	_ = os.WriteFile(out, []byte("existing"), 0o600)

	err := runDownloadWithAPI(api, "F1", out, true, 100*1024*1024, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("download with --force failed: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "new" {
		t.Errorf("file should be overwritten with new content; got %q", got)
	}
}

func TestDownload_IgnoresPreexistingSidecarSymlink(t *testing.T) {
	api := &fakeSlackAPI{
		getFileInfoFile: &goslack.File{ID: "F1", Name: fixtureFileName, Size: 5, URLPrivate: "x"},
		getFileBytes:    []byte(testFileContent),
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "x.txt")
	victim := filepath.Join(tmp, "victim.txt")
	if err := os.WriteFile(victim, []byte("victim"), 0o600); err != nil {
		t.Fatalf("WriteFile victim: %v", err)
	}
	if err := os.Symlink(victim, out+".tmp"); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := runDownloadWithAPI(api, "F1", out, false, 100*1024*1024, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	gotVictim, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("ReadFile victim: %v", err)
	}
	if string(gotVictim) != "victim" {
		t.Errorf("preexisting sidecar symlink target was modified: %q", gotVictim)
	}
	gotOut, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile out: %v", err)
	}
	if string(gotOut) != testFileContent {
		t.Errorf("downloaded file = %q, want %q", gotOut, testFileContent)
	}
}

func TestDownload_FinalPathRaceDoesNotOverwriteWithoutForce(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "x.txt")
	api := &fakeSlackAPI{
		getFileInfoFile: &goslack.File{ID: "F1", Name: fixtureFileName, Size: 5, URLPrivate: "x"},
		getFileBytes:    []byte(testFileContent),
		beforeGetFileWrite: func() {
			if err := os.WriteFile(out, []byte("raced"), 0o600); err != nil {
				t.Fatalf("WriteFile raced target: %v", err)
			}
		},
	}

	err := runDownloadWithAPI(api, "F1", out, false, 100*1024*1024, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when final path appears before commit")
	}
	got, readErr := os.ReadFile(out)
	if readErr != nil {
		t.Fatalf("ReadFile out: %v", readErr)
	}
	if string(got) != "raced" {
		t.Errorf("raced file should be untouched, got %q", got)
	}
}

func TestDownload_SizeExceedsCap(t *testing.T) {
	api := &fakeSlackAPI{
		getFileInfoFile: &goslack.File{ID: "F1", Name: "big.bin", Size: 1024 * 1024},
	}
	err := runDownloadWithAPI(api, "F1", "/tmp/should-not-exist", false, 100, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when file size exceeds cap")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention 'exceeds', got: %v", err)
	}
}

func TestDownload_GetFileInfoError(t *testing.T) {
	api := &fakeSlackAPI{getFileInfoErr: errors.New("file_not_found")}
	err := runDownloadWithAPI(api, "F404", "/tmp/x", false, 100*1024*1024, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "file_not_found") {
		t.Errorf("error should mention file_not_found, got: %v", err)
	}
}
