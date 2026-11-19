package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runTemplate executes the root command the way main does and returns what
// it wrote to stdout.
func runTemplate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand("2.0.3")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"template"}, args...))
	err := root.Execute()
	return out.String(), err
}

func writeTemplateVarFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTemplate_Output(t *testing.T) {
	yamlFile := writeTemplateVarFile(t, "v.yaml", "ns: from-file\nenv: prod\ndb:\n  host: a\n  port: 5432\n")
	jsonFile := writeTemplateVarFile(t, "v.json", `{"n": 1000000}`)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "function", args: []string{"{{ krmgenGenerated }}"}, want: "krmgen-2.0.3"},
		// The result is printed byte for byte: `krmgen template ... > cert.pem`
		// must not gain a newline the template did not produce, nor lose one
		// it did.
		{name: "no newline added", args: []string{"x"}, want: "x"},
		{name: "trailing newline kept", args: []string{"x\n"}, want: "x\n"},
		{name: "var", args: []string{"--var", "ns=black widow", "{{ .Values.ns }}"}, want: "black widow"},
		{
			name: "var file",
			args: []string{"--var-file", yamlFile, "{{ .Values.ns }}/{{ .Values.env }}"},
			want: "from-file/prod",
		},
		{
			name: "var overrides file",
			args: []string{"--var-file", yamlFile, "--var", "ns=black widow", "{{ .Values.ns }}/{{ .Values.env }}"},
			want: "black widow/prod",
		},
		{
			name: "dotted var keeps siblings",
			args: []string{"--var-file", yamlFile, "--var", "db.host=b", "{{ .Values.db.host }}:{{ .Values.db.port }}"},
			want: "b:5432",
		},
		// Not 1e+06: the file is decoded by the YAML decoder, which keeps
		// integers as integers.
		{name: "json number", args: []string{"--var-file", jsonFile, "{{ .Values.n }}"}, want: "1000000"},
		// A StringSlice flag would split this into two assignments.
		{name: "comma in value", args: []string{"--var", "hosts=a,b", "{{ .Values.hosts }}"}, want: "a,b"},
		{name: "sprig", args: []string{"--var", "ns=abc", "{{ upper .Values.ns }}"}, want: "ABC"},
		// A template that starts with a dash would otherwise be parsed as a flag.
		{name: "leading dash after --", args: []string{"--", "-x"}, want: "-x"},
		{name: "empty template", args: []string{""}, want: ""},
		{name: "whitespace-only template", args: []string{" \n "}, want: " \n "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runTemplate(t, tc.args...)
			if err != nil {
				t.Fatalf("krmgen template %q error = %v", tc.args, err)
			}
			if got != tc.want {
				t.Errorf("krmgen template %q printed %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestTemplate_Errors(t *testing.T) {
	yamlFile := writeTemplateVarFile(t, "v.yaml", "ns: from-file\n")
	txtFile := writeTemplateVarFile(t, "v.txt", "ns: from-file\n")

	const hint = "use --var key=value or --var-file"
	cases := []struct {
		name string
		args []string
		// wantIn lists substrings the error must carry, notIn ones it must not.
		wantIn []string
		notIn  []string
	}{
		{name: "no arguments", args: nil, wantIn: []string{"exactly one <template> argument", "got 0"}},
		{name: "two positional arguments", args: []string{"a", "b"}, wantIn: []string{"exactly one <template> argument", "got 2"}},
		{name: "missing key", args: []string{"{{ .Values.missing }}"}, wantIn: []string{`"missing"`, hint}},
		{
			// Text before the failing action must not leak out: the result
			// is printed only once the whole template has evaluated.
			name:   "missing key after literal text",
			args:   []string{"printed-too-early {{ .Values.missing }}"},
			wantIn: []string{`"missing"`, hint},
		},
		// The hint is about a variable nobody passed; a template that does
		// not even parse has a different problem.
		{name: "parse error", args: []string{"{{ .Values.ns"}, wantIn: []string{"template evaluation failed"}, notIn: []string{hint}},
		{
			name:   "function failure is not a missing variable",
			args:   []string{`{{ fail "boom" }}`},
			wantIn: []string{"boom"},
			notIn:  []string{hint},
		},
		{
			// A function may fail with an error quoting a URL that carries a
			// credential; the password must not reach stderr.
			name:   "URL password in an error is masked",
			args:   []string{`{{ fail "cloning https://user:s3cr3t-token@example.com/repo failed" }}`},
			wantIn: []string{"user:***@example.com"},
			notIn:  []string{"s3cr3t-token"},
		},
		{
			name:   "var without equals",
			args:   []string{"--var", "ok=1", "--var", "password:s3cr3t-token", "x"},
			wantIn: []string{"--var #2", "key=value"},
			notIn:  []string{"s3cr3t-token"},
		},
		{
			name:   "var value is not echoed",
			args:   []string{"--var", "a=1", "--var", "a.password=s3cr3t-token", "x"},
			wantIn: []string{"--var #2", `"a.password"`},
			notIn:  []string{"s3cr3t-token"},
		},
		{name: "unsupported var file extension", args: []string{"--var-file", txtFile, "x"}, wantIn: []string{txtFile, ".yaml"}},
		{
			// The second file must not silently win over the first.
			name:   "var file given twice",
			args:   []string{"--var-file", yamlFile, "--var-file", yamlFile, "x"},
			wantIn: []string{"--var-file given 2 times"},
		},
		{
			// A template starting with a dash and passed without -- is
			// rejected by flag parsing, before the command runs. That error
			// quotes the argument, so it needs the same masking - and it
			// should say what to do.
			name:   "template starting with a dash without --",
			args:   []string{"-xhttps://user:s3cr3t-token@example.com/"},
			wantIn: []string{"must follow --"},
			notIn:  []string{"s3cr3t-token"},
		},
		// The -- advice belongs to an unknown flag only.
		{name: "flag without its value", args: []string{"--var"}, wantIn: []string{"--var"}, notIn: []string{"must follow --"}},
		{
			// What an unset shell variable produces: --var-file "$VARS".
			// Rendering without the variables would hide the mistake.
			name:   "empty var file path",
			args:   []string{"--var-file", "", "x"},
			wantIn: []string{"--var-file", "empty"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runTemplate(t, tc.args...)
			if err == nil {
				t.Fatalf("krmgen template %q succeeded and printed %q, want an error", tc.args, got)
			}
			if got != "" {
				t.Errorf("krmgen template %q printed %q to stdout on failure, want nothing", tc.args, got)
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			for _, unwanted := range tc.notIn {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("error %q must not contain %q", err, unwanted)
				}
			}
		})
	}
}

// TestTemplate_GenerateCommandUnchanged pins the scope of the variable flags:
// they belong to the template command only. generate takes its values from
// KrmGen files, and growing a second source there is a separate decision.
func TestTemplate_GenerateCommandUnchanged(t *testing.T) {
	root := NewRootCommand("x")
	generate, _, err := root.Find([]string{"generate"})
	if err != nil || generate == root {
		t.Fatalf("generate command not found: %v", err)
	}
	for _, name := range []string{"var", "var-file"} {
		if generate.Flags().Lookup(name) != nil {
			t.Errorf("generate has a --%s flag, want it on the template command only", name)
		}
	}
	template, _, err := root.Find([]string{"template"})
	if err != nil || template == root {
		t.Fatalf("template command not found: %v", err)
	}
	for _, name := range []string{"var", "var-file"} {
		if template.Flags().Lookup(name) == nil {
			t.Errorf("template has no --%s flag", name)
		}
	}
}
