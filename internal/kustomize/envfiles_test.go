package kustomize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// checkEnvFileFixture writes an env file and runs the check over a
// kustomization that references it the way a user would.
func checkEnvFileFixture(t *testing.T, kustomization, envFile string) error {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "resources"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "resources", "secrets.properties"), []byte(envFile), 0600); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(kustomization), &parsed); err != nil {
		t.Fatal(err)
	}
	return checkGeneratorEnvFiles(parsed, dir)
}

const envGenerator = `
secretGenerator:
  - name: demo
    envs:
      - resources/secrets.properties
`

func TestCheckGeneratorEnvFiles_RejectsAMultilineValue(t *testing.T) {
	const secret = "TOP-SECRET-VALUE-12345"
	err := checkEnvFileFixture(t, envGenerator,
		"creds={\n  \"private_key\": \""+secret+"\",\n  \"id\": \"x\"\n}\n")
	if err == nil {
		t.Fatal("error = nil, want a value spanning several lines to be rejected")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error quoted the secret back: %v", err)
	}
	for _, want := range []string{"secretGenerator", `"demo"`, "resources/secrets.properties", "line 2", `"files"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to contain %q", err, want)
		}
	}
}

// The check must not turn ordinary env files into errors. Every line here is
// something kustomize accepts today and produces a usable key from.
func TestCheckGeneratorEnvFiles_AcceptsOrdinaryEnvFiles(t *testing.T) {
	cases := map[string]string{
		"plain":                     "KEY=value\n",
		"value with an equals sign": "KEY=a=b=c\n",
		"empty value":               "KEY=\n",
		// kustomize reads a bare key as "key with an empty value".
		"bare key":           "KEY\n",
		"comment and blanks": "# a comment\n\n   \nKEY=value\n",
		"indented":           "   KEY=value\n",
		// Valid ConfigMap/Secret keys that are not valid env var names -
		// these work today and must keep working.
		"leading digit":       "1password=value\n",
		"dots and dashes":     "my.key-name_x=value\n",
		"no trailing newline": "KEY=value",
		"utf8 bom":            "\xEF\xBB\xBFKEY=value\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkEnvFileFixture(t, envGenerator, content); err != nil {
				t.Errorf("error = %v, want %q to be accepted", err, content)
			}
		})
	}
}

func TestCheckGeneratorEnvFiles_ReportsTheFirstOffendingLine(t *testing.T) {
	err := checkEnvFileFixture(t, envGenerator, "OK=1\nnot a key\nalso not a key\n")
	if err == nil {
		t.Fatal("error = nil, want the malformed line to be rejected")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error = %v, want it to point at line 2", err)
	}
}

func TestCheckGeneratorEnvFiles_CoversConfigMapsAndTheDeprecatedSingularField(t *testing.T) {
	cases := map[string]string{
		"configMapGenerator envs": "\nconfigMapGenerator:\n  - name: demo\n    envs:\n      - resources/secrets.properties\n",
		"deprecated env":          "\nsecretGenerator:\n  - name: demo\n    env: resources/secrets.properties\n",
	}
	for name, kustomization := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkEnvFileFixture(t, kustomization, "creds={\n  \"x\": 1\n}\n"); err == nil {
				t.Error("error = nil, want the malformed env file to be rejected")
			}
		})
	}
}

// A generator with no env source, and a kustomization with no generator at
// all, are the common case - neither may be turned into an error.
func TestCheckGeneratorEnvFiles_IgnoresWhatItDoesNotCover(t *testing.T) {
	cases := map[string]string{
		"no generator":            "resources:\n  - base.yaml\n",
		"literals":                "\nconfigMapGenerator:\n  - name: demo\n    literals:\n      - key=value\n",
		"files":                   "\nsecretGenerator:\n  - name: demo\n    files:\n      - creds=resources/secrets.properties\n",
		"generator is not a list": "secretGenerator: nonsense\n",
	}
	for name, kustomization := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkEnvFileFixture(t, kustomization, "creds={\n  \"x\": 1\n}\n"); err != nil {
				t.Errorf("error = %v, want no error", err)
			}
		})
	}
}

// A missing env file is kustomize's error to report; this check must not
// pre-empt it with a worse message.
func TestCheckGeneratorEnvFiles_LeavesAMissingFileToKustomize(t *testing.T) {
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(envGenerator), &parsed); err != nil {
		t.Fatal(err)
	}
	if err := checkGeneratorEnvFiles(parsed, t.TempDir()); err != nil {
		t.Errorf("error = %v, want a missing env file to pass through untouched", err)
	}
}
