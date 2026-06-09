package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

var (
	downloadFile  string
	downloadOut   string
	downloadForce bool
)

const (
	defaultMaxDownloadBytes = int64(100 * 1024 * 1024)
	errFileTooLarge         = "file_too_large"
)

var errDownloadBodyTooLarge = errors.New("download body exceeds size cap")

type downloadCapWriter struct {
	dst     io.Writer
	limit   int64
	written int64
}

func (w *downloadCapWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, errDownloadBodyTooLarge
	}
	if int64(len(p)) > remaining {
		n, err := w.dst.Write(p[:remaining])
		w.written += int64(n)
		if err != nil {
			return n, err
		}
		return n, errDownloadBodyTooLarge
	}
	n, err := w.dst.Write(p)
	w.written += int64(n)
	return n, err
}

func downloadBodyTooLargeError(capBytes int64) error {
	return &errs.SlackError{
		Code:   errs.Usage,
		Err:    errFileTooLarge,
		Detail: fmt.Sprintf("download body exceeds cap %d", capBytes),
	}
}

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download a file from Slack by file ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		api, err := loadReactAPI()
		if err != nil {
			return err
		}
		cap := defaultMaxDownloadBytes
		if v := os.Getenv("SLACKLINE_MAX_DOWNLOAD_BYTES"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				cap = n
			}
		}
		if downloadOut == "-" {
			return runDownloadWithAPIWriter(api, downloadFile, "-", downloadForce, cap, cmd.OutOrStdout(), cmd.OutOrStderr())
		}
		return runDownloadWithAPI(api, downloadFile, downloadOut, downloadForce, cap, cmd.OutOrStderr())
	},
}

func init() {
	downloadCmd.Flags().StringVar(&downloadFile, "file", "", "Slack file ID (F...) (required)")
	downloadCmd.Flags().StringVar(&downloadOut, "out", "", "output path, or '-' for stdout (required)")
	downloadCmd.Flags().BoolVar(&downloadForce, "force", false, "overwrite existing file at --out")
	_ = downloadCmd.MarkFlagRequired("file")
	_ = downloadCmd.MarkFlagRequired("out")
	rootCmd.AddCommand(downloadCmd)
}

// runDownloadWithAPI writes to a path on disk.
func runDownloadWithAPI(api slackpkg.SlackAPI, fileID, outPath string, force bool, capBytes int64, stderr io.Writer) error {
	info, _, _, err := api.GetFileInfo(fileID, 0, 0)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "get_file_info_failed", Detail: err.Error()}
	}
	if int64(info.Size) > capBytes {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    errFileTooLarge,
			Detail: fmt.Sprintf("file size %d exceeds cap %d (override with SLACKLINE_MAX_DOWNLOAD_BYTES)", info.Size, capBytes),
		}
	}
	if !force {
		if _, statErr := os.Stat(outPath); statErr == nil {
			return &errs.SlackError{Code: errs.Usage, Err: "out_exists", Detail: fmt.Sprintf("%s already exists; pass --force to overwrite", outPath)}
		}
	}
	parent := filepath.Dir(outPath)
	if _, err := os.Stat(parent); err != nil {
		return &errs.SlackError{Code: errs.Usage, Err: "no_parent_dir", Detail: fmt.Sprintf("parent dir %s does not exist", parent)}
	}
	tmp, err := os.CreateTemp(parent, "."+filepath.Base(outPath)+".*.tmp")
	if err != nil {
		return &errs.SlackError{Code: errs.Usage, Err: "tmp_open_failed", Detail: err.Error()}
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	cappedTmp := &downloadCapWriter{dst: tmp, limit: capBytes}
	if err := api.GetFile(info.URLPrivate, cappedTmp); err != nil {
		_ = tmp.Close()
		if errors.Is(err, errDownloadBodyTooLarge) {
			return downloadBodyTooLargeError(capBytes)
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "download_failed", Detail: err.Error()}
	}
	if err := tmp.Close(); err != nil {
		return &errs.SlackError{Code: errs.Usage, Err: "tmp_close_failed", Detail: err.Error()}
	}
	if force {
		if err := os.Rename(tmpPath, outPath); err != nil {
			return &errs.SlackError{Code: errs.Usage, Err: "rename_failed", Detail: err.Error()}
		}
		cleanupTmp = false
	} else {
		if err := os.Link(tmpPath, outPath); err != nil {
			if os.IsExist(err) {
				return &errs.SlackError{Code: errs.Usage, Err: "out_exists", Detail: fmt.Sprintf("%s already exists; pass --force to overwrite", outPath)}
			}
			return &errs.SlackError{Code: errs.Usage, Err: "commit_failed", Detail: err.Error()}
		}
	}
	return writeDownloadSummary(stderr, info, outPath)
}

// runDownloadWithAPIWriter writes to a stream (used for --out -).
func runDownloadWithAPIWriter(api slackpkg.SlackAPI, fileID, outPath string, force bool, capBytes int64, stdout, stderr io.Writer) error {
	if outPath != "-" {
		return runDownloadWithAPI(api, fileID, outPath, force, capBytes, stderr)
	}
	info, _, _, err := api.GetFileInfo(fileID, 0, 0)
	if err != nil {
		return &errs.SlackError{Code: errs.SlackAPI, Err: "get_file_info_failed", Detail: err.Error()}
	}
	if int64(info.Size) > capBytes {
		return &errs.SlackError{
			Code:   errs.Usage,
			Err:    errFileTooLarge,
			Detail: fmt.Sprintf("file size %d exceeds cap %d", info.Size, capBytes),
		}
	}
	cappedStdout := &downloadCapWriter{dst: stdout, limit: capBytes}
	if err := api.GetFile(info.URLPrivate, cappedStdout); err != nil {
		if errors.Is(err, errDownloadBodyTooLarge) {
			return downloadBodyTooLargeError(capBytes)
		}
		return &errs.SlackError{Code: errs.SlackAPI, Err: "download_failed", Detail: err.Error()}
	}
	return nil
}

func writeDownloadSummary(stderr io.Writer, info *goslack.File, path string) error {
	out := struct {
		OK       bool   `json:"ok"`
		File     string `json:"file"`
		Name     string `json:"name"`
		Mimetype string `json:"mimetype"`
		Size     int    `json:"size"`
		Path     string `json:"path"`
	}{OK: true, File: info.ID, Name: info.Name, Mimetype: info.Mimetype, Size: info.Size, Path: path}
	enc := json.NewEncoder(stderr)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
