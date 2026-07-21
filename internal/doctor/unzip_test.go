package doctor

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTestZip builds an in-memory zip archive from entries (in the given
// order, to keep the archive deterministic) and writes it to a temp file
// under t.TempDir(), returning its path.
func buildTestZip(t *testing.T, entries []struct{ name, content string }) string {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("zw.Create(%q) error = %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.content)); err != nil {
			t.Fatalf("write zip entry %q error = %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zw.Close() error = %v", err)
	}

	f, err := os.CreateTemp(t.TempDir(), "test-*.zip")
	if err != nil {
		t.Fatalf("os.CreateTemp() error = %v", err)
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		t.Fatalf("write zip file error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close zip file error = %v", err)
	}

	return f.Name()
}

func TestUnzipTo(t *testing.T) {
	t.Run("nested file", func(t *testing.T) {
		destDir := t.TempDir()
		zipPath := buildTestZip(t, []struct{ name, content string }{
			{name: "subdir/file.txt", content: "nested content"},
		})

		if err := unzipTo(zipPath, destDir); err != nil {
			t.Fatalf("unzipTo() error = %v, want nil", err)
		}

		content, err := os.ReadFile(filepath.Join(destDir, "subdir", "file.txt"))
		if err != nil {
			t.Fatalf("failed to read extracted file: %v", err)
		}
		if string(content) != "nested content" {
			t.Errorf("content = %q, want %q", string(content), "nested content")
		}
	})

	t.Run("top-level file", func(t *testing.T) {
		destDir := t.TempDir()
		zipPath := buildTestZip(t, []struct{ name, content string }{
			{name: "top.txt", content: "top level content"},
		})

		if err := unzipTo(zipPath, destDir); err != nil {
			t.Fatalf("unzipTo() error = %v, want nil", err)
		}

		content, err := os.ReadFile(filepath.Join(destDir, "top.txt"))
		if err != nil {
			t.Fatalf("failed to read extracted file: %v", err)
		}
		if string(content) != "top level content" {
			t.Errorf("content = %q, want %q", string(content), "top level content")
		}
	})

	t.Run("traversal is rejected", func(t *testing.T) {
		destDir := t.TempDir()
		zipPath := buildTestZip(t, []struct{ name, content string }{
			{name: "../../escaped.txt", content: "should never be written"},
		})

		err := unzipTo(zipPath, destDir)
		if err == nil {
			t.Fatal("unzipTo() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "illegal file path in zip") {
			t.Errorf("error = %q, want substring %q", err.Error(), "illegal file path in zip")
		}

		escapedPath := filepath.Join(destDir, "..", "..", "escaped.txt")
		if _, statErr := os.Stat(escapedPath); !os.IsNotExist(statErr) {
			t.Errorf("expected %s to not exist, stat err = %v", escapedPath, statErr)
		}

		entries, err := os.ReadDir(destDir)
		if err != nil {
			t.Fatalf("os.ReadDir(destDir) error = %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("destDir has %d entries, want 0", len(entries))
		}
	})
}
