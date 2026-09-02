package kustomize

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	cons "github.com/librucha/krmgen/internal/utils"
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

func TestKrustyBuilder_RendersADirectory(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("kustomization.yaml", "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nnamespace: ns\nresources:\n  - cm.yaml\n")
	write("cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n")

	got, err := newKrustyBuilder().Build(dir)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(got, "namespace: ns") {
		t.Errorf("Build() = %q, want the namespace transformer applied", got)
	}
}

// The library defaults to no reordering; kubectl applies the legacy order.
// Leaving that unset would reorder every rendered document and move every
// golden file, which is easy to mistake for an unavoidable version
// difference. It is not - it is one option.
func TestKrustyBuilder_UsesLegacyResourceOrdering(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Deliberately listed service-first; legacy ordering emits the CRD first.
	write("kustomization.yaml", "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - res.yaml\n")
	write("res.yaml", "apiVersion: v1\nkind: Service\nmetadata:\n  name: svc\nspec:\n  ports:\n    - port: 80\n---\napiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: demos.krmgen.test\nspec:\n  group: krmgen.test\n  scope: Namespaced\n  names:\n    plural: demos\n    singular: demo\n    kind: Demo\n  versions:\n    - name: v1\n      served: true\n      storage: true\n      schema:\n        openAPIV3Schema:\n          type: object\n")

	got, err := newKrustyBuilder().Build(dir)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	crd := strings.Index(got, "kind: CustomResourceDefinition")
	svc := strings.Index(got, "kind: Service")
	if crd < 0 || svc < 0 {
		t.Fatalf("Build() = %q, want both documents", got)
	}
	if crd > svc {
		t.Error("the CustomResourceDefinition must come first - legacy ordering is not in effect")
	}
}

func TestKrustyBuilder_ReportsAMissingKustomization(t *testing.T) {
	_, err := newKrustyBuilder().Build(t.TempDir())
	if err == nil {
		t.Fatal("Build() error = nil, want an error for a directory with no kustomization")
	}
	if !strings.Contains(err.Error(), "unable to find one of 'kustomization.yaml'") {
		t.Errorf("error = %v, want the same wording kubectl reports", err)
	}
}

func TestSelectBuilder_EmbeddedByDefault(t *testing.T) {
	t.Setenv(cons.EnvKubectlExecutable, "")
	if err := os.Unsetenv(cons.EnvKubectlExecutable); err != nil {
		t.Fatalf("os.Unsetenv() error = %v", err)
	}

	if got := selectBuilder().Name(); got != "embedded kustomize" {
		t.Errorf("selectBuilder() = %q, want the embedded backend when no executable is configured", got)
	}
}

func TestSelectBuilder_ExternalWhenConfigured(t *testing.T) {
	t.Setenv(cons.EnvKubectlExecutable, "/opt/bin/kubectl")

	got := selectBuilder().Name()
	if !strings.Contains(got, "/opt/bin/kubectl") {
		t.Errorf("selectBuilder() = %q, want the configured binary", got)
	}
}

func TestSelectBuilder_EmptyValueMeansEmbedded(t *testing.T) {
	// An exported-but-empty variable is a configuration accident, not a
	// request for the external tool.
	t.Setenv(cons.EnvKubectlExecutable, "")

	if got := selectBuilder().Name(); got != "embedded kustomize" {
		t.Errorf("selectBuilder() = %q, want the embedded backend for an empty value", got)
	}
}

// The reported secret disclosure: ResMap.AsYaml reports a resource it cannot
// serialize by prefixing the reason with a %#v dump of that resource's whole
// map. Wrapping it propagated every value of a generated Secret into the
// error. The key below is long enough that kyaml folds the line, which is
// what makes the rendered resource unserializable in the first place.
func TestKrustyBuilder_DoesNotLeakRenderedValuesOnASerializationFailure(t *testing.T) {
	const marker = "LEAKED-SECRET-MARKER"
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("kustomization.yaml", "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\ngeneratorOptions:\n  disableNameSuffixHash: true\nsecretGenerator:\n  - name: demo\n    behavior: create\n    envs:\n      - secrets.properties\n")
	write("secrets.properties", marker+strings.Repeat("A", 1400)+"=x\n")

	_, err := newKrustyBuilder().Build(dir)
	if err == nil {
		t.Fatal("Build() error = nil, want the serialization failure to surface")
	}
	if strings.Contains(err.Error(), marker) {
		t.Errorf("error leaked the rendered secret value: %v", err)
	}
	if !strings.Contains(err.Error(), "Secret demo") {
		t.Errorf("error = %v, want it to name the resource that could not be serialized", err)
	}
}

func TestKubectlBuilder_RedactsTheRenderDumpFromStderr(t *testing.T) {
	const marker = "LEAKED-SECRET-MARKER"
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		dump := `error: map[string]interface {}{"apiVersion":"v1", "data":map[string]interface {}{"creds":"` +
			marker + `"}}: yaml: did not find expected ',' or '}'`
		return "", dump, errors.New("exit status 1")
	}

	_, err := newKubectlBuilder("kubectl").Build("/work")
	if err == nil {
		t.Fatal("Build() error = nil, want the failure to propagate")
	}
	if strings.Contains(err.Error(), marker) {
		t.Errorf("error leaked the rendered secret value: %v", err)
	}
	if !strings.Contains(err.Error(), "redacted") {
		t.Errorf("error = %v, want it to say the tool output was withheld", err)
	}
}
