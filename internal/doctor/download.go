package doctor

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

// downloadToTempFile downloads url to a new temp file created with the
// given pattern (see os.CreateTemp) and returns its path. The caller is
// responsible for removing the file (defer os.Remove(path)).
func downloadToTempFile(url, pattern string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to write download: %w", err)
	}

	return tmp.Name(), nil
}
