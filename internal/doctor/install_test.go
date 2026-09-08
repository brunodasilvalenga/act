package doctor

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// noopCmd returns a command that runs the current test binary with a
// run-filter that matches nothing, so it exits 0 immediately. This is
// portable across the darwin/linux/windows CI matrix, unlike relying on a
// platform-specific binary such as "true".
func noopCmd() *exec.Cmd {
	return exec.Command(os.Args[0], "-test.run=NONE")
}

// failingCmd returns a command that reliably exits non-zero on all three
// CI platforms, by passing the test binary an unrecognized flag.
func failingCmd() *exec.Cmd {
	return exec.Command(os.Args[0], "-test.run=NONE", "-nonexistent-flag-xyz")
}

func TestRunDownloadAndInstall(t *testing.T) {
	t.Run("no extraction, success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("payload"))
		}))
		defer srv.Close()

		err := runDownloadAndInstall(&bytes.Buffer{}, "Downloading", srv.URL, "install-test-*.bin", nil, func(artifactPath string) *exec.Cmd {
			return noopCmd()
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("with extraction, success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("payload"))
		}))
		defer srv.Close()

		var destDirSeen string
		var artifactSeenByBuildCmd string
		wantArtifact := "the-fake-artifact-path"

		extract := func(archivePath, destDir string) (string, error) {
			if _, statErr := os.Stat(destDir); statErr != nil {
				t.Errorf("expected destDir %q to exist when extract is called, stat error: %v", destDir, statErr)
			}
			destDirSeen = destDir
			return filepath.Join(destDir, wantArtifact), nil
		}

		err := runDownloadAndInstall(&bytes.Buffer{}, "Downloading", srv.URL, "install-test-*.bin", extract, func(artifactPath string) *exec.Cmd {
			artifactSeenByBuildCmd = artifactPath
			return noopCmd()
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if destDirSeen == "" {
			t.Fatal("expected extract to be called with a non-empty destDir")
		}
		wantPath := filepath.Join(destDirSeen, wantArtifact)
		if artifactSeenByBuildCmd != wantPath {
			t.Fatalf("buildCmd got artifact path %q, want %q", artifactSeenByBuildCmd, wantPath)
		}
	})

	t.Run("download failure propagates", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		var extractCalled, buildCmdCalled bool
		extract := func(archivePath, destDir string) (string, error) {
			extractCalled = true
			return "", nil
		}
		buildCmd := func(artifactPath string) *exec.Cmd {
			buildCmdCalled = true
			return noopCmd()
		}

		err := runDownloadAndInstall(&bytes.Buffer{}, "Downloading", srv.URL, "install-test-*.bin", extract, buildCmd)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "download failed") {
			t.Fatalf("expected error to contain %q, got: %v", "download failed", err)
		}
		if extractCalled {
			t.Error("expected extract not to be called on download failure")
		}
		if buildCmdCalled {
			t.Error("expected buildCmd not to be called on download failure")
		}
	})

	t.Run("extract failure propagates", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("payload"))
		}))
		defer srv.Close()

		extract := func(archivePath, destDir string) (string, error) {
			return "", errors.New("boom")
		}
		var buildCmdCalled bool
		buildCmd := func(artifactPath string) *exec.Cmd {
			buildCmdCalled = true
			return noopCmd()
		}

		err := runDownloadAndInstall(&bytes.Buffer{}, "Downloading", srv.URL, "install-test-*.bin", extract, buildCmd)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Fatalf("expected error to contain %q, got: %v", "boom", err)
		}
		if buildCmdCalled {
			t.Error("expected buildCmd not to be called when extract fails")
		}
	})

	t.Run("command failure propagates", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("payload"))
		}))
		defer srv.Close()

		err := runDownloadAndInstall(&bytes.Buffer{}, "Downloading", srv.URL, "install-test-*.bin", nil, func(artifactPath string) *exec.Cmd {
			return failingCmd()
		})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "installer failed") {
			t.Fatalf("expected error to contain %q, got: %v", "installer failed", err)
		}
	})
}
