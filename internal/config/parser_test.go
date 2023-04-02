package config

import (
	"github.com/librucha/krmgen/internal"
	"log"
	"os"
	"reflect"
	"testing"
)

func TestIsConfigFile(t *testing.T) {
	type args struct {
		filePath string
	}
	tempDir, _ := os.MkdirTemp("", "TestIsConfigFile")
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			log.Println(err)
		}
	}(tempDir)
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "full config",
			args: args{filePath: "../../test/resources/full/full-krmgen-config.yaml"},
			want: true,
		},
		{
			name: "common yaml file",
			args: args{filePath: "../../test/resources/no-krmgen.yaml"},
			want: false,
		},
		{
			name: "wrong path",
			args: args{filePath: ""},
			want: false,
		},
		{
			name: "directory path",
			args: args{filePath: tempDir},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConfigFile(tt.args.filePath); got != tt.want {
				t.Errorf("IsConfigFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	_ = os.Setenv("ARGOCD_APP_REL_NAME", "krmgen-app")
	_ = os.Setenv("ARGOCD_APP_REL_PROFILE", "test0")
	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		args    args
		want    *types.Config
		wantErr bool
	}{
		{
			name: "full config",
			args: args{filePath: "../../test/resources/full/full-krmgen-config.yaml"},
			want: &types.Config{
				ApiVersion: "krmgen.config.librucha.com/v1alpha1",
				Kind:       "KrmGen",
				Metadata: &types.Metadata{
					Annotations: map[string]string{"krmgen.io/plugin": "some-plugin"},
					Labels:      map[string]string{"app.kubernetes.io/name": "krmgen-controller"},
				},
				Helm: &types.Helm{
					Charts: &[]types.HelmChart{
						{
							Name:        "helm-app",
							RepoUrl:     "oci://helm.registry.io/helm/",
							Username:    `{{ argocdEnv "MY_USERNAME" "krmgenUser" }}`,
							Password:    `{{ argocdEnv "MY_PASSWORD" "" }}`,
							ReleaseName: `{{ argocdEnv "REL_NAME" }}`,
							Version:     "5.4.3",
							ValuesInline: map[string]any{
								"appVersion": "1.0.0",
								"name":       "test",
								"profile":    `{{ argocdEnv "REL_PROFILE" }}`,
								"logging":    map[string]any{"enabled": true},
							},
							ValuesFile: "",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig(tt.args.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseConfig() got = %v, want %v", got, tt.want)
			}
		})
	}
}
