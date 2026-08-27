package helm

import (
	"testing"

	cons "github.com/librucha/krmgen/internal/utils"
)

func TestSelectRenderer(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		setEnv   bool
		wantName string
	}{
		{name: "unset selects the library", setEnv: false, wantName: "helm library"},
		{name: "empty is treated as unset", env: "", setEnv: true, wantName: "helm library"},
		{name: "a path selects the binary", env: "/usr/local/bin/helm", setEnv: true, wantName: "helm binary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(cons.EnvHelmExecutable, tt.env)
			} else {
				t.Setenv(cons.EnvHelmExecutable, "")
			}
			if got := selectRenderer().Name(); got != tt.wantName {
				t.Errorf("selectRenderer().Name() = %q, want %q", got, tt.wantName)
			}
		})
	}
}
