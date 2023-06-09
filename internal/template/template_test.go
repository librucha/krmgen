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
				t.Errorf("EvalGoTemplates() got = %s, want %s", got, tt.want)
			}
		})
	}
}
