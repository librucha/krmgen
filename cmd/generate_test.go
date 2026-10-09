package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/librucha/krmgen/v2/internal/config"
)

// TestCopiedFilesAreNotWorldReadable covers every way krmgen puts something
// into its working directory: a template-evaluated file, a file copied as-is
// because it matches a skip pattern, and the directories it creates on the
// way. All three can carry or expose rendered secrets, and each is written by
// a different call, so one assertion would not cover the others.
func TestCopiedFilesAreNotWorldReadable(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "krmgen.yaml"), []byte("kind: KrmGen\n"), 0600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(src, "certs", "prod")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	// Deliberately not valid Go template syntax: it must reach the working
	// directory untouched, through the skip branch rather than the eval one.
	if err := os.WriteFile(filepath.Join(nested, "client.pfx"), []byte("{{ not a template"), 0644); err != nil {
		t.Fatal(err)
	}

	workDir, err := copySrcDir(src, []string{"*.pfx"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	cases := []struct {
		what string
		path string
		want os.FileMode
	}{
		{what: "template-evaluated file", path: "krmgen.yaml", want: 0600},
		{what: "skipped file copied as-is", path: filepath.Join("certs", "prod", "client.pfx"), want: 0600},
		{what: "created directory", path: "certs", want: 0700},
		{what: "created nested directory", path: filepath.Join("certs", "prod"), want: 0700},
	}
	for _, tc := range cases {
		info, err := os.Stat(filepath.Join(workDir, tc.path))
		if err != nil {
			t.Errorf("%s: %v", tc.what, err)
			continue
		}
		if perm := info.Mode().Perm(); perm != tc.want {
			t.Errorf("%s (%s) mode = %04o, want %04o - it may hold rendered secrets", tc.what, tc.path, perm, tc.want)
		}
	}
}

func TestMatchesSkipPattern(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		{
			name:     "no patterns",
			relPath:  "cert.pfx",
			patterns: nil,
			want:     false,
		},
		{
			name:     "extension wildcard matches flat file",
			relPath:  "cert.pfx",
			patterns: []string{"*.pfx"},
			want:     true,
		},
		{
			name:     "extension wildcard matches nested file by basename",
			relPath:  "certs/prod/cert.pfx",
			patterns: []string{"*.pfx"},
			want:     true,
		},
		{
			name:     "extension wildcard does not match different extension",
			relPath:  "cert.pem",
			patterns: []string{"*.pfx"},
			want:     false,
		},
		{
			name:     "directory-scoped pattern matches file in that directory",
			relPath:  "assets/logo.png",
			patterns: []string{"assets/*.png"},
			want:     true,
		},
		{
			name:     "directory-scoped pattern does not match file in other directory",
			relPath:  "static/logo.png",
			patterns: []string{"assets/*.png"},
			want:     false,
		},
		{
			name:     "exact filename match",
			relPath:  "secret.yaml",
			patterns: []string{"secret.yaml"},
			want:     true,
		},
		{
			name:     "exact filename does not match different file",
			relPath:  "config.yaml",
			patterns: []string{"secret.yaml"},
			want:     false,
		},
		{
			name:     "first matching pattern wins",
			relPath:  "cert.pfx",
			patterns: []string{"*.pem", "*.pfx", "*.crt"},
			want:     true,
		},
		{
			name:     "multiple patterns, none match",
			relPath:  "values.yaml",
			patterns: []string{"*.pfx", "*.png", "*.pem"},
			want:     false,
		},
		{
			name:     "question mark wildcard matches single char",
			relPath:  "cert.p12",
			patterns: []string{"cert.p??"},
			want:     true,
		},
		{
			name:     "empty relPath with matching pattern",
			relPath:  "file.bin",
			patterns: []string{"*.bin"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSkipPattern(tt.relPath, tt.patterns)
			if got != tt.want {
				t.Errorf("matchesSkipPattern(%q, %v) = %v, want %v", tt.relPath, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestMergeSkipPatterns(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{
			name: "both empty",
			a:    nil,
			b:    nil,
			want: nil,
		},
		{
			name: "only a",
			a:    []string{"*.pfx"},
			b:    nil,
			want: []string{"*.pfx"},
		},
		{
			name: "only b",
			a:    nil,
			b:    []string{"*.png"},
			want: []string{"*.png"},
		},
		{
			name: "no duplicates",
			a:    []string{"*.pfx", "*.png"},
			b:    []string{"*.pem", "*.bin"},
			want: []string{"*.pfx", "*.png", "*.pem", "*.bin"},
		},
		{
			name: "deduplicates across a and b",
			a:    []string{"*.pfx", "*.png"},
			b:    []string{"*.png", "*.pem"},
			want: []string{"*.pfx", "*.png", "*.pem"},
		},
		{
			name: "deduplicates within a",
			a:    []string{"*.pfx", "*.pfx"},
			b:    nil,
			want: []string{"*.pfx"},
		},
		{
			name: "config patterns come before cli patterns",
			a:    []string{"*.pfx"},
			b:    []string{"*.png"},
			want: []string{"*.pfx", "*.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeSkipPatterns(tt.a, tt.b)
			if len(got) != len(tt.want) {
				t.Fatalf("mergeSkipPatterns(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("mergeSkipPatterns result[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCopySrcDir_EvaluatesTemplatesExceptSkipped(t *testing.T) {
	src := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("plain.yaml", `value: '{{ kubeEnv "TESTVAR" "fallback" }}'`)
	write("certs/keep.pfx", `raw: {{ this is not a template }}`)

	workDir, err := copySrcDir(src, []string{"*.pfx"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	evaluated, err := os.ReadFile(filepath.Join(workDir, "plain.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(evaluated), "fallback") {
		t.Errorf("template was not evaluated: %q", evaluated)
	}

	skipped, err := os.ReadFile(filepath.Join(workDir, "certs", "keep.pfx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skipped), "{{ this is not a template }}") {
		t.Errorf("skipped file was altered: %q", skipped)
	}
}

func TestCopySrcDir_WorkDirIsPrivate(t *testing.T) {
	workDir, err := copySrcDir(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	info, err := os.Stat(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("work dir mode = %o, want 0700 - it holds rendered secrets", perm)
	}
}

func TestCopyDir_BrokenTemplateReturnsError(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "bad.yaml"), []byte("{{ .Unclosed "), 0600); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()

	err := copyDir(src, dst, src, nil, nil)
	if err == nil {
		t.Fatal("expected a broken template to return an error")
	}
	if !strings.Contains(err.Error(), "template evaluation") {
		t.Errorf("error = %q, want it to name template evaluation", err)
	}
}

// TestGenerate_ReportsWorkingDirCleanupFailureButStillSucceeds closes the gap
// phase 5 left behind: the warning path (generate, cmd/generate.go) was
// exercised in practice only by a read-only mount, which the test suite
// cannot rely on. removeAll is a seam over os.RemoveAll for exactly this -
// it lets the test force the failure deterministically instead.
func TestGenerate_ReportsWorkingDirCleanupFailureButStillSucceeds(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "krmgen.yaml"), []byte("kind: KrmGen\n"), 0600); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("simulated: read-only mount")
	original := removeAll
	removeAll = func(string) error { return wantErr }
	t.Cleanup(func() { removeAll = original })

	stderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	genErr := generate(src, nil)

	_ = w.Close()
	os.Stderr = stderr

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	if genErr != nil {
		t.Fatalf("a working-dir cleanup failure must not turn a successful run into a failing one, got: %v", genErr)
	}
	if !strings.Contains(buf.String(), "warning: could not remove working dir") {
		t.Errorf("stderr = %q, want it to contain the documented warning", buf.String())
	}
	if !strings.Contains(buf.String(), wantErr.Error()) {
		t.Errorf("stderr = %q, want it to name the underlying cleanup error %q", buf.String(), wantErr)
	}
}

func TestProcessWorkDir_PrintsOnlyKrmGenFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "not-krmgen.yaml"), []byte("kind: Other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "krmgen.yaml"), []byte("kind: KrmGen\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	processErr := processWorkDir(dir)
	_ = w.Close()
	os.Stdout = stdout
	if processErr != nil {
		t.Fatal(processErr)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	// A KrmGen file with neither charts nor a kustomization renders nothing,
	// so exactly one empty line is printed for it.
	if got := buf.String(); got != "\n" {
		t.Errorf("stdout = %q, want a single empty line", got)
	}
}

func TestGenerate_ValuesReachEveryTemplatedFile(t *testing.T) {
	t.Setenv("ARGOCD_ENV_CLUSTER_PROFILE", "prod")
	src, err := filepath.Abs("../test/resources/values")
	if err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	genErr := generate(src, mergeSkipPatterns(config.ReadSkipPatterns(src), nil))
	_ = w.Close()
	os.Stdout = stdout
	if genErr != nil {
		t.Fatal(genErr)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"namespace: prod",
		"keyvault: rixocz-prod-aks-vault",
		`replicas: "2"`,
		"dbUrl: postgres://db.prod.internal:5432",
		"[a@prod][b@prod]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func TestCopySrcDir_SkippedFileGetsNoValues(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "raw.txt"), []byte("{{ .Values.keyvault }}"), 0600); err != nil {
		t.Fatal(err)
	}
	workDir, err := copySrcDir(src, []string{"*.txt"}, map[string]any{"Values": map[string]any{"keyvault": "kv"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })
	got, err := os.ReadFile(filepath.Join(workDir, "raw.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{{ .Values.keyvault }}" {
		t.Errorf("skipped file was evaluated: %q", got)
	}
}

func TestCopyDir_MissingValueErrorHasQuotingHint(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "secret.yaml"), []byte("x: {{ .Values.keyvault }}"), 0600); err != nil {
		t.Fatal(err)
	}
	err := copyDir(src, t.TempDir(), src, nil, map[string]any{"Values": map[string]any{}})
	if err == nil {
		t.Fatal("expected a missing value to fail")
	}
	for _, want := range []string{"template evaluation of file", `map has no entry for key "keyvault"`, "key not defined in values:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestGenerate_ValueErrorFailsTheRun(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "krmgen.yaml"), []byte("kind: KrmGen\nvalues:\n  a: '{{ .Values.b }}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := generate(src, nil)
	if err == nil || !strings.Contains(err.Error(), "evaluating value values.a in krmgen.yaml failed") {
		t.Errorf("err = %v, want the value error", err)
	}
}

func TestGenerate_ValuesAreEvaluatedOnce(t *testing.T) {
	t.Setenv("ARGOCD_ENV_PW", "it's")
	src := t.TempDir()
	files := map[string]string{
		"krmgen.yaml":        "kind: KrmGen\nvalues:\n  pw: '{{ argocdEnv \"PW\" }}'\n",
		"kustomization.yaml": "resources:\n  - cm.yaml\n",
		"cm.yaml":            "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\ndata:\n  pw: {{ .Values.pw | quote }}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	genErr := generate(src, nil)
	_ = w.Close()
	os.Stdout = stdout
	if genErr != nil {
		t.Fatal(genErr)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); !strings.Contains(out, "pw: it's") {
		t.Errorf("output does not contain the value:\n%s", out)
	}
}

func TestProcessWorkDir_BrokenKrmGenFileFails(t *testing.T) {
	for name, content := range map[string]string{
		"block style": "kind: KrmGen\nfoo: 'it's'\n",
		"flow style":  "{kind: KrmGen, values: {pw: 'it's'}}\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "krmgen.yaml"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			err := processWorkDir(dir)
			if err == nil || !strings.Contains(err.Error(), "config file krmgen.yaml is not valid YAML after templating: ") {
				t.Errorf("err = %v, want the invalid YAML error", err)
			}
		})
	}
}
