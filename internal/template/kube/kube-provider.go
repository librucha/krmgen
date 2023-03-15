package kube

import (
	"fmt"
	"os"
)

const EnvFunc = "kubeEnv"
const EnvKeyPrefix = "KUBE_"

func ResolveKubeEnv(args ...string) (string, error) {
	switch len(args) {
	case 1:
		return getKubeEnvValue(args[0], nil)
	case 2:
		return getKubeEnvValue(args[0], &args[1])
	default:
		return "", fmt.Errorf("wrong arguments count for function %q expected 1 or 2 aruments but got %d", EnvFunc, len(args))
	}
}

func getKubeEnvValue(key string, fallback *string) (string, error) {
	kubeKey := EnvKeyPrefix + key
	value, found := os.LookupEnv(kubeKey)
	if found {
		return value, nil
	}
	if fallback != nil {
		return *fallback, nil
	}
	return "", fmt.Errorf("ArgoCD Kube env value %q not found in env and default value not provided", kubeKey)
}
