package kustomize

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestUnwrapResources(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		want    []string
		wantErr bool
	}{
		{name: "empty list", in: []any{}, want: []string{}},
		{name: "string items", in: []any{"a.yaml", "b.yaml"}, want: []string{"a.yaml", "b.yaml"}},
		{name: "not a list", in: "a.yaml", wantErr: true},
		{name: "non-string item", in: []any{"a.yaml", 42}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unwrapResources(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unwrapResources() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unwrapResources() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindKustomizeFile(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantBase string
	}{
		{name: "no kustomization", files: []string{"a.yaml"}, wantBase: ""},
		{name: "yaml at top level", files: []string{"kustomization.yaml"}, wantBase: "kustomization.yaml"},
		{name: "yml variant", files: []string{"kustomization.yml"}, wantBase: "kustomization.yml"},
		{name: "extensionless variant", files: []string{"kustomization"}, wantBase: "kustomization"},
		{name: "case insensitive", files: []string{"Kustomization.YAML"}, wantBase: "Kustomization.YAML"},
		{name: "found in a subdirectory", files: []string{"nested/kustomization.yaml"}, wantBase: "kustomization.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				path := filepath.Join(dir, f)
				if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("kind: Kustomization\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got := FindKustomizeFile(dir)
			if tt.wantBase == "" {
				if got != "" {
					t.Errorf("FindKustomizeFile() = %q, want empty", got)
				}
				return
			}
			if filepath.Base(got) != tt.wantBase {
				t.Errorf("FindKustomizeFile() = %q, want basename %q", got, tt.wantBase)
			}
		})
	}
}

// captureFatal redirects logrus' exit so a log.Fatalf in production code
// aborts the call under test instead of the test binary.
func captureFatal(t *testing.T, call func()) (fatal bool) {
	t.Helper()
	original := log.StandardLogger().ExitFunc
	t.Cleanup(func() { log.StandardLogger().ExitFunc = original })
	log.StandardLogger().ExitFunc = func(int) { panic("log.Fatal") }
	defer func() {
		if r := recover(); r != nil {
			fatal = true
		}
	}()
	call()
	return false
}

func TestFindKustomizeFile_MultipleFilesIsFatal(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"kustomization.yaml", "nested/kustomization.yaml"} {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("kind: Kustomization\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !captureFatal(t, func() { FindKustomizeFile(dir) }) {
		t.Error("expected two kustomization files to be fatal, but the call returned")
	}
}

func TestBuildKustomize_AppendsResourcesAndInvokesKubectl(t *testing.T) {
	dir := t.TempDir()
	kustomizeFile := filepath.Join(dir, "kustomization.yaml")
	if err := os.WriteFile(kustomizeFile, []byte("kind: Kustomization\nresources:\n  - existing.yaml\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var gotName string
	var gotArgs []string
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		gotName, gotArgs = name, arg
		return "rendered: true\n", "", nil
	}

	got := BuildKustomize(kustomizeFile, dir, "kind: ConfigMap\n")

	if got != "rendered: true\n" {
		t.Errorf("BuildKustomize() = %q, want the kubectl output", got)
	}
	if gotName != "kubectl" {
		t.Errorf("invoked %q, want %q", gotName, "kubectl")
	}
	if !reflect.DeepEqual(gotArgs, []string{"kustomize", dir}) {
		t.Errorf("args = %v, want %v", gotArgs, []string{"kustomize", dir})
	}

	updated, err := os.ReadFile(kustomizeFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "existing.yaml") {
		t.Error("the pre-existing resource entry was dropped")
	}
	if !strings.Contains(string(updated), ".yml") {
		t.Error("the generated resources file was not appended to the kustomization")
	}
}
