package helm

import (
	types "github.com/librucha/krmgen/internal"
	"testing"
)

// Test_repoHelmGenerator_chartIdShort is a characterization test: it pins
// today's behaviour of helmUrlRegexp, including the bug. The regexp's
// character class ([0-9a-zA-Z-_]) has no dot, so it stops at the first dot
// in the host and returns a truncated hostname instead of the full one -
// "charts" for "https://charts.example.com", not "charts.example.com". This
// phase forbids behaviour changes, so the wrong output is captured on
// purpose. Do not "fix" this test by correcting the expected value without
// also fixing helmUrlRegexp and confirming that is an intended, reviewed
// behaviour change.
func Test_repoHelmGenerator_chartIdShort(t *testing.T) {
	tests := []struct {
		name    string
		repoUrl string
		want    string
	}{
		{
			name:    "hostname with a dot is truncated at the first dot (known bug)",
			repoUrl: "https://charts.example.com",
			want:    "charts",
		},
		{
			name:    "hostname with a dot and a path is truncated at the first dot (known bug)",
			repoUrl: "https://charts.example.com/repo",
			want:    "charts",
		},
		{
			name:    "single-label hostname matches in full",
			repoUrl: "https://charts",
			want:    "charts",
		},
		{
			name:    "no scheme match falls back to the raw repo URL",
			repoUrl: "not-a-url",
			want:    "not-a-url",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newRepoHelmGenerator(&types.HelmChart{RepoUrl: tt.repoUrl})
			if got := g.chartIdShort(); got != tt.want {
				t.Errorf("chartIdShort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_repoHelmGenerator_chartId(t *testing.T) {
	tests := []struct {
		name      string
		repoUrl   string
		chartName string
		want      string
	}{
		{
			name:      "combines the truncated hostname with the chart name (known bug in the hostname part)",
			repoUrl:   "https://charts.example.com",
			chartName: "mychart",
			want:      "charts/mychart",
		},
		{
			name:      "falls back to the raw repo URL when the regexp does not match",
			repoUrl:   "not-a-url",
			chartName: "mychart",
			want:      "not-a-url/mychart",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newRepoHelmGenerator(&types.HelmChart{RepoUrl: tt.repoUrl, Name: tt.chartName})
			if got := g.chartId(); got != tt.want {
				t.Errorf("chartId() = %q, want %q", got, tt.want)
			}
		})
	}
}
