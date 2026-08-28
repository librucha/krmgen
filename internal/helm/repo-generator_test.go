package helm

import (
	types "github.com/librucha/krmgen/internal"
	"testing"
)

func Test_repoHelmGenerator_login_noop(t *testing.T) {
	called := false
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		called = true
		return "", "", nil
	}

	g := newRepoHelmGenerator(&types.HelmChart{RepoUrl: "https://charts.example.com", Name: "mychart"})
	if err := g.login(); err != nil {
		t.Fatalf("login() error = %v, want nil", err)
	}
	if called {
		t.Error("login() invoked runCommand, want it to be a no-op (repo login is unsupported)")
	}
}

// Test_repoHelmGenerator_chartIdShort pins the host extraction. Until phase 6
// the character class had no dot, so the host was truncated at the first one
// ("charts" for "https://charts.example.com"). Phases 2 to 6 forbade behaviour
// changes, so that was captured as a known bug rather than fixed; it is fixed
// now. Nothing in production reads these two methods - addRepoArgs passes
// config.RepoUrl straight through and login is a no-op - so they exist only to
// satisfy the idProvider interface, and the fix changes no rendered output.
func Test_repoHelmGenerator_chartIdShort(t *testing.T) {
	tests := []struct {
		name    string
		repoUrl string
		want    string
	}{
		{
			name:    "hostname keeps its dots",
			repoUrl: "https://charts.example.com",
			want:    "charts.example.com",
		},
		{
			name:    "a path after the host is not part of it",
			repoUrl: "https://charts.example.com/repo",
			want:    "charts.example.com",
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
			name:      "combines the host with the chart name",
			repoUrl:   "https://charts.example.com",
			chartName: "mychart",
			want:      "charts.example.com/mychart",
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
