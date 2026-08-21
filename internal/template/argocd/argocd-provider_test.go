package argocd

import "testing"

func TestResolveArgocdEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "ARGOCD_ENV_ wins", env: map[string]string{"ARGOCD_ENV_X": "env"}, args: []string{"X"}, want: "env"},
		{name: "ARGOCD_APP_ is the fallback", env: map[string]string{"ARGOCD_APP_X": "app"}, args: []string{"X"}, want: "app"},
		{name: "ARGOCD_ENV_ beats ARGOCD_APP_", env: map[string]string{"ARGOCD_ENV_X": "env", "ARGOCD_APP_X": "app"}, args: []string{"X"}, want: "env"},
		{name: "default when unset", args: []string{"X", "fallback"}, want: "fallback"},
		{name: "empty default is honoured", args: []string{"X", ""}, want: ""},
		{name: "error when unset without default", args: []string{"X"}, wantErr: true},
		{name: "no arguments is an error", args: []string{}, wantErr: true},
		{name: "three arguments is an error", args: []string{"X", "a", "b"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := ResolveArgocdEnv(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveArgocdEnv() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ResolveArgocdEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}
