package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func describeSSMPluginInstall() string {
	switch runtime.GOOS {
	case "darwin":
		return "Download the official Session Manager plugin bundle for " +
			"macOS from session-manager-downloads.s3.amazonaws.com and " +
			"install it with 'sudo ./install' (requires sudo password)"
	case "linux":
		return "Download the official Session Manager plugin package for " +
			"Linux from session-manager-downloads.s3.amazonaws.com and " +
			"install it with 'sudo dpkg -i' or 'sudo rpm -i' (requires sudo)"
	case "windows":
		return "Download the official Session Manager plugin installer " +
			"from session-manager-downloads.s3.amazonaws.com and run it " +
			"silently"
	default:
		return fmt.Sprintf("no automated installer available for GOOS=%s", runtime.GOOS)
	}
}

func installSSMPlugin(w io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return installSSMPluginDarwin(w)
	case "linux":
		return installSSMPluginLinux(w)
	case "windows":
		return installSSMPluginWindows(w)
	default:
		return fmt.Errorf("no automated Session Manager plugin installer for GOOS=%s; install manually: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html", runtime.GOOS)
	}
}

func installSSMPluginDarwin(w io.Writer) error {
	const url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/mac/sessionmanager-bundle.zip"
	fmt.Fprintf(w, "Downloading Session Manager plugin bundle from %s...\n", url)
	zipPath, err := downloadToTempFile(url, "sessionmanager-bundle-*.zip")
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(zipPath)

	tmpDir, err := os.MkdirTemp("", "ssm-plugin-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintf(w, "Unzipping %s to %s...\n", zipPath, tmpDir)
	if err := unzipTo(zipPath, tmpDir); err != nil {
		return fmt.Errorf("unzip failed: %w", err)
	}

	installScript := filepath.Join(tmpDir, "sessionmanager-bundle", "install")
	fmt.Fprintf(w, "Running: sudo %s -i /usr/local/sessionmanagerplugin -b /usr/local/bin/session-manager-plugin\n", installScript)
	cmd := exec.Command("sudo", installScript, "-i", "/usr/local/sessionmanagerplugin", "-b", "/usr/local/bin/session-manager-plugin")
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	fmt.Fprintln(w, "Session Manager plugin installed successfully.")
	return nil
}

func installSSMPluginLinux(w io.Writer) error {
	hasDpkg := commandExists("dpkg")
	hasRpm := commandExists("rpm")

	if !hasDpkg && !hasRpm {
		return fmt.Errorf("neither dpkg nor rpm found on this system; cannot determine package format. Install manually: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html")
	}

	useRpm := hasRpm && !hasDpkg

	var url, pattern string
	if useRpm {
		url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/linux_64bit/session-manager-plugin.rpm"
		pattern = "session-manager-plugin-*.rpm"
	} else {
		url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/ubuntu_64bit/session-manager-plugin.deb"
		pattern = "session-manager-plugin-*.deb"
	}

	fmt.Fprintf(w, "Downloading Session Manager plugin package from %s...\n", url)
	pkgPath, err := downloadToTempFile(url, pattern)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(pkgPath)

	var cmd *exec.Cmd
	if useRpm {
		fmt.Fprintf(w, "Running: sudo rpm -i %s\n", pkgPath)
		cmd = exec.Command("sudo", "rpm", "-i", pkgPath)
	} else {
		fmt.Fprintf(w, "Running: sudo dpkg -i %s\n", pkgPath)
		cmd = exec.Command("sudo", "dpkg", "-i", pkgPath)
	}
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	fmt.Fprintln(w, "Session Manager plugin installed successfully.")
	return nil
}

func installSSMPluginWindows(w io.Writer) error {
	const url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/windows/SessionManagerPluginSetup.exe"
	fmt.Fprintf(w, "Downloading Session Manager plugin installer from %s...\n", url)
	exePath, err := downloadToTempFile(url, "SessionManagerPluginSetup-*.exe")
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(exePath)

	fmt.Fprintf(w, "Running: %s /S\n", exePath)
	cmd := exec.Command(exePath, "/S")
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	fmt.Fprintln(w, "Session Manager plugin installed successfully.")
	return nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
