package template

import (
	"github.com/librucha/krmgen/internal/template/argocd"
	"os"
	"testing"
)

func Test_EvalGoTemplates(t *testing.T) {
	type args struct {
		text string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
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
		// Azure secrets
		{
			name: "azSec function",
			args: args{`Prefix {{ azSec "some-vault" "some-secret" }} suffix`},
			// args: args{`Prefix {{ azSec "vault_name" "key_id" }} suffix`},
			want: "Prefix secretValue suffix",
		},
		{
			name: "azSec function with version",
			// args: args{`Prefix {{ azSec "vault_name" "key_id" "key_version" }} suffix`},
			args: args{`Prefix {{ azSec "some-vault" "some-secret" "2d5b71a61fca4a269a735216f6f1ec8f" }} suffix`},
			want: "Prefix LAe9cFYtnG2NZmVYur5MVVLV5zYYC2NNAhEFTSjLEh78MTcrdGP5aa6G78nPYwaJ suffix",
		},
		{
			name:    "azSec function without id",
			args:    args{`Prefix {{ azSec }} suffix`},
			wantErr: true,
		},
		{
			name:    "azSec function with too many args",
			args:    args{`Prefix {{ azSec "1" "2" "3" "4" }} suffix`},
			wantErr: true,
		},
		// Azure Storage key
		{
			name:    "azStoreKey",
			args:    args{`Prefix {{ azStoreKey "subscription-id" "some-resource-group" "someazurestorage" }} suffix`},
			want:    "Prefix XXYYZZ0123456789ZZYYXX suffix",
			wantErr: false,
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
		{
			name: "argocd not existing env with default",
			args: args{`Prefix {{ argocdEnv "KRMGEN_UNKNOWN_ENV_KEY" "fallback value" }} suffix`},
			want: "Prefix fallback value suffix",
		},
		// unknown func
		{
			name:    "unknown func",
			args:    args{"Prefix {{`{{ anyTotallyUnknownFunc }}`}} suffix"},
			want:    `Prefix {{ anyTotallyUnknownFunc }} suffix`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		_ = os.Setenv(argocd.EnvEnvKeyPrefix+"TEST_KEY", "ArgoCD data")
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalGoTemplates(tt.args.text)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvalGoTemplates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvalGoTemplates() got = %v, want %v", got, tt.want)
			}
		})
	}
}
