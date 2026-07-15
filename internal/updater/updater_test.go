package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	dstPath := filepath.Join(dir, "act")
	srcPath := filepath.Join(dir, "act-new")

	if err := os.WriteFile(dstPath, []byte("old binary content"), 0755); err != nil {
		t.Fatalf("setup: failed to write old binary: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("new binary content"), 0644); err != nil {
		t.Fatalf("setup: failed to write new binary: %v", err)
	}

	if err := replaceBinary(srcPath, dstPath); err != nil {
		t.Fatalf("replaceBinary failed: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read replaced binary: %v", err)
	}
	if string(got) != "new binary content" {
		t.Errorf("expected dst to contain new binary content, got %q", string(got))
	}

	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("failed to stat replaced binary: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %v", info.Mode().Perm())
	}

	// No leftover temp file in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "act" && e.Name() != "act-new" {
			t.Errorf("unexpected leftover file in dst dir: %s", e.Name())
		}
	}
}

func TestReplaceBinaryMissingSource(t *testing.T) {
	dir := t.TempDir()
	dstPath := filepath.Join(dir, "act")
	if err := os.WriteFile(dstPath, []byte("old binary content"), 0755); err != nil {
		t.Fatalf("setup: failed to write old binary: %v", err)
	}

	err := replaceBinary(filepath.Join(dir, "does-not-exist"), dstPath)
	if err == nil {
		t.Fatal("expected error when source binary is missing, got nil")
	}

	// dst must be untouched — this is the core regression check.
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("dst was removed even though replaceBinary failed: %v", err)
	}
	if string(got) != "old binary content" {
		t.Errorf("dst content changed despite replaceBinary failing: got %q", string(got))
	}
}
