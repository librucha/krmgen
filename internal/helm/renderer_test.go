package helm

import (
	"os"
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
			// t.Setenv first so the cleanup restores whatever the host had,
			// then genuinely unset it - otherwise the "unset" row would test
			// the empty-string case and selectRenderer's found==false branch
			// would never be entered by any unit test.
			t.Setenv(cons.EnvHelmExecutable, tt.env)
			if !tt.setEnv {
				if err := os.Unsetenv(cons.EnvHelmExecutable); err != nil {
					t.Fatal(err)
				}
			}
			if got := selectRenderer().Name(); got != tt.wantName {
				t.Errorf("selectRenderer().Name() = %q, want %q", got, tt.wantName)
			}
		})
	}
}
