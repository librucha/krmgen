package template

import (
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/librucha/krmgen/internal/template/argocd"
)

func Test_EvalGoTemplates(t *testing.T) {
	type args struct {
		text string
	}
	tests := []struct {
		name         string
		args         args
		want         string
		wantErr      bool
		requireAzure bool
	}{
		{
			name: "builtin function",
			args: args{`Prefix {{ print "hello" }} suffix`},
			want: "Prefix hello suffix",
		},
		{
			name: "sprig function",
			args: args{`Prefix {{ upper "hello" }} suffix`},
			want: "Prefix HELLO suffix",
		},
		{
			name: "empty input",
			args: args{""},
			want: "",
		},
		{
			name: "blank input",
			args: args{" \t"},
			want: " \t",
		},
		{
			name: "param as func",
			args: args{`Prefix {{ with $name:= "TEST_KEY"}}{{ argocdEnv $name }}{{end}} suffix`},
			want: "Prefix ArgoCD data suffix",
		},
		{
			name: "part of param as func",
			args: args{`Prefix {{ with $name:= "_KEY"}}{{ printf "TEST%s" $name | argocdEnv }}{{end}} suffix`},
			want: "Prefix ArgoCD data suffix",
		},
		{
			name: "part of param as func without variable",
			args: args{`Prefix {{ upper "test_key" | printf "%s"  | argocdEnv }} suffix`},
			want: "Prefix ArgoCD data suffix",
		},
		// Rainy scenarios
		{
			name:    "sprig env function",
			args:    args{`Prefix {{ env "PATH" }} suffix`},
			wantErr: true,
		},
		{
			name:    "sprig expandenv function",
			args:    args{`Prefix {{ expandenv "PATH" }} suffix`},
			wantErr: true,
		},
		// ArgoCD env
		{
			name: "argocd existing env",
			args: args{`Prefix {{ argocdEnv "TEST_KEY" }} suffix`},
			want: "Prefix ArgoCD data suffix",
		},
		{
			name: "argocd existing env with default",
			args: args{`Prefix {{ argocdEnv "TEST_KEY" "not used" }} suffix`},
			want: "Prefix ArgoCD data suffix",
		},
		// unknown func
		{
			name:    "unknown func",
			args:    args{"Prefix {{`{{ anyTotallyUnknownFunc }}`}} suffix"},
			want:    `Prefix {{ anyTotallyUnknownFunc }} suffix`,
			wantErr: false,
		},
		// Azure secret — require real Azure credentials, skipped in unit test runs
		{
			name:         "azure secret",
			args:         args{`Prefix {{ azSec "vault_name" "key_id" }} suffix`},
			want:         "Prefix secretValue suffix",
			requireAzure: true,
		},
		{
			name:         "azure secret with version",
			args:         args{`Prefix {{ azSec "vault_name" "key_id" "version" }} suffix`},
			want:         "Prefix secretValue suffix",
			requireAzure: true,
		},
	}
	for _, tt := range tests {
		_ = os.Setenv(argocd.EnvEnvKeyPrefix+"TEST_KEY", "ArgoCD data")
		t.Run(tt.name, func(t *testing.T) {
			if tt.requireAzure && os.Getenv("AZURE_TENANT_ID") == "" {
				t.Skip("skipping: requires Azure credentials (AZURE_TENANT_ID not set)")
			}
			got, err := EvalGoTemplates(tt.args.text)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvalGoTemplates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvalGoTemplates() got = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestEvalGoTemplates_RegistersEveryDocumentedFunction is the actual regression
// safety net for the phase 3 library extraction: the golden suite does not
// exercise Azure functions at all, so a botched rewiring (a dropped function,
// a rename, a lost deprecated alias) would otherwise pass unnoticed.
func TestEvalGoTemplates_RegistersEveryDocumentedFunction(t *testing.T) {
	// Every name docs/specification.md section 4 documents, plus the
	// deprecated alias krmgen keeps for backward compatibility.
	names := []string{
		"krmgenVer", "krmgenGenerated",
		"argocdEnv", "kubeEnv", "readF",
		"azSec", "toPem", "azPfxKey", "azPfxCrt",
		"azCert", "azKey", "azStoreKey", "azUserIdentityClientId",
		"azUaIdClientId",
		// a sprig function, to prove sprig is still merged in
		"upper",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			// A template that only references the function without calling it
			// parses if and only if the function is registered. "if false"
			// means the reference never runs, so a registered function
			// yields no error at all - any error means the probe failed.
			if _, err := EvalGoTemplates("{{ if false }}{{ " + name + " }}{{ end }}"); err != nil {
				t.Errorf("template function %q is not registered: %v", name, err)
			}
		})
	}
}

// TestAliasFunc_MissingTargetIsAnError proves a missing alias target is
// reported as an error rather than storing a nil entry that later panics in
// t.Funcs(). This is the scenario a library rename or a dropped function
// would trigger for the deprecated azUaIdClientId alias.
func TestAliasFunc_MissingTargetIsAnError(t *testing.T) {
	funcs := template.FuncMap{"present": func() string { return "x" }}
	if err := aliasFunc(funcs, "alias", "missing"); err == nil {
		t.Fatal("aliasFunc() error = nil, want an error when the target is not registered")
	}
	if _, ok := funcs["alias"]; ok {
		t.Error("aliasFunc() registered the alias despite the missing target")
	}
}

// TestAliasFunc_CopiesTheTarget proves a present target is aliased correctly.
func TestAliasFunc_CopiesTheTarget(t *testing.T) {
	funcs := template.FuncMap{"target": func() string { return "x" }}
	if err := aliasFunc(funcs, "alias", "target"); err != nil {
		t.Fatalf("aliasFunc() error = %v", err)
	}
	if _, ok := funcs["alias"]; !ok {
		t.Error("aliasFunc() did not register the alias")
	}
}

// TestEvalGoTemplates_DoesNotRegisterEnvFunctions guards the security
// deletion in initFuncs: sprig's env and expandenv must never be reachable
// from a template.
func TestEvalGoTemplates_DoesNotRegisterEnvFunctions(t *testing.T) {
	// sprig's env and expandenv are removed deliberately: templates must not
	// read arbitrary process environment.
	for _, name := range []string{"env", "expandenv"} {
		_, err := EvalGoTemplates("{{ if false }}{{ " + name + " \"X\" }}{{ end }}")
		if err == nil || !strings.Contains(err.Error(), "not defined") {
			t.Errorf("%q must not be registered, got err = %v", name, err)
		}
	}
}
