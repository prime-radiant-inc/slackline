package cmd

import (
	"testing"
)

func TestSetVersion(t *testing.T) {
	t.Cleanup(func() { SetVersion("dev") })

	SetVersion("v1.2.3")
	if rootCmd.Version != "v1.2.3" {
		t.Errorf("rootCmd.Version = %q, want %q", rootCmd.Version, "v1.2.3")
	}
}
