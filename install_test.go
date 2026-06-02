package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallFailsWhenReleaseDigestDoesNotMatch(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll binDir: %v", err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("MkdirAll home: %v", err)
	}

	writeExecutable(t, filepath.Join(binDir, "uname"), `#!/bin/sh
case "$1" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
esac
`)
	ghLog := filepath.Join(tmp, "gh.log")
	writeExecutable(t, filepath.Join(binDir, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$GH_LOG"
case "$1 $2" in
  "auth status") exit 0 ;;
  "release view")
    case "$*" in
      *"tagName"*) echo v1.2.3; exit 0 ;;
      *"assets"*) echo sha256:0000000000000000000000000000000000000000000000000000000000000000; exit 0 ;;
    esac
    exit 1 ;;
  "release download")
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "--output" ]; then
        shift
        cat > "$1" <<'BIN'
#!/bin/sh
echo slackline test
BIN
        chmod +x "$1"
        exit 0
      fi
      shift
    done
    exit 1 ;;
esac
exit 1
`)

	cmd := exec.Command("bash", "./install.sh")
	cmd.Env = append(os.Environ(),
		"GH_LOG="+ghLog,
		"HOME="+home,
		"PATH="+binDir+":"+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("install should fail when release digest does not match; output:\n%s", out)
	}
	if !strings.Contains(string(out), "release asset digest mismatch") {
		t.Fatalf("install failed for the wrong reason; output:\n%s", out)
	}
	logBytes, readErr := os.ReadFile(ghLog)
	if readErr != nil {
		t.Fatalf("ReadFile gh log: %v", readErr)
	}
	if !strings.Contains(string(logBytes), "release view") {
		t.Fatalf("installer did not inspect release metadata; gh log:\n%s", logBytes)
	}
	if strings.Contains(string(logBytes), "attestation verify") {
		t.Fatalf("installer should not call unsupported attestation verification; gh log:\n%s", logBytes)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".local", "bin", "slackline")); !os.IsNotExist(statErr) {
		t.Fatalf("binary should not be installed when release digest does not match")
	}
}

func TestInstallVerifiesInstalledBinaryEvenWhenOlderBinaryIsOnPath(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll binDir: %v", err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("MkdirAll home: %v", err)
	}

	writeExecutable(t, filepath.Join(binDir, "uname"), `#!/bin/sh
case "$1" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
esac
`)
	writeExecutable(t, filepath.Join(binDir, "slackline"), `#!/bin/sh
echo slackline version v0.0.1
`)
	writeExecutable(t, filepath.Join(binDir, "gh"), `#!/bin/sh
case "$1 $2" in
  "auth status") exit 0 ;;
  "release view")
    case "$*" in
      *"tagName"*) echo v1.2.3; exit 0 ;;
      *"assets"*) echo sha256:d2b678635d6f9c7c4dd63b4498c689ef9c1ca8e75ea61b44eb63a7ef92a79be6; exit 0 ;;
    esac
    exit 1 ;;
  "release download")
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "--output" ]; then
        shift
        cat > "$1" <<'BIN'
#!/bin/sh
echo slackline version v1.2.3
BIN
        chmod +x "$1"
        exit 0
      fi
      shift
    done
    exit 1 ;;
esac
exit 1
`)

	cmd := exec.Command("bash", "./install.sh")
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+binDir+":"+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "slackline version v1.2.3") {
		t.Fatalf("installer did not verify the newly installed binary; output:\n%s", out)
	}
	if strings.Contains(string(out), "slackline version v0.0.1") {
		t.Fatalf("installer verified an older binary from PATH; output:\n%s", out)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}
