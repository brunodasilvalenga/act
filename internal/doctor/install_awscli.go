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
	fmt.Fprintf(w, "Downloading AWS CLI v2 installer from %s...\n", url)
	pkgPath, err := downloadToTempFile(url, "awscliv2-*.pkg")
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(pkgPath)

	fmt.Fprintf(w, "Running: sudo installer -pkg %s -target /\n", pkgPath)
	cmd := exec.Command("sudo", "installer", "-pkg", pkgPath, "-target", "/")
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	fmt.Fprintln(w, "AWS CLI installed successfully.")
	return nil
}

func installAWSCLILinux(w io.Writer) error {
	url := "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip"
	if runtime.GOARCH == "arm64" {
		url = "https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip"
	}

	fmt.Fprintf(w, "Downloading AWS CLI v2 install bundle from %s...\n", url)
	zipPath, err := downloadToTempFile(url, "awscli-exe-linux-*.zip")
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(zipPath)

	tmpDir, err := os.MkdirTemp("", "awscli-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintf(w, "Unzipping %s to %s...\n", zipPath, tmpDir)
	if err := unzipTo(zipPath, tmpDir); err != nil {
		return fmt.Errorf("unzip failed: %w", err)
	}

	installScript := filepath.Join(tmpDir, "aws", "install")
	fmt.Fprintf(w, "Running: sudo %s\n", installScript)
	cmd := exec.Command("sudo", installScript)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	fmt.Fprintln(w, "AWS CLI installed successfully.")
	return nil
}

func installAWSCLIWindows(w io.Writer) error {
	const url = "https://awscli.amazonaws.com/AWSCLIV2.msi"
	fmt.Fprintf(w, "Downloading AWS CLI v2 MSI installer from %s...\n", url)
	msiPath, err := downloadToTempFile(url, "awscliv2-*.msi")
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(msiPath)

	fmt.Fprintf(w, "Running: msiexec.exe /i %s /qn\n", msiPath)
	cmd := exec.Command("msiexec.exe", "/i", msiPath, "/qn")
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	fmt.Fprintln(w, "AWS CLI installed successfully.")
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
