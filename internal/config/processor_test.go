package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/librucha/krmgen/internal"
)

func TestProcessConfig_NoHelmNoKustomization(t *testing.T) {
	got, err := ProcessConfig(&types.Config{Kind: "KrmGen"}, t.TempDir())
	if err != nil {
		t.Fatalf("ProcessConfig() error = %v", err)
	}
	if got != "" {
		t.Errorf("ProcessConfig() = %q, want empty output", got)
	}
}

func TestProcessConfig_KustomizationWithoutHelm(t *testing.T) {
	dir := t.TempDir()
	kustomization := filepath.Join(dir, "kustomization.yaml")
	content := "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - cm.yaml\n"
	if err := os.WriteFile(kustomization, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cm := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n"
	if err := os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(cm), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := ProcessConfig(&types.Config{Kind: "KrmGen"}, dir)
	if err != nil {
		t.Fatalf("ProcessConfig() error = %v", err)
	}
	if !strings.Contains(got, "kind: ConfigMap") {
		t.Errorf("ProcessConfig() = %q, want the kustomize output", got)
	}
}
