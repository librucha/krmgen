package helm

import (
	"errors"
	types "github.com/librucha/krmgen/internal"
	"reflect"
	"strings"
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

func Test_ociHelmGenerator_login_failure(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		return "", "unauthorized", errors.New("exit status 1")
	}

	g := newOciHelmGenerator(&types.HelmChart{RepoUrl: "oci://registry.example.com/helm", Name: "mychart"})
	err := g.login()
	if err == nil {
		t.Fatal("login() error = nil, want the helm failure to propagate")
	}
	if !strings.Contains(err.Error(), "login to helm registry") {
		t.Errorf("error = %v, want it to contain %q", err, "login to helm registry")
	}
	if !strings.Contains(err.Error(), "registry.example.com") {
		t.Errorf("error = %v, want it to contain the registry name %q", err, "registry.example.com")
	}
}

func Test_ociHelmGenerator_login_success(t *testing.T) {
	var gotArgs []string
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		gotArgs = arg
		return "", "", nil
	}

	g := newOciHelmGenerator(&types.HelmChart{
		RepoUrl:  "oci://registry.example.com/helm",
		Name:     "mychart",
		Username: "user",
		Password: "pass",
	})
	if err := g.login(); err != nil {
		t.Fatalf("login() error = %v, want nil", err)
	}
	want := []string{"registry", "login", "registry.example.com", "--username", "user", "--password", "pass"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("helm invoked with args = %v, want %v", gotArgs, want)
	}
}
