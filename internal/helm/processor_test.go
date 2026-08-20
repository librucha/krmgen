package helm

import (
	cons "github.com/librucha/krmgen/internal/utils"
	"os"
	"os/exec"
	"testing"
)

func Test_helmExecutable(t *testing.T) {
	helmExec, _ := exec.LookPath("helm")
	tests := []struct {
		env  map[string]string
		name string
		want string
	}{
		{
			name: "fallback to default",
			want: helmExec,
		},
		{
			name: "from ENV",
			env:  map[string]string{cons.EnvHelmExecutable: "/usr/bin/myOwnHelmExec"},
			want: "/usr/bin/myOwnHelmExec",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				_ = os.Setenv(k, v)
			}
			if got := helmExecutable(); got != tt.want {
				t.Errorf("helmExecutable() = %v, want %v", got, tt.want)
			}
			for k, _ := range tt.env {
				_ = os.Unsetenv(k)
			}
		})
	}
}

func Test_stripHelmBanner(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no banner",
			in:   "---\n# Source: app/templates/svc.yaml\napiVersion: v1\n",
			want: "---\n# Source: app/templates/svc.yaml\napiVersion: v1\n",
		},
		{
			name: "helm v4 OCI banner",
			in:   "Pulled: reg.example.com/helm/app:3.0.0\nDigest: sha256:07495f6d\n---\napiVersion: v1\n",
			want: "---\napiVersion: v1\n",
		},
		{
			name: "provenance banner",
			in:   "Pulled: reg.example.com/helm/app:3.0.0\nDigest: sha256:07495f6d\nSigned by: Some One <a@b.c>\nChart Hash Verified: sha256:abc\n---\napiVersion: v1\n",
			want: "---\napiVersion: v1\n",
		},
		{
			name: "banner with CRLF line endings",
			in:   "Pulled: reg.example.com/helm/app:3.0.0\r\n---\napiVersion: v1\n",
			want: "---\napiVersion: v1\n",
		},
		{
			name: "banner only, no manifests",
			in:   "Pulled: reg.example.com/helm/app:3.0.0\nDigest: sha256:07495f6d\n",
			want: "",
		},
		{
			name: "banner-like text inside manifests is kept",
			in:   "---\napiVersion: v1\ndata:\n  note: |\n    Pulled: something\n",
			want: "---\napiVersion: v1\ndata:\n  note: |\n    Pulled: something\n",
		},
		{
			name: "empty output",
			in:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripHelmBanner(tt.in); got != tt.want {
				t.Errorf("stripHelmBanner() = %q, want %q", got, tt.want)
			}
		})
	}
}
