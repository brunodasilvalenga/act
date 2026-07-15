package aws

import (
	"fmt"
	"regexp"
)

var safeProfileRegionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateSSHProxyToken rejects values that would be unsafe to splice
// unescaped into an OpenSSH ProxyCommand string, which is executed by the
// user's shell. AWS profile and region names never legitimately contain
// characters outside this set.
func validateSSHProxyToken(value, kind string) error {
	if value == "" {
		return nil
	}
	if !safeProfileRegionPattern.MatchString(value) {
		return fmt.Errorf("%s %q contains characters not allowed in an SSH ProxyCommand (allowed: letters, digits, '.', '_', '-')", kind, value)
	}
	return nil
}
