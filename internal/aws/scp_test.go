package aws

import "testing"

func TestScpEndpoints(t *testing.T) {
	tests := []struct {
		name            string
		download        bool
		wantLocal       string
		wantRemote      string
		wantRemoteFirst bool
	}{
		{"upload", false, "local.txt", "ec2-user@i-abc123:/remote/path", false},
		{"download", true, "local.txt", "ec2-user@i-abc123:/remote/path", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var source, dest string
			if tt.download {
				source, dest = "/remote/path", "local.txt"
			} else {
				source, dest = "local.txt", "/remote/path"
			}
			local, remote, remoteFirst := scpEndpoints("i-abc123", "ec2-user", source, dest, tt.download)
			if local != tt.wantLocal {
				t.Errorf("local = %q, want %q", local, tt.wantLocal)
			}
			if remote != tt.wantRemote {
				t.Errorf("remote = %q, want %q", remote, tt.wantRemote)
			}
			if remoteFirst != tt.wantRemoteFirst {
				t.Errorf("remoteFirst = %v, want %v", remoteFirst, tt.wantRemoteFirst)
			}
		})
	}
}
