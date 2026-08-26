package kustomize

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestKubectlBuilder_InvokesTheBinaryWithTheDirectory(t *testing.T) {
	var gotName string
	var gotArgs []string
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		gotName, gotArgs = name, arg
		return "kind: ConfigMap\n", "", nil
	}

	got, err := newKubectlBuilder("kubectl").Build("/work")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got != "kind: ConfigMap\n" {
		t.Errorf("Build() = %q, want the command output", got)
	}
	if gotName != "kubectl" {
		t.Errorf("invoked %q, want %q", gotName, "kubectl")
	}
	if !reflect.DeepEqual(gotArgs, []string{"kustomize", "/work"}) {
		t.Errorf("args = %v, want [kustomize /work]", gotArgs)
	}
}

func TestKubectlBuilder_HonoursAnExplicitExecutable(t *testing.T) {
	var gotName string
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		gotName = name
		return "", "", nil
	}

	if _, err := newKubectlBuilder("/opt/bin/kubectl").Build("/work"); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if gotName != "/opt/bin/kubectl" {
		t.Errorf("invoked %q, want the configured path", gotName)
	}
}

func TestKubectlBuilder_CarriesStderrIntoTheError(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		return "", "unable to find one of 'kustomization.yaml'", errors.New("exit status 1")
	}

	_, err := newKubectlBuilder("kubectl").Build("/work")
	if err == nil {
		t.Fatal("Build() error = nil, want the failure to propagate")
	}
	if !strings.Contains(err.Error(), "unable to find one of 'kustomization.yaml'") {
		t.Errorf("error = %v, want it to carry the tool's stderr", err)
	}
}
