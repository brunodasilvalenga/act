package doctor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDownloadToTempFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello from test server"))
		}))
		defer srv.Close()

		path, err := downloadToTempFile(srv.URL, "download-test-*.bin")
		if err != nil {
			t.Fatalf("downloadToTempFile() error = %v, want nil", err)
		}

		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected downloaded file to exist at %s: %v", path, statErr)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read downloaded file: %v", err)
		}
		if string(content) != "hello from test server" {
			t.Errorf("downloaded content = %q, want %q", string(content), "hello from test server")
		}

		if err := os.Remove(path); err != nil {
			t.Errorf("os.Remove(path) error = %v, want nil", err)
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		path, err := downloadToTempFile(srv.URL, "download-test-*.bin")
		if err == nil {
			t.Fatal("downloadToTempFile() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "download returned status") {
			t.Errorf("error = %q, want substring %q", err.Error(), "download returned status")
		}
		if path != "" {
			t.Errorf("path = %q, want empty string", path)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()

		path, err := downloadToTempFile(url, "download-test-*.bin")
		if err == nil {
			t.Fatal("downloadToTempFile() error = nil, want non-nil")
		}
		if path != "" {
			t.Errorf("path = %q, want empty string", path)
		}
	})
}
