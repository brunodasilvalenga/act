package aws

import "testing"

func TestSSHProxyOptionArgs(t *testing.T) {
	const base = "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
	tests := []struct {
		name         string
		profile      string
		region       string
		wantProxyCmd string
	}{
		{
			name:         "profile and region set",
			profile:      "myprofile",
			region:       "us-west-2",
			wantProxyCmd: "ProxyCommand=" + base + " --profile myprofile --region us-west-2",
		},
		{
			name:         "empty profile and region",
			profile:      "",
			region:       "",
			wantProxyCmd: "ProxyCommand=" + base,
		},
		{
			name:         "profile set, region empty",
			profile:      "myprofile",
			region:       "",
			wantProxyCmd: "ProxyCommand=" + base + " --profile myprofile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sshProxyOptionArgs(tt.profile, tt.region)
			want := []string{
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", tt.wantProxyCmd,
			}
			if len(got) != len(want) {
				t.Fatalf("sshProxyOptionArgs(%q, %q) = %v, want %v", tt.profile, tt.region, got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("sshProxyOptionArgs(%q, %q)[%d] = %q, want %q", tt.profile, tt.region, i, got[i], want[i])
				}
			}
		})
	}
}
