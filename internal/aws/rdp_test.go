package aws

import (
	"os"
	"os/exec"
	"testing"
)

// fastExitCmd returns a command that runs the current test binary with a
// run-filter that matches nothing, so it exits almost immediately.
// extraArgs lets a case pass an unrecognized flag to force a non-zero exit.
// This mirrors the noopCmd()/failingCmd() pattern in
// internal/doctor/install_test.go, which uses the same trick to get a
// reliable, portable exit-0 or exit-nonzero subprocess without depending on
// a real external binary being present on the darwin/linux/windows CI
// matrix.
func fastExitCmd(extraArgs ...string) *exec.Cmd {
	args := append([]string{"-test.run=NONE"}, extraArgs...)
	return exec.Command(os.Args[0], args...)
}

func TestRunRDPTunnel_SubprocessFailsFast(t *testing.T) {
	cmd := fastExitCmd("-nonexistent-flag-xyz") // exits non-zero almost immediately
	var launched bool
	err := runRDPTunnel(cmd, 13389, true, func(int) { launched = true })
	if err == nil {
		t.Fatal("expected an error when the subprocess exits immediately with a failure, got nil")
	}
	if launched {
		t.Error("expected launchClient not to be called when the subprocess fails fast")
	}
}

func TestRunRDPTunnel_SubprocessExitsCleanFast(t *testing.T) {
	cmd := fastExitCmd() // exits 0 almost immediately
	var launched bool
	err := runRDPTunnel(cmd, 13390, true, func(int) { launched = true })
	if err == nil {
		t.Fatal("expected an error when the subprocess exits immediately, even with exit code 0")
	}
	if launched {
		t.Error("expected launchClient not to be called when the subprocess exits immediately")
	}
}
