package helm

import (
	types "github.com/librucha/krmgen/internal"
	"testing"
)

func Test_ociHelmGenerator_chartId(t *testing.T) {
	tests := []struct {
		name      string
		repoUrl   string
		chartName string
		want      string
	}{
		{
			name:      "repo without trailing slash gets one added",
			repoUrl:   "oci://registry.example.com/helm",
			chartName: "mychart",
			want:      "oci://registry.example.com/helm/mychart",
		},
		{
			name:      "repo with trailing slash is used as-is",
			repoUrl:   "oci://registry.example.com/helm/",
			chartName: "mychart",
			want:      "oci://registry.example.com/helm/mychart",
		},
		{
			name:      "bare host repo",
			repoUrl:   "oci://registry.example.com",
			chartName: "mychart",
			want:      "oci://registry.example.com/mychart",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newOciHelmGenerator(&types.HelmChart{RepoUrl: tt.repoUrl, Name: tt.chartName})
			if got := g.chartId(); got != tt.want {
				t.Errorf("chartId() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test_ociHelmGenerator_chartIdShort covers chartIdShort feeding the
// registry host into `helm registry login` - unlike the repo generator's
// chartIdShort, helmRegistryRegexp's character class includes the dot, so a
// multi-label hostname is captured in full rather than truncated.
func Test_ociHelmGenerator_chartIdShort(t *testing.T) {
	tests := []struct {
		name      string
		repoUrl   string
		chartName string
		want      string
	}{
		{
			name:      "multi-label hostname is captured in full",
			repoUrl:   "oci://registry.example.com/helm",
			chartName: "mychart",
			want:      "registry.example.com",
		},
		{
			name:      "bare host repo",
			repoUrl:   "oci://registry.example.com",
			chartName: "mychart",
			want:      "registry.example.com",
		},
		{
			name:      "no scheme match falls back to chartId itself",
			repoUrl:   "not-a-url",
			chartName: "mychart",
			want:      "not-a-url/mychart",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newOciHelmGenerator(&types.HelmChart{RepoUrl: tt.repoUrl, Name: tt.chartName})
			if got := g.chartIdShort(); got != tt.want {
				t.Errorf("chartIdShort() = %q, want %q", got, tt.want)
			}
		})
	}
}
