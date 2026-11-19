package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeVarFile puts content into a fresh temp directory under name and
// returns the full path. The name matters: LoadVars decides by extension.
func writeVarFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadVars_File(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
		content  string
		want     map[string]any
	}{
		{
			name:     "yaml",
			fileName: "v.yaml",
			content:  "ns: black widow\nreplicas: 3\n",
			want:     map[string]any{"ns": "black widow", "replicas": 3},
		},
		{
			name:     "yml",
			fileName: "v.yml",
			content:  "db:\n  host: a\n  port: 5432\n",
			want:     map[string]any{"db": map[string]any{"host": "a", "port": 5432}},
		},
		{
			// 1000000 must stay an int: decoded as float64 it would print as
			// 1e+06 in a template.
			name:     "json",
			fileName: "v.json",
			content:  `{"n": 1000000, "tags": ["a", "b"], "on": true}`,
			want:     map[string]any{"n": 1000000, "tags": []any{"a", "b"}, "on": true},
		},
		{
			name:     "upper-case extension",
			fileName: "v.YAML",
			content:  "a: b\n",
			want:     map[string]any{"a": "b"},
		},
		{
			name:     "empty file",
			fileName: "v.yaml",
			content:  "",
			want:     map[string]any{},
		},
		{
			name:     "only a comment",
			fileName: "v.yaml",
			content:  "# nothing here\n",
			want:     map[string]any{},
		},
		{
			// A key is its text, at any depth, as under values: in a KrmGen
			// file. Decoded generically, ports would be a map[any]any, which
			// {{ index .Values.ports "80" }} cannot read and sprig's map
			// functions reject.
			name:     "non-string keys become their text",
			fileName: "v.yaml",
			content:  "80: top\nports:\n  80: http\n  true: yes-key\n  0x10: hex\n",
			want: map[string]any{
				"80":    "top",
				"ports": map[string]any{"80": "http", "true": "yes-key", "0x10": "hex"},
			},
		},
		{
			name:     "null value",
			fileName: "v.yaml",
			content:  "db:\nname: x\n",
			want:     map[string]any{"db": nil, "name": "x"},
		},
		{
			name:     "alias",
			fileName: "v.yaml",
			content:  "base: &b\n  host: a\ncopy: *b\n",
			want:     map[string]any{"base": map[string]any{"host": "a"}, "copy": map[string]any{"host": "a"}},
		},
		{
			name:     "list of mappings",
			fileName: "v.yaml",
			content:  "hosts:\n  - name: a\n    port: 1\n  - name: b\n",
			want: map[string]any{"hosts": []any{
				map[string]any{"name": "a", "port": 1},
				map[string]any{"name": "b"},
			}},
		},
		{
			name:     "first document only",
			fileName: "v.yaml",
			content:  "a: 1\n---\nb: 2\n",
			want:     map[string]any{"a": 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LoadVars(writeVarFile(t, tc.fileName, tc.content), nil)
			if err != nil {
				t.Fatalf("LoadVars() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("LoadVars() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestLoadVars_FileErrors(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
		content  string
		// wantIn lists substrings the error must carry besides the file path.
		wantIn []string
	}{
		{name: "unsupported extension", fileName: "v.txt", content: "a: b\n", wantIn: []string{".yaml", ".yml", ".json"}},
		{name: "no extension", fileName: "vars", content: "a: b\n"},
		{name: "top-level sequence", fileName: "v.yaml", content: "- a\n- b\n", wantIn: []string{"mapping"}},
		{name: "top-level scalar", fileName: "v.yaml", content: "just text\n", wantIn: []string{"mapping"}},
		{name: "invalid yaml", fileName: "v.yaml", content: "a: [1, 2\n"},
		// The last one would silently win.
		{name: "duplicate key", fileName: "v.yaml", content: "a: 1\na: 2\n", wantIn: []string{"a", "twice"}},
		{name: "nested duplicate key", fileName: "v.yaml", content: "db:\n  host: a\n  host: b\n", wantIn: []string{"db.host", "twice"}},
		// Rejected as under values: - the entries would not be merged.
		{name: "merge key", fileName: "v.yaml", content: "base: &b\n  host: a\ncopy:\n  <<: *b\n", wantIn: []string{"copy", "merge key"}},
		{name: "mapping used as a key", fileName: "v.yaml", content: "? [a, b]\n: x\n", wantIn: []string{"key"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeVarFile(t, tc.fileName, tc.content)
			got, err := LoadVars(path, nil)
			if err == nil {
				t.Fatalf("LoadVars() = %#v, want an error", got)
			}
			for _, want := range append([]string{path}, tc.wantIn...) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "absent.yaml")
		got, err := LoadVars(path, nil)
		if err == nil {
			t.Fatalf("LoadVars() = %#v, want an error", got)
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error %q does not mention %q", err, path)
		}
	})
}

func TestLoadVars_NoFileNoVars(t *testing.T) {
	got, err := LoadVars("", nil)
	if err != nil {
		t.Fatalf("LoadVars() error = %v", err)
	}
	if got == nil {
		t.Fatal("LoadVars() = nil map, want an empty one: a nil .Values turns a missing key into a nil-pointer error")
	}
	if len(got) != 0 {
		t.Errorf("LoadVars() = %#v, want an empty map", got)
	}
}

func TestLoadVars_Var(t *testing.T) {
	cases := []struct {
		name string
		// file is the content of a v.yaml; empty means no --var-file at all.
		file string
		vars []string
		want map[string]any
	}{
		{name: "plain", vars: []string{"ns=black widow"}, want: map[string]any{"ns": "black widow"}},
		{name: "always a string", vars: []string{"n=3", "b=true"}, want: map[string]any{"n": "3", "b": "true"}},
		{name: "empty value", vars: []string{"x="}, want: map[string]any{"x": ""}},
		{name: "value with equals", vars: []string{"q=a=b"}, want: map[string]any{"q": "a=b"}},
		{name: "value with comma", vars: []string{"hosts=a,b"}, want: map[string]any{"hosts": "a,b"}},
		{name: "overrides file", file: "ns: one\n", vars: []string{"ns=two"}, want: map[string]any{"ns": "two"}},
		{name: "later wins", vars: []string{"ns=one", "ns=two"}, want: map[string]any{"ns": "two"}},
		{name: "nested", vars: []string{"db.host=x"}, want: map[string]any{"db": map[string]any{"host": "x"}}},
		{
			name: "nested keeps siblings",
			file: "db:\n  host: a\n  port: 5432\n",
			vars: []string{"db.host=b"},
			want: map[string]any{"db": map[string]any{"host": "b", "port": 5432}},
		},
		{
			name: "deep",
			vars: []string{"a.b.c=x"},
			want: map[string]any{"a": map[string]any{"b": map[string]any{"c": "x"}}},
		},
		{name: "plain replaces mapping", file: "db:\n  host: a\n", vars: []string{"db=x"}, want: map[string]any{"db": "x"}},
		{
			name: "leaf replaces mapping",
			file: "a:\n  b:\n    c: 1\n",
			vars: []string{"a.b=x"},
			want: map[string]any{"a": map[string]any{"b": "x"}},
		},
		{
			// "db:" with nothing below is YAML null: nothing is there to
			// protect, so it is filled in like a missing key.
			name: "nested through a null from the file",
			file: "db:\n",
			vars: []string{"db.host=h"},
			want: map[string]any{"db": map[string]any{"host": "h"}},
		},
		{
			name: "nested under a non-string file key",
			file: "ports:\n  80: http\n",
			vars: []string{"ports.8080=alt"},
			want: map[string]any{"ports": map[string]any{"80": "http", "8080": "alt"}},
		},
		{
			// An alias must not share its mapping with the anchor.
			name: "alias is independent of its anchor",
			file: "base: &b\n  host: a\ncopy: *b\n",
			vars: []string{"copy.host=z"},
			want: map[string]any{"base": map[string]any{"host": "a"}, "copy": map[string]any{"host": "z"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			varFile := ""
			if tc.file != "" {
				varFile = writeVarFile(t, "v.yaml", tc.file)
			}
			got, err := LoadVars(varFile, tc.vars)
			if err != nil {
				t.Fatalf("LoadVars() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("LoadVars() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestLoadVars_VarErrors also pins what an error must NOT carry: the value of
// a --var. It may be a secret, and stderr ends up in CI logs. The error names
// the position of the flag and, once it is known, the key - the same rule
// ResolveValues follows for values: ("name the key, never the result").
func TestLoadVars_VarErrors(t *testing.T) {
	const secret = "s3cr3t-token"
	cases := []struct {
		name string
		file string
		vars []string
		// wantIn lists substrings the error must carry.
		wantIn []string
	}{
		// Without "=" nothing tells key from value, so only the position is
		// named: "--var password:s3cr3t-token" is one typo away.
		{name: "no equals sign", vars: []string{"ok=1", "password:" + secret}, wantIn: []string{"--var #2", "key=value"}},
		{name: "empty key", vars: []string{"=" + secret}, wantIn: []string{"--var #1"}},
		{name: "empty middle segment", vars: []string{"a..b=" + secret}, wantIn: []string{"--var #1", `"a..b"`}},
		{name: "empty first segment", vars: []string{".a=" + secret}, wantIn: []string{"--var #1", `".a"`}},
		{name: "empty last segment", vars: []string{"a.=" + secret}, wantIn: []string{"--var #1", `"a."`}},
		{
			// Turning db into a mapping silently would make {{ .Values.db }}
			// print map[host:...] instead of prod-db.
			name:   "through a scalar from the file",
			file:   "db: prod-db\n",
			vars:   []string{"db.host=" + secret},
			wantIn: []string{"--var #1", `"db.host"`, "db is already set", "mapping"},
		},
		{
			name:   "through a scalar from an earlier var",
			vars:   []string{"a=x", "a.password=" + secret},
			wantIn: []string{"--var #2", `"a.password"`, "mapping"},
		},
		{
			name:   "through a nested scalar",
			file:   "a:\n  b: text\n",
			vars:   []string{"a.b.c=" + secret},
			wantIn: []string{"--var #1", "a.b is already set"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			varFile := ""
			if tc.file != "" {
				varFile = writeVarFile(t, "v.yaml", tc.file)
			}
			got, err := LoadVars(varFile, tc.vars)
			if err == nil {
				t.Fatalf("LoadVars() = %#v, want an error", got)
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error %q echoes the value of the variable", err)
			}
		})
	}
}
