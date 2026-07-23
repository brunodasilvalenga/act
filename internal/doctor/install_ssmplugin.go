package doctor

import (
	"fmt"
	"io"
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

	extract := func(archivePath, destDir string) (string, error) {
		fmt.Fprintf(w, "Unzipping %s to %s...\n", archivePath, destDir)
		if err := unzipTo(archivePath, destDir); err != nil {
			return "", fmt.Errorf("unzip failed: %w", err)
		}
		return filepath.Join(destDir, "sessionmanager-bundle", "install"), nil
	}

	err := runDownloadAndInstall(w, "Downloading Session Manager plugin bundle", url, "sessionmanager-bundle-*.zip", extract, func(artifactPath string) *exec.Cmd {
		return exec.Command("sudo", artifactPath, "-i", "/usr/local/sessionmanagerplugin", "-b", "/usr/local/bin/session-manager-plugin")
	})
	if err != nil {
		return err
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

	buildCmd := func(artifactPath string) *exec.Cmd {
		if useRpm {
			return exec.Command("sudo", "rpm", "-i", artifactPath)
		}
		return exec.Command("sudo", "dpkg", "-i", artifactPath)
	}

	err := runDownloadAndInstall(w, "Downloading Session Manager plugin package", url, pattern, nil, buildCmd)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Session Manager plugin installed successfully.")
	return nil
}

func installSSMPluginWindows(w io.Writer) error {
	const url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/windows/SessionManagerPluginSetup.exe"
	err := runDownloadAndInstall(w, "Downloading Session Manager plugin installer", url, "SessionManagerPluginSetup-*.exe", nil, func(artifactPath string) *exec.Cmd {
		return exec.Command(artifactPath, "/S")
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Session Manager plugin installed successfully.")
	return nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
