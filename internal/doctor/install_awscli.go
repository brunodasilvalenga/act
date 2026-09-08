package doctor

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func describeAWSCLIInstall() string {
	switch runtime.GOOS {
	case "darwin":
		return "Download the official AWS CLI v2 .pkg installer from " +
			"awscli.amazonaws.com and install it with 'sudo installer' " +
			"(requires sudo password)"
	case "linux":
		return "Download the official AWS CLI v2 install bundle from " +
			"awscli.amazonaws.com, unzip it, and run its bundled " +
			"'./aws/install' script (requires sudo)"
	case "windows":
		return "Download the official AWS CLI v2 MSI installer from " +
			"awscli.amazonaws.com and run it silently via msiexec"
	default:
		return fmt.Sprintf("no automated installer available for GOOS=%s", runtime.GOOS)
	}
}

func installAWSCLI(w io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return installAWSCLIDarwin(w)
	case "linux":
		return installAWSCLILinux(w)
	case "windows":
		return installAWSCLIWindows(w)
	default:
		return fmt.Errorf("no automated AWS CLI installer for GOOS=%s; install manually: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html", runtime.GOOS)
	}
}

func installAWSCLIDarwin(w io.Writer) error {
	const url = "https://awscli.amazonaws.com/AWSCLIV2.pkg"
	err := runDownloadAndInstall(w, "Downloading AWS CLI v2 installer", url, "awscliv2-*.pkg", nil, func(artifactPath string) *exec.Cmd {
		return exec.Command("sudo", "installer", "-pkg", artifactPath, "-target", "/")
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "AWS CLI installed successfully.")
	return nil
}

func installAWSCLILinux(w io.Writer) error {
	url := "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip"
	if runtime.GOARCH == "arm64" {
		url = "https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip"
	}

	extract := func(archivePath, destDir string) (string, error) {
		fmt.Fprintf(w, "Unzipping %s to %s...\n", archivePath, destDir)
		if err := unzipTo(archivePath, destDir); err != nil {
			return "", fmt.Errorf("unzip failed: %w", err)
		}
		return filepath.Join(destDir, "aws", "install"), nil
	}

	err := runDownloadAndInstall(w, "Downloading AWS CLI v2 install bundle", url, "awscli-exe-linux-*.zip", extract, func(artifactPath string) *exec.Cmd {
		return exec.Command("sudo", artifactPath)
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "AWS CLI installed successfully.")
	return nil
}

func installAWSCLIWindows(w io.Writer) error {
	const url = "https://awscli.amazonaws.com/AWSCLIV2.msi"
	err := runDownloadAndInstall(w, "Downloading AWS CLI v2 MSI installer", url, "awscliv2-*.msi", nil, func(artifactPath string) *exec.Cmd {
		return exec.Command("msiexec.exe", "/i", artifactPath, "/qn")
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "AWS CLI installed successfully.")
	return nil
}

// runDownloadAndInstall downloads url to a temp file (named per pattern, see
// downloadToTempFile), optionally extracts it (when extract is non-nil)
// into a fresh temp directory, then runs the *exec.Cmd built by buildCmd
// against the resulting artifact path, streaming its output to w. It
// returns an error if any stage fails. The downloaded file and any
// extraction temp dir are always cleaned up before returning.
//
// downloadMsg is printed verbatim followed by " from <url>...\n" (e.g.
// "Downloading AWS CLI v2 installer" becomes "Downloading AWS CLI v2
// installer from <url>...\n"), matching each caller's original wording.
func runDownloadAndInstall(w io.Writer, downloadMsg, url, tempPattern string, extract func(archivePath, destDir string) (artifactPath string, err error), buildCmd func(artifactPath string) *exec.Cmd) error {
	fmt.Fprintf(w, "%s from %s...\n", downloadMsg, url)
	downloaded, err := downloadToTempFile(url, tempPattern)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(downloaded)

	artifactPath := downloaded
	if extract != nil {
		tmpDir, err := os.MkdirTemp("", "act-install-*")
		if err != nil {
			return fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		artifactPath, err = extract(downloaded, tmpDir)
		if err != nil {
			return err
		}
	}

	cmd := buildCmd(artifactPath)
	fmt.Fprintf(w, "Running: %s\n", strings.Join(cmd.Args, " "))
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	return nil
}

// unzipTo extracts every regular file in the zip archive at zipPath into
// destDir, preserving the archive's relative directory structure.
func unzipTo(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		targetPath := filepath.Join(destDir, f.Name)
		if !isWithinDir(destDir, targetPath) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// isWithinDir reports whether target is contained within dir, guarding
// against zip-slip path traversal from malicious archive entries.
func isWithinDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	if filepath.IsAbs(rel) || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
