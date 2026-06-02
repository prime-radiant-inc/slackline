package cmd

import (
	"io"
	"testing"
)

func stubReadSecretLine(t *testing.T, values ...string) func() {
	t.Helper()
	original := readSecretLine
	calls := 0
	readSecretLine = func(_ io.Reader, _ string, _ io.Writer) (string, error) {
		if calls >= len(values) {
			t.Fatalf("readSecretLine called %d times, only %d values stubbed", calls+1, len(values))
		}
		value := values[calls]
		calls++
		return value, nil
	}
	return func() {
		readSecretLine = original
		if calls != len(values) {
			t.Fatalf("readSecretLine called %d times, want %d", calls, len(values))
		}
	}
}
