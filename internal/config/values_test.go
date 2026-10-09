package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolveValues(t *testing.T) {
	t.Setenv("ARGOCD_ENV_CLUSTER_PROFILE", "prod")
	tests := []struct {
		name  string
		files map[string]string
		want  map[string]any
	}{
		{
			name: "chaining in document order",
			files: map[string]string{"krmgen.yaml": `kind: KrmGen
values:
  clusterProfile: '{{ argocdEnv "CLUSTER_PROFILE" }}'
  keyvault: '{{ printf "rixocz-%s-aks-vault" .Values.clusterProfile }}'
`},
			want: map[string]any{"clusterProfile": "prod", "keyvault": "rixocz-prod-aks-vault"},
		},
		{
			name: "non-string scalars stay typed, templated leaf is a string",
			files: map[string]string{"krmgen.yaml": `kind: KrmGen
values:
  replicas: 2
  ratio: 0.5
  enabled: true
  nothing: null
  rendered: '{{ 3 }}'
`},
			want: map[string]any{"replicas": 2, "ratio": 0.5, "enabled": true, "nothing": nil, "rendered": "3"},
		},
		{
			name: "nested sibling reference",
			files: map[string]string{"krmgen.yaml": `kind: KrmGen
values:
  db:
    host: db.internal
    url: '{{ printf "postgres://%s:5432" .Values.db.host }}'
`},
			want: map[string]any{"db": map[string]any{"host": "db.internal", "url": "postgres://db.internal:5432"}},
		},
		{
			name: "lists are walked",
			files: map[string]string{"krmgen.yaml": `kind: KrmGen
values:
  profile: prod
  zones: [a, '{{ .Values.profile }}-b', {name: '{{ .Values.profile }}'}]
`},
			want: map[string]any{"profile": "prod", "zones": []any{"a", "prod-b", map[string]any{"name": "prod"}}},
		},
		{
			name: "cross-file reference follows file name order",
			files: map[string]string{
				"a.yaml": "kind: KrmGen\nvalues:\n  base: kv\n",
				"b.yaml": "kind: KrmGen\nvalues:\n  full: '{{ .Values.base }}-prod'\n",
			},
			want: map[string]any{"base": "kv", "full": "kv-prod"},
		},
		{
			name: "non-KrmGen file is ignored",
			files: map[string]string{
				"krmgen.yaml": "kind: KrmGen\n",
				"values.yaml": "values:\n  x: '{{ .Values.missing }}'\n",
			},
			want: map[string]any{},
		},
		{
			name: "unparseable file is skipped",
			files: map[string]string{
				"krmgen.yaml": "kind: KrmGen\nvalues:\n  x: 'unterminated\n",
			},
			want: map[string]any{},
		},
		{
			name:  "no values anywhere",
			files: map[string]string{"krmgen.yaml": "kind: KrmGen\n"},
			want:  map[string]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveValues(writeFiles(t, tt.files))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveValues_UnparseableFixtureReallyFails(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("kind: KrmGen\nvalues:\n  x: 'unterminated\n"), &doc); err == nil {
		t.Fatal("fixture must be invalid YAML")
	}
}

func TestResolveValues_Errors(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr []string
	}{
		{
			name:    "forward reference in one file fails",
			files:   map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues:\n  a: '{{ .Values.b }}'\n  b: x\n"},
			wantErr: []string{"evaluating value values.a in krmgen.yaml failed", `map has no entry for key "b"`},
		},
		{
			name: "forward reference across files fails",
			files: map[string]string{
				"a.yaml": "kind: KrmGen\nvalues:\n  full: '{{ .Values.base }}-prod'\n",
				"b.yaml": "kind: KrmGen\nvalues:\n  base: kv\n",
			},
			wantErr: []string{"evaluating value values.full in a.yaml failed"},
		},
		{
			name: "duplicate key across files",
			files: map[string]string{
				"a.yaml":      "kind: KrmGen\nvalues:\n  keyvault: one\n",
				"krmgen.yaml": "kind: KrmGen\nvalues:\n  keyvault: two\n",
			},
			wantErr: []string{`value "keyvault" defined in both a.yaml and krmgen.yaml`},
		},
		{
			name:    "values not a mapping",
			files:   map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues: [a, b]\n"},
			wantErr: []string{"values in krmgen.yaml must be a mapping"},
		},
		{
			name:    "unquoted template is rejected",
			files:   map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues:\n  x: {{ argocdEnv \"CLUSTER_PROFILE\" }}\n"},
			wantErr: []string{"values in krmgen.yaml: values.x has a non-string key - quote templated values"},
		},
		{
			name:    "nested path is named",
			files:   map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues:\n  db:\n    url: '{{ .Values.nope }}'\n"},
			wantErr: []string{"evaluating value values.db.url in krmgen.yaml failed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveValues(writeFiles(t, tt.files))
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

func TestResolveValues_ErrorDoesNotLeakResolvedValues(t *testing.T) {
	dir := writeFiles(t, map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues:\n  secret: s3cr3t-value\n  broken: '{{ .Values.secret }}{{ .Values.nope }}'\n"})
	_, err := ResolveValues(dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "s3cr3t-value") {
		t.Errorf("error leaks a resolved value: %q", err)
	}
}

func TestResolveValues_FollowsSymlinkedConfig(t *testing.T) {
	target := writeFiles(t, map[string]string{"real.yaml": "kind: KrmGen\nvalues:\n  keyvault: kv\n"})
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(target, "real.yaml"), filepath.Join(dir, "krmgen.yaml")); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveValues(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"keyvault": "kv"}; !reflect.DeepEqual(got, want) {
		t.Errorf("values = %v, want %v", got, want)
	}
}

func TestResolveValues_DuplicateAndMergeKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "nested duplicate",
			content: "kind: KrmGen\nvalues:\n  db:\n    host: a\n    host: b\n",
			wantErr: "value values.db.host defined twice in krmgen.yaml",
		},
		{
			name:    "top-level duplicate in one file",
			content: "kind: KrmGen\nvalues:\n  keyvault: one\n  keyvault: two\n",
			wantErr: "value values.keyvault defined twice in krmgen.yaml",
		},
		{
			name:    "merge key",
			content: "kind: KrmGen\nvalues:\n  base: &base\n    host: a\n  db:\n    <<: *base\n    port: 1\n",
			wantErr: "values in krmgen.yaml: values.db uses a YAML merge key, which is not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveValues(writeFiles(t, map[string]string{"krmgen.yaml": tt.content}))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestStripValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "values in the middle",
			content: "kind: KrmGen\nvalues:\n  a: '{{ x }}'\n  b:\n    c: d\n# trailing comment\nskip:\n  - '*.pfx'\n",
			want:    "kind: KrmGen\n\n\n\n\n\nskip:\n  - '*.pfx'\n",
		},
		{
			// Blanking lines would also wipe kind: KrmGen sharing the line,
			// turning the config into nothing - so flow style is left as is.
			name:    "flow-style root is untouched",
			content: "{kind: KrmGen, values: {pw: \"x\"}}\n",
			want:    "{kind: KrmGen, values: {pw: \"x\"}}\n",
		},
		{
			name:    "multi-line flow-style root is untouched",
			content: "{kind: KrmGen,\n values: {pw: \"x\"}\n}\n",
			want:    "{kind: KrmGen,\n values: {pw: \"x\"}\n}\n",
		},
		{
			name:    "values last",
			content: "kind: KrmGen\nskip: []\nvalues:\n  a: b\n  c: d\n",
			want:    "kind: KrmGen\nskip: []\n\n\n\n",
		},
		{
			name:    "flow-style values",
			content: "kind: KrmGen\nvalues: {a: b}\nhelm: {}\n",
			want:    "kind: KrmGen\n\nhelm: {}\n",
		},
		{
			name:    "no values",
			content: "kind: KrmGen\nskip: []\n",
			want:    "kind: KrmGen\nskip: []\n",
		},
		{
			name:    "non-KrmGen untouched",
			content: "kind: ConfigMap\nvalues:\n  a: b\n",
			want:    "kind: ConfigMap\nvalues:\n  a: b\n",
		},
		{
			name:    "unparseable untouched",
			content: "kind: KrmGen\nvalues:\n  x: 'unterminated\n",
			want:    "kind: KrmGen\nvalues:\n  x: 'unterminated\n",
		},
		{
			name:    "values last before next document",
			content: "kind: KrmGen\nvalues:\n  a: b\n---\nkind: Other\n",
			want:    "kind: KrmGen\n\n\n---\nkind: Other\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(StripValues([]byte(tt.content))); got != tt.want {
				t.Errorf("StripValues() = %q, want %q", got, tt.want)
			}
		})
	}
}
