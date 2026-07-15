package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/brunodasilvalenga/act/internal/config"
	"github.com/brunodasilvalenga/act/internal/updater"
	"github.com/charmbracelet/lipgloss"
)

type status int

const (
	statusPass status = iota
	statusWarn
	statusFail
)

type result struct {
	Name   string
	Status status
	Detail string
}

var (
	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
)

func Run(profile, region, version string) error {
	results := []result{
		checkAWSCLI(),
		checkSessionManagerPlugin(),
		checkCredentials(profile, region),
		checkRegion(region),
		checkProfile(profile),
		checkConfigFile(),
		checkVersion(version),
	}

	fmt.Println()
	hasFailure := false
	for _, r := range results {
		var icon string
		var style lipgloss.Style
		switch r.Status {
		case statusPass:
			icon = "✓"
			style = passStyle
		case statusWarn:
			icon = "○"
			style = warnStyle
		case statusFail:
			icon = "✗"
			style = failStyle
			hasFailure = true
		}
		fmt.Printf("%s %s: %s\n", style.Render(icon), r.Name, r.Detail)
	}
	fmt.Println()

	if hasFailure {
		fmt.Println(failStyle.Render("Some checks failed. Fix the issues above."))
		os.Exit(1)
	}
	fmt.Println(passStyle.Render("All checks passed!"))
	return nil
}

func checkAWSCLI() result {
	path, err := exec.LookPath("aws")
	if err != nil {
		return result{
			Name:   "AWS CLI",
			Status: statusFail,
			Detail: "not found. Install: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
		}
	}

	out, err := exec.Command("aws", "--version").Output()
	ver := strings.TrimSpace(string(out))
	if err != nil || ver == "" {
		ver = "unknown version"
	}
	return result{
		Name:   "AWS CLI",
		Status: statusPass,
		Detail: fmt.Sprintf("%s (%s)", path, ver),
	}
}

func checkSessionManagerPlugin() result {
	path, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		return result{
			Name:   "Session Manager plugin",
			Status: statusFail,
			Detail: "not found. Install: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html",
		}
	}
	return result{
		Name:   "Session Manager plugin",
		Status: statusPass,
		Detail: path,
	}
}

func checkCredentials(profile, region string) result {
	resolvedProfile := config.ResolveProfile(profile, "")
	resolvedRegion := config.ResolveRegion(region, "")

	args := []string{"sts", "get-caller-identity", "--output", "json"}
	if resolvedProfile != "" {
		args = append(args, "--profile", resolvedProfile)
	}
	if resolvedRegion != "" {
		args = append(args, "--region", resolvedRegion)
	}

	out, err := exec.Command("aws", args...).Output()
	if err != nil {
		detail := "authentication failed"
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			detail = strings.TrimSpace(string(ee.Stderr))
		}
		return result{
			Name:   "AWS credentials",
			Status: statusFail,
			Detail: detail,
		}
	}

	// Parse ARN and account from output
	output := string(out)
	arn := extractJSON(output, "Arn")
	account := extractJSON(output, "Account")
	detail := arn
	if account != "" {
		detail = fmt.Sprintf("%s (account: %s)", arn, account)
	}
	return result{
		Name:   "AWS credentials",
		Status: statusPass,
		Detail: detail,
	}
}

func checkRegion(flagRegion string) result {
	resolved := config.ResolveRegion(flagRegion, "")
	if resolved == "" {
		return result{
			Name:   "Region",
			Status: statusWarn,
			Detail: "not configured (use --region, AWS_REGION, or ~/.act.json)",
		}
	}
	return result{
		Name:   "Region",
		Status: statusPass,
		Detail: resolved,
	}
}

func checkProfile(flagProfile string) result {
	resolved := config.ResolveProfile(flagProfile, "")
	if resolved == "" {
		return result{
			Name:   "Profile",
			Status: statusWarn,
			Detail: "using default (no explicit profile set)",
		}
	}
	return result{
		Name:   "Profile",
		Status: statusPass,
		Detail: resolved,
	}
}

func checkConfigFile() result {
	home, err := os.UserHomeDir()
	if err != nil {
		return result{
			Name:   "Config",
			Status: statusWarn,
			Detail: "could not determine home directory",
		}
	}
	path := home + "/.act.json"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return result{
			Name:   "Config",
			Status: statusWarn,
			Detail: fmt.Sprintf("%s (not found - run `act init` to create)", path),
		}
	}
	// Try to parse it
	cfg := config.Load()
	_ = cfg
	return result{
		Name:   "Config",
		Status: statusPass,
		Detail: path,
	}
}

func checkVersion(version string) result {
	if version == "dev" {
		return result{
			Name:   "Version",
			Status: statusPass,
			Detail: "dev (development build)",
		}
	}

	latest, err := updater.CheckLatestVersion()
	if err != nil {
		return result{
			Name:   "Version",
			Status: statusWarn,
			Detail: fmt.Sprintf("%s (could not check for updates: %v)", version, err),
		}
	}

	if latest != version && latest != strings.TrimPrefix(version, "v") {
		return result{
			Name:   "Version",
			Status: statusWarn,
			Detail: fmt.Sprintf("%s (new version v%s available, run `act upgrade`)", version, latest),
		}
	}
	return result{
		Name:   "Version",
		Status: statusPass,
		Detail: fmt.Sprintf("%s (up to date)", version),
	}
}

func extractJSON(jsonStr, key string) string {
	// Simple extraction without importing encoding/json for a struct
	search := fmt.Sprintf(`"%s": "`, key)
	idx := strings.Index(jsonStr, search)
	if idx == -1 {
		return ""
	}
	start := idx + len(search)
	end := strings.Index(jsonStr[start:], `"`)
	if end == -1 {
		return ""
	}
	return jsonStr[start : start+end]
}
