package helm

import (
	cons "github.com/librucha/krmgen/internal/utils"
	"os"
	"os/exec"
	"testing"
)

func Test_helmExecutable(t *testing.T) {
	helmExec, _ := exec.LookPath("helm")
	tests := []struct {
		env  map[string]string
		name string
		want string
	}{
		{
			name: "fallback to default",
			want: helmExec,
		},
		{
			name: "from ENV",
			env:  map[string]string{cons.EnvHelmExecutable: "/usr/bin/myOwnHelmExec"},
			want: "/usr/bin/myOwnHelmExec",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				_ = os.Setenv(k, v)
			}
			if got := helmExecutable(); got != tt.want {
				t.Errorf("helmExecutable() = %v, want %v", got, tt.want)
			}
			for k, _ := range tt.env {
				_ = os.Unsetenv(k)
			}
		})
	}
}
