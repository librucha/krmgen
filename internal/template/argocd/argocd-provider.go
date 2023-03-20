package argocd

import (
	"fmt"
	"os"
)

const EnvFunc = "argocdEnv"
const EnvEnvKeyPrefix = "ARGOCD_ENV_"
const EnvAppKeyPrefix = "ARGOCD_APP_"

func ResolveArgocdEnv(args ...string) (string, error) {
	switch len(args) {
	case 1:
		return getArgocdEnvValue(args[0], nil)
	case 2:
		return getArgocdEnvValue(args[0], &args[1])
	default:
		return "", fmt.Errorf("wrong arguments count for function %q expected 1 or 2 aruments but got %d", EnvFunc, len(args))
	}
}

func getArgocdEnvValue(key string, fallback *string) (string, error) {
	argocdKey := EnvEnvKeyPrefix + key
	value, found := os.LookupEnv(argocdKey)
	if found {
		return value, nil
	}

	argocdKey = EnvAppKeyPrefix + key
	value, found = os.LookupEnv(argocdKey)
	if found {
		return value, nil
	}

	if fallback != nil {
		return *fallback, nil
	}
	return "", fmt.Errorf("ArgoCD env value %s not found in env and default value not provided", argocdKey)
}
