package helm

import (
	types "github.com/librucha/krmgen/internal"
	cons "github.com/librucha/krmgen/internal/utils"
	"os"
	"reflect"
	"testing"
)

func Test_newGenerator(t *testing.T) {
	repoConfig := &types.HelmChart{RepoUrl: "https://grafana.github.io/helm-charts"}
	ociConfig := &types.HelmChart{RepoUrl: "oci://github.com"}
	type args struct {
		config *types.HelmChart
	}
	tests := []struct {
		name    string
		args    args
		want    generator
		wantErr bool
	}{
		{
			name: "repo generator",
			args: args{
				config: repoConfig},
			want:    newRepoHelmGenerator(repoConfig),
			wantErr: false,
		},
		{
			name: "oci generator",
			args: args{
				config: ociConfig},
			want:    newOciHelmGenerator(ociConfig),
			wantErr: false,
		},
		{
			name: "unknown generator",
			args: args{
				config: &types.HelmChart{RepoUrl: "totally unknown"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newGenerator(tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("newGenerator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newGenerator() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_credentialsProvided(t *testing.T) {
	type args struct {
		config *types.HelmChart
	}
	tests := []struct {
		name string
		env  map[string]string
		args args
		want bool
	}{
		{
			name: "provided both inline",
			args: args{
				config: &types.HelmChart{
					Username: "username",
					Password: "password",
				}},
			want: true,
		},
		{
			name: "provided both in ENV",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "",
				},
			},
			env: map[string]string{
				cons.EnvHelmUsername: "username",
				cons.EnvHelmPassword: "password",
			},
			want: true,
		},
		{
			name: "empty username",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "password",
				}},
			want: true,
		},
		{
			name: "provided only username in ENV",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "",
				},
			},
			env: map[string]string{
				cons.EnvHelmUsername: "username",
			},
			want: true,
		},
		{
			name: "provided only password in ENV",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "",
				},
			},
			env: map[string]string{
				cons.EnvHelmPassword: "password",
			},
			want: true,
		},
		{
			name: "empty password",
			args: args{
				config: &types.HelmChart{
					Username: "username",
					Password: "",
				}},
			want: true,
		},
		{
			name: "empty both credentials",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "",
				}},
			want: false,
		},
		{
			name: "ignored credentials inline",
			args: args{
				config: &types.HelmChart{
					Username:          "username",
					Password:          "password",
					IgnoreCredentials: true,
				}},
			want: false,
		},
		{
			name: "ignored credentials in ENV",
			args: args{
				config: &types.HelmChart{
					Username:          "",
					Password:          "",
					IgnoreCredentials: true,
				}},
			env: map[string]string{
				cons.EnvHelmUsername: "username",
				cons.EnvHelmPassword: "password",
			},
			want: false,
		},
		{
			name: "ignored empty credentials",
			args: args{
				config: &types.HelmChart{
					Username:          "",
					Password:          "",
					IgnoreCredentials: true,
				}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				_ = os.Setenv(k, v)
			}
			if got := credentialsProvided(tt.args.config); got != tt.want {
				t.Errorf("credentialsProvided() = %v, want %v", got, tt.want)
			}
			for k, _ := range tt.env {
				_ = os.Unsetenv(k)
			}
		})
	}
}

func Test_credentialsArgs(t *testing.T) {
	type args struct {
		config *types.HelmChart
	}
	tests := []struct {
		name string
		args args
		env  map[string]string
		want []string
	}{
		{
			name: "provided both inline",
			args: args{
				config: &types.HelmChart{
					Username: "username",
					Password: "password",
				}},
			want: []string{"--username", "username", "--password", "password"},
		},
		{
			name: "provided both in ENV",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "",
				},
			},
			env: map[string]string{
				cons.EnvHelmUsername: "username",
				cons.EnvHelmPassword: "password",
			},
			want: []string{"--username", "username", "--password", "password"},
		},
		{
			name: "empty username",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "password",
				}},
			want: []string{"--password", "password"},
		},
		{
			name: "provided only username in ENV",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "",
				},
			},
			env: map[string]string{
				cons.EnvHelmUsername: "username",
			},
			want: []string{"--username", "username"},
		},
		{
			name: "provided only password in ENV",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "",
				},
			},
			env: map[string]string{
				cons.EnvHelmPassword: "password",
			},
			want: []string{"--password", "password"},
		},
		{
			name: "empty password",
			args: args{
				config: &types.HelmChart{
					Username: "username",
					Password: "",
				}},
			want: []string{"--username", "username"},
		},
		{
			name: "empty both credentials",
			args: args{
				config: &types.HelmChart{
					Username: "",
					Password: "",
				}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				_ = os.Setenv(k, v)
			}
			if got := credentialsArgs(tt.args.config); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("credentialsArgs() = %v, want %v", got, tt.want)
			}
			for k, _ := range tt.env {
				_ = os.Unsetenv(k)
			}
		})
	}
}
