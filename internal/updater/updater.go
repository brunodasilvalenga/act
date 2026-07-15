package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const repoAPI = "https://api.github.com/repos/brunodasilvalenga/act/releases/latest"

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func CheckLatestVersion() (string, error) {
	resp, err := http.Get(repoAPI)
	if err != nil {
		return "", fmt.Errorf("failed to check latest version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release info: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func Upgrade(currentVersion string) error {
	resp, err := http.Get(repoAPI)
	if err != nil {
		return fmt.Errorf("failed to fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse release info: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if latestVersion == currentVersion {
		fmt.Println("Already up to date.")
		return nil
	}

	assetName := buildAssetName(latestVersion)
	downloadURL, checksumsURL := selectReleaseAssets(release.Assets, assetName)

	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (expected %s)", runtime.GOOS, runtime.GOARCH, assetName)
	}

	fmt.Printf("Downloading %s...\n", assetName)

	tmpFile, err := downloadToTemp(downloadURL)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	if checksumsURL == "" {
		return fmt.Errorf("checksums.txt asset not found in release %s; refusing to install an unverified binary", release.TagName)
	}
	if err := verifyChecksum(tmpFile, assetName, checksumsURL); err != nil {
		return err
	}

	binary, err := extractBinary(tmpFile, assetName)
	if err != nil {
		return err
	}
	defer os.Remove(binary)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	if err := replaceBinary(binary, execPath); err != nil {
		return err
	}

	fmt.Printf("Successfully upgraded to v%s\n", latestVersion)
	return nil
}

func buildAssetName(version string) string {
	return buildAssetNameFor(version, runtime.GOOS, runtime.GOARCH)
}

func buildAssetNameFor(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf("act_%s_%s_%s.%s", version, goos, goarch, ext)
}

// selectReleaseAssets finds the download URL for the current platform's
// asset and the checksums.txt URL among a release's assets. It does not
// perform any I/O.
func selectReleaseAssets(assets []releaseAsset, assetName string) (downloadURL, checksumsURL string) {
	for _, asset := range assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumsURL = asset.BrowserDownloadURL
		}
	}
	return downloadURL, checksumsURL
}

func downloadToTemp(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "act-upgrade-*")
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

func verifyChecksum(filePath, assetName, checksumsURL string) error {
	resp, err := http.Get(checksumsURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read checksums: %w", err)
	}

	var expectedHash string
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			expectedHash = parts[0]
			break
		}
	}

	if expectedHash == "" {
		return fmt.Errorf("checksum not found for %s", assetName)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	fmt.Println("Checksum verified.")
	return nil
}

func replaceBinary(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open new binary: %w", err)
	}
	defer srcFile.Close()

	dstDir := filepath.Dir(dst)
	tmpDst, err := os.CreateTemp(dstDir, ".act-new-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for new binary: %w", err)
	}
	tmpDstPath := tmpDst.Name()
	defer os.Remove(tmpDstPath) // no-op if rename below succeeds

	if _, err := io.Copy(tmpDst, srcFile); err != nil {
		tmpDst.Close()
		return fmt.Errorf("failed to copy new binary: %w", err)
	}
	if err := tmpDst.Close(); err != nil {
		return fmt.Errorf("failed to finalize new binary: %w", err)
	}

	if err := os.Chmod(tmpDstPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions on new binary: %w", err)
	}

	// os.Rename is atomic on the same filesystem: dst is never left
	// missing or partially written, unlike remove-then-create.
	if err := os.Rename(tmpDstPath, dst); err != nil {
		return fmt.Errorf("failed to replace old binary: %w", err)
	}

	return nil
}
