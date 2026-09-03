package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseGlobalFlags(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantRemaining   []string
		wantProfile     string
		wantRegion      string
		wantEnv         string
		wantShowVersion bool
	}{
		{
			name:          "no flags",
			args:          []string{"ec2"},
			wantRemaining: []string{"ec2"},
		},
		{
			name:          "profile flag",
			args:          []string{"--profile", "prod", "ec2"},
			wantRemaining: []string{"ec2"},
			wantProfile:   "prod",
		},
		{
			name:          "region flag",
			args:          []string{"--region", "us-west-2", "ec2"},
			wantRemaining: []string{"ec2"},
			wantRegion:    "us-west-2",
		},
		{
			name:          "env flag",
			args:          []string{"--env", "staging", "ec2"},
			wantRemaining: []string{"ec2"},
			wantEnv:       "staging",
		},
		{
			name:            "version flag",
			args:            []string{"--version"},
			wantRemaining:   nil,
			wantShowVersion: true,
		},
		{
			name:          "single-dash variants",
			args:          []string{"-profile", "prod", "-region", "us-east-1", "ec2"},
			wantRemaining: []string{"ec2"},
			wantProfile:   "prod",
			wantRegion:    "us-east-1",
		},
		{
			name:          "flag with no following value is dropped silently",
			args:          []string{"ec2", "--profile"},
			wantRemaining: []string{"ec2"},
		},
		{
			name:          "flags interspersed with subcommand args",
			args:          []string{"ec2", "--tag", "Name=x", "--profile", "prod"},
			wantRemaining: []string{"ec2", "--tag", "Name=x"},
			wantProfile:   "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var profile, region, env string
			var showVersion bool
			remaining := parseGlobalFlags(tt.args, &profile, &region, &env, &showVersion)

			if !reflect.DeepEqual(remaining, tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", remaining, tt.wantRemaining)
			}
			if profile != tt.wantProfile {
				t.Errorf("profile = %q, want %q", profile, tt.wantProfile)
			}
			if region != tt.wantRegion {
				t.Errorf("region = %q, want %q", region, tt.wantRegion)
			}
			if env != tt.wantEnv {
				t.Errorf("env = %q, want %q", env, tt.wantEnv)
			}
			if showVersion != tt.wantShowVersion {
				t.Errorf("showVersion = %v, want %v", showVersion, tt.wantShowVersion)
			}
		})
	}
}

func TestHasHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"empty args", []string{}, false},
		{"help keyword", []string{"help"}, true},
		{"double-dash help", []string{"--help"}, true},
		{"short help flag", []string{"-h"}, true},
		{"not help", []string{"ec2"}, false},
		{"help not in first position", []string{"ec2", "help"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasHelp(tt.args); got != tt.want {
				t.Errorf("hasHelp(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestSubcommandNeedsAWSCLI(t *testing.T) {
	tests := []struct {
		name   string
		subcmd string
		want   bool
	}{
		{"doctor does not need aws cli", "doctor", false},
		{"ec2 needs aws cli", "ec2", true},
		{"forward needs aws cli", "forward", true},
		{"ecs needs aws cli", "ecs", true},
		{"ssm needs aws cli", "ssm", true},
		{"rds needs aws cli", "rds", true},
		{"fav needs aws cli", "fav", true},
		{"env needs aws cli (unchanged behavior)", "env", true},
		{"init needs aws cli (unchanged behavior)", "init", true},
		{"upgrade needs aws cli (unchanged behavior)", "upgrade", true},
		{"empty subcmd needs aws cli (unchanged behavior)", "", true},
		{"unknown subcmd needs aws cli (unchanged behavior)", "bogus", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subcommandNeedsAWSCLI(tt.subcmd); got != tt.want {
				t.Errorf("subcommandNeedsAWSCLI(%q) = %v, want %v", tt.subcmd, got, tt.want)
			}
		})
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantRemaining []string
		wantTags      []string
	}{
		{
			name:          "no tags",
			args:          []string{"--local-port", "5432"},
			wantRemaining: []string{"--local-port", "5432"},
		},
		{
			name:          "single tag",
			args:          []string{"--tag", "Environment=prod"},
			wantRemaining: nil,
			wantTags:      []string{"Environment=prod"},
		},
		{
			name:          "multiple tags interspersed",
			args:          []string{"--tag", "Environment=prod", "--local-port", "5432", "--tag", "Team=platform"},
			wantRemaining: []string{"--local-port", "5432"},
			wantTags:      []string{"Environment=prod", "Team=platform"},
		},
		{
			name:          "single-dash tag flag",
			args:          []string{"-tag", "Name=bastion"},
			wantRemaining: nil,
			wantTags:      []string{"Name=bastion"},
		},
		{
			name:          "tag with no following value is dropped silently",
			args:          []string{"--tag"},
			wantRemaining: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining, tags := parseTags(tt.args)
			if !reflect.DeepEqual(remaining, tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", remaining, tt.wantRemaining)
			}
			if !reflect.DeepEqual(tags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", tags, tt.wantTags)
			}
		})
	}
}

func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestHelpFunctionsContainDocumentedFlags(t *testing.T) {
	tests := []struct {
		name         string
		fn           func()
		wantContains []string
	}{
		{"printUsage", printUsage, []string{"ec2", "forward", "ecs", "rds", "fav", "init", "doctor", "upgrade", "--profile", "--region", "--env", "--version"}},
		{"printEC2Help", printEC2Help, []string{"ssh", "rdp", "cp", "--tag"}},
		{"printForwardHelp", printForwardHelp, []string{"--local-port", "--remote-port", "--target", "--remote-host", "--tag"}},
		{"printECSHelp", printECSHelp, []string{"logs", "--cluster", "--service"}},
		{"printRDSHelp", printRDSHelp, []string{"--local-port", "--bastion", "--no-bastion", "--tag"}},
		{"printECSLogsHelp", printECSLogsHelp, []string{"--cluster", "--service", "--log-group", "--since", "--no-follow"}},
		{"printEC2SSHHelp", printEC2SSHHelp, []string{"--target", "--user", "--tag", "--push-key", "--push-key-path"}},
		{"printFavHelp", printFavHelp, []string{"add", "rm"}},
		{"printDoctorHelp", printDoctorHelp, []string{"--profile", "--region"}},
		{"printInitHelp", printInitHelp, []string{"~/.act.json"}},
		{"printEC2RDPHelp", printEC2RDPHelp, []string{"--target", "--local-port", "--key", "--no-open", "--tag"}},
		{"printEC2CPHelp", printEC2CPHelp, []string{"--target", "--user", "--download", "--recursive", "--tag"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(tt.fn)
			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("%s output missing expected substring %q\nfull output:\n%s", tt.name, want, output)
				}
			}
		})
	}
}
