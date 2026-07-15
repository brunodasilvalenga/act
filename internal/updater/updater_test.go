package updater

import "testing"

func TestSelectReleaseAssets(t *testing.T) {
	assets := []releaseAsset{
		{Name: "act_1.2.3_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/binary"},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums"},
		{Name: "act_1.2.3_darwin_amd64.tar.gz", BrowserDownloadURL: "https://example.com/other-binary"},
	}

	downloadURL, checksumsURL := selectReleaseAssets(assets, "act_1.2.3_linux_amd64.tar.gz")
	if downloadURL != "https://example.com/binary" {
		t.Errorf("expected binary download URL, got %q", downloadURL)
	}
	if checksumsURL != "https://example.com/checksums" {
		t.Errorf("expected checksums URL, got %q", checksumsURL)
	}
}

func TestSelectReleaseAssetsMissingChecksums(t *testing.T) {
	assets := []releaseAsset{
		{Name: "act_1.2.3_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/binary"},
	}

	downloadURL, checksumsURL := selectReleaseAssets(assets, "act_1.2.3_linux_amd64.tar.gz")
	if downloadURL != "https://example.com/binary" {
		t.Errorf("expected binary download URL, got %q", downloadURL)
	}
	if checksumsURL != "" {
		t.Errorf("expected empty checksums URL when asset is missing, got %q", checksumsURL)
	}
}
