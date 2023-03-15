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
			name: "valid file",
			args: args{filePath: "../../test/resources/valid-krmgen.yaml"},
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
	_ = os.Setenv("MY_APP_NAME", "krmgen-app")
	_ = os.Setenv("MY_APP_PROFILE", "test0")
	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		args    args
		want    types.Config
		wantErr bool
	}{
		{
			name: "full config",
			args: args{filePath: "../../test/resources/full-krmgen-config.yaml"},
			want: types.Config{
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
							Username:    "krmgenUser",
							Password:    "",
							ReleaseName: "krmgen-app",
							Version:     "5.4.3",
							ValuesInline: map[string]any{
								"appVersion": "1.0.0",
								"name":       "test",
								"profile":    "test0",
								"logging":    map[string]any{"enabled": true},
							},
							ValuesFile: "",
						},
					},
				},
				Kustomize: &types.Kustomize{
					ConfigInline: map[string]any{
						"apiVersion": "kustomize.config.k8s.io/v1beta1",
						"kind":       "Kustomization",
						"namespace":  "default",
						"resources": []any{
							"kustomize/resources/cm.yaml",
							"kustomize/resources/sec.yaml",
						},
					},
					ConfigFile: "",
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
