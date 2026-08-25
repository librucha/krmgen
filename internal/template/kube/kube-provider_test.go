package kube

import "testing"

func TestResolveKubeEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "reads KUBE_ prefix", env: map[string]string{"KUBE_X": "value"}, args: []string{"X"}, want: "value"},
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
			got, err := ResolveKubeEnv(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveKubeEnv() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ResolveKubeEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}
