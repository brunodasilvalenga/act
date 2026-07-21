package doctor

import (
	"path/filepath"
	"testing"
)

// TestIsWithinDir locks in the current (correct) branches of isWithinDir,
// the zip-slip path-traversal guard used by unzipTo. isWithinDir is a pure
// function of two strings with no I/O, so this test uses no t.TempDir()
// and no real filesystem paths.
//
// Note: the filepath.Rel error branch (`if err != nil { return false }`)
// is intentionally not covered by a table case here. filepath.Rel only
// errors when the two paths can't be made relative to each other (e.g.
// different volume/drive letters on Windows, such as `C:\foo` vs
// `D:\bar`) — that isn't reliably or portably triggerable from a single
// table-driven test that must also pass on Linux and macOS, so this
// branch is left to manual code inspection instead of a forced,
// platform-specific test case.
func TestIsWithinDir(t *testing.T) {
	dir := filepath.Join("tmp", "extract")

	tests := []struct {
		name   string
		dir    string
		target string
		want   bool
	}{
		{
			name:   "normal nested file",
			dir:    dir,
			target: filepath.Join(dir, "subdir", "file.txt"),
			want:   true,
		},
		{
			name:   "sibling escape",
			dir:    dir,
			target: filepath.Join("tmp", "sibling", "file.txt"),
			want:   false,
		},
		{
			name:   "exact traversal - target is the parent dir itself",
			dir:    dir,
			target: filepath.Join(dir, ".."),
			want:   false,
		},
		{
			name:   "deep traversal",
			dir:    dir,
			target: filepath.Join(dir, "..", "..", "etc", "passwd"),
			want:   false,
		},
		{
			// filepath.Rel(dir, dir) returns ".", which is neither ".."
			// nor has the ".."+separator prefix, so isWithinDir correctly
			// treats "the root extraction dir itself" as within itself.
			name:   "target equals dir",
			dir:    dir,
			target: dir,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWithinDir(tt.dir, tt.target)
			if got != tt.want {
				t.Errorf("isWithinDir(%q, %q) = %v, want %v", tt.dir, tt.target, got, tt.want)
			}
		})
	}
}
