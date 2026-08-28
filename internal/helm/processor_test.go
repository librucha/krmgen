package helm

import (
	"errors"
	"fmt"
	types "github.com/librucha/krmgen/internal"
	cons "github.com/librucha/krmgen/internal/utils"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func Test_helmExecutable(t *testing.T) {
	helmExec, lookErr := exec.LookPath("helm")
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
			if tt.name == "fallback to default" && lookErr != nil {
				// helm not on PATH in this environment - skip rather than
				// exercise the error path here (covered by
				// TestHelmExecutableMissing below).
				t.Skip("helm not found on PATH")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := helmExecutable()
			if err != nil {
				t.Fatalf("helmExecutable() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("helmExecutable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHelmExecutableMissing(t *testing.T) {
	t.Setenv(cons.EnvHelmExecutable, "")
	t.Setenv("PATH", t.TempDir()) // no helm anywhere
	if _, err := helmExecutable(); err == nil {
		t.Error("want an error when helm is not on PATH")
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

func TestGetValuesArgs(t *testing.T) {
	workDir := t.TempDir()

	t.Run("no values", func(t *testing.T) {
		args, err := getValuesArgs(&types.HelmChart{}, workDir)
		if err != nil {
			t.Fatalf("getValuesArgs() error = %v", err)
		}
		if len(args) != 0 {
			t.Errorf("args = %v, want none", args)
		}
	})

	t.Run("values file is joined with the work dir", func(t *testing.T) {
		args, err := getValuesArgs(&types.HelmChart{ValuesFile: "values.yaml"}, workDir)
		if err != nil {
			t.Fatalf("getValuesArgs() error = %v", err)
		}
		want := []string{"--values", filepath.Join(workDir, "values.yaml")}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("args = %v, want %v", args, want)
		}
	})

	t.Run("inline values are written to a file", func(t *testing.T) {
		chart := &types.HelmChart{
			ReleaseName:  "rel",
			ValuesInline: map[string]any{"replicaCount": 2},
		}
		args, err := getValuesArgs(chart, workDir)
		if err != nil {
			t.Fatalf("getValuesArgs() error = %v", err)
		}
		if len(args) != 2 || args[0] != "--values" {
			t.Fatalf("args = %v, want a --values pair", args)
		}
		content, err := os.ReadFile(args[1])
		if err != nil {
			t.Fatalf("reading the generated values file: %v", err)
		}
		if !strings.Contains(string(content), "replicaCount: 2") {
			t.Errorf("generated values = %q, want it to contain replicaCount: 2", content)
		}
	})

	t.Run("both sources produce two --values in order", func(t *testing.T) {
		chart := &types.HelmChart{
			ReleaseName:  "rel",
			ValuesFile:   "values.yaml",
			ValuesInline: map[string]any{"a": 1},
		}
		args, err := getValuesArgs(chart, workDir)
		if err != nil {
			t.Fatalf("getValuesArgs() error = %v", err)
		}
		if len(args) != 4 || args[0] != "--values" || args[2] != "--values" {
			t.Fatalf("args = %v, want two --values pairs", args)
		}
		if args[1] != filepath.Join(workDir, "values.yaml") {
			t.Errorf("the values file must come first, got %v", args)
		}
	})
}

func TestTemplateHelmCharts_InvocationPerBackend(t *testing.T) {
	// These tests mock runCommand and assert on the exact argv the binary
	// renderer builds, so they must force selectRenderer to pick the binary
	// renderer regardless of the library default (internal/helm/renderer.go).
	t.Setenv(cons.EnvHelmExecutable, "/mock/helm")

	tests := []struct {
		name  string
		chart types.HelmChart
		want  []string
	}{
		{
			name: "oci backend passes the repo url positionally",
			chart: types.HelmChart{
				Name: "app", RepoUrl: "oci://reg.example.com/helm/app",
				ReleaseName: "rel", Version: "1.2.3", Namespace: "ns",
				IgnoreCredentials: true,
			},
			want: []string{"template", "rel", "--include-crds", "--version", "1.2.3", "--namespace", "ns", "oci://reg.example.com/helm/app"},
		},
		{
			name: "http backend passes --repo and the chart name",
			chart: types.HelmChart{
				Name: "app", RepoUrl: "https://charts.example.com",
				ReleaseName: "rel", Version: "1.2.3", Namespace: "ns",
				IgnoreCredentials: true,
			},
			want: []string{"template", "rel", "--include-crds", "--version", "1.2.3", "--namespace", "ns", "--repo", "https://charts.example.com", "--release-name", "app"},
		},
		{
			name: "version and namespace are omitted when unset",
			chart: types.HelmChart{
				Name: "app", RepoUrl: "oci://reg.example.com/helm/app",
				ReleaseName: "rel", IgnoreCredentials: true,
			},
			want: []string{"template", "rel", "--include-crds", "oci://reg.example.com/helm/app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs []string
			original := runCommand
			t.Cleanup(func() { runCommand = original })
			runCommand = func(name string, arg ...string) (string, string, error) {
				gotArgs = arg
				return "---\nkind: ConfigMap\n", "", nil
			}

			charts := []types.HelmChart{tt.chart}
			out, err := TemplateHelmCharts(&types.Helm{Charts: &charts}, t.TempDir())
			if err != nil {
				t.Fatalf("TemplateHelmCharts() error = %v", err)
			}
			if out != "---\nkind: ConfigMap\n" {
				t.Errorf("output = %q, want the helm output unchanged", out)
			}
			if !reflect.DeepEqual(gotArgs, tt.want) {
				t.Errorf("args =\n  %v\nwant\n  %v", gotArgs, tt.want)
			}
		})
	}
}

func TestTemplateHelmCharts_ConcatenatesInDeclarationOrder(t *testing.T) {
	// Force the binary renderer - see comment in
	// TestTemplateHelmCharts_InvocationPerBackend.
	t.Setenv(cons.EnvHelmExecutable, "/mock/helm")

	calls := 0
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		calls++
		return fmt.Sprintf("---\nkind: Chart%d\n", calls), "", nil
	}

	charts := []types.HelmChart{
		{Name: "a", RepoUrl: "oci://reg.example.com/a", ReleaseName: "a", IgnoreCredentials: true},
		{Name: "b", RepoUrl: "oci://reg.example.com/b", ReleaseName: "b", IgnoreCredentials: true},
	}
	out, err := TemplateHelmCharts(&types.Helm{Charts: &charts}, t.TempDir())
	if err != nil {
		t.Fatalf("TemplateHelmCharts() error = %v", err)
	}
	want := "---\nkind: Chart1\n---\nkind: Chart2\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestTemplateHelmCharts_PropagatesFailure(t *testing.T) {
	// Force the binary renderer - see comment in
	// TestTemplateHelmCharts_InvocationPerBackend.
	t.Setenv(cons.EnvHelmExecutable, "/mock/helm")

	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		return "", "chart not found", errors.New("exit status 1")
	}

	charts := []types.HelmChart{{Name: "a", RepoUrl: "oci://reg.example.com/a", ReleaseName: "a", IgnoreCredentials: true}}
	_, err := TemplateHelmCharts(&types.Helm{Charts: &charts}, t.TempDir())
	if err == nil {
		t.Fatal("TemplateHelmCharts() error = nil, want the helm failure to propagate")
	}
	if !strings.Contains(err.Error(), "chart not found") {
		t.Errorf("error = %v, want it to carry helm's stderr", err)
	}
}

func TestTemplateHelmCharts_StripsBannerBeforeConcatenating(t *testing.T) {
	// Force the binary renderer - see comment in
	// TestTemplateHelmCharts_InvocationPerBackend.
	t.Setenv(cons.EnvHelmExecutable, "/mock/helm")

	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		return "Pulled: reg/app:1\nDigest: sha256:abc\n---\nkind: ConfigMap\n", "", nil
	}

	charts := []types.HelmChart{
		{Name: "a", RepoUrl: "oci://reg.example.com/a", ReleaseName: "a", IgnoreCredentials: true},
		{Name: "b", RepoUrl: "oci://reg.example.com/b", ReleaseName: "b", IgnoreCredentials: true},
	}
	out, err := TemplateHelmCharts(&types.Helm{Charts: &charts}, t.TempDir())
	if err != nil {
		t.Fatalf("TemplateHelmCharts() error = %v", err)
	}
	if strings.Contains(out, "Pulled:") || strings.Contains(out, "Digest:") {
		t.Errorf("banner survived concatenation: %q", out)
	}
}

func TestTemplateHelm_LoginFailurePropagatesAndSkipsTemplate(t *testing.T) {
	// Force the binary renderer - see comment in
	// TestTemplateHelmCharts_InvocationPerBackend.
	t.Setenv(cons.EnvHelmExecutable, "/mock/helm")

	templateInvoked := false
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		if len(arg) > 0 && arg[0] == "template" {
			templateInvoked = true
			return "---\nkind: ConfigMap\n", "", nil
		}
		return "", "unauthorized", errors.New("exit status 1")
	}

	g := newOciHelmGenerator(&types.HelmChart{
		RepoUrl:     "oci://registry.example.com/helm",
		Name:        "app",
		ReleaseName: "rel",
		Username:    "user",
		Password:    "pass",
	})
	_, err := templateHelm(g, t.TempDir())
	if err == nil {
		t.Fatal("templateHelm() error = nil, want the login failure to propagate")
	}
	if !strings.Contains(err.Error(), "login to helm registry") {
		t.Errorf("error = %v, want it to carry login's error", err)
	}
	if templateInvoked {
		t.Error("templateHelm() ran `helm template` despite login failing, want it skipped")
	}
}
