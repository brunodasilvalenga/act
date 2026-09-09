package aws

import "testing"

func TestValidateSSHProxyToken(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"plain profile", "production", false},
		{"profile with dashes and dots", "my-profile_v1.2", false},
		{"plain region", "us-west-2", false},
		{"semicolon injection attempt", "prod; rm -rf ~", true},
		{"backtick injection attempt", "prod`whoami`", true},
		{"dollar injection attempt", "prod$(whoami)", true},
		{"space", "prod eu", true},
		{"pipe", "prod|cat", true},
		{"plain username", "ec2-user", false},
		{"ubuntu username", "ubuntu", false},
		{"root username", "root", false},
		{"user with equals sign", "user=evil", true},
		{"user with space", "ec2 user", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSSHProxyToken(tt.value, "profile")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSSHProxyToken(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}
