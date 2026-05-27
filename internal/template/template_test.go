package template

import (
	"os"
	"testing"

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
