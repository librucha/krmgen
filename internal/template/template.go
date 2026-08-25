package template

import (
	"context"
	"strings"
	"sync"
	"text/template"

	"github.com/Masterminds/goutils"
	"github.com/Masterminds/sprig/v3"
	"github.com/librucha/cloud-go-templates/azure"
	"github.com/librucha/krmgen/internal/template/argocd"
	"github.com/librucha/krmgen/internal/template/files"
	"github.com/librucha/krmgen/internal/template/krmgen"
	"github.com/librucha/krmgen/internal/template/kube"
)

// The provider is built once per process, not once per template. Templates are
// evaluated file by file, and a provider per file would give each file its own
// cache - turning one Azure lookup into one per file that mentions the secret.
var (
	azureOnce     sync.Once
	azureProvider *azure.Provider
	azureErr      error
)

func azureFuncs() (template.FuncMap, error) {
	azureOnce.Do(func() {
		azureProvider, azureErr = azure.New(context.Background())
	})
	if azureErr != nil {
		return nil, azureErr
	}
	return azureProvider.FuncMap(), nil
}

func initFuncs(t *template.Template) error {
	funcs := sprig.FuncMap()
	// Deleted for security reasons
	delete(funcs, "env")
	delete(funcs, "expandenv")

	// Add Krmgen functions
	funcs[krmgen.VersionFunc] = krmgen.ResolveKrmgenVersion
	funcs[krmgen.GeneratedFunc] = krmgen.ResolveKrmgenGenerated

	// Add Azure functions from cloud-go-templates
	azFuncs, err := azureFuncs()
	if err != nil {
		return err
	}
	for name, fn := range azFuncs {
		funcs[name] = fn
	}
	// Deprecated alias: azUaIdClientId was this function's name before it moved
	// to the library. Kept so existing krmgen.yaml files keep working.
	funcs["azUaIdClientId"] = funcs["azUserIdentityClientId"]

	// Add ArgoCD env function
	funcs[argocd.EnvFunc] = argocd.ResolveArgocdEnv

	// Add ArgoCD Kube env function
	funcs[kube.EnvFunc] = kube.ResolveKubeEnv

	// Add files func
	funcs[files.ReadFileFunc] = files.ReadFile

	t.Funcs(funcs)
	return nil
}

func EvalGoTemplates(content string) (string, error) {
	if goutils.IsBlank(content) {
		return content, nil
	}
	t := template.New("krmgen")
	if err := initFuncs(t); err != nil {
		return "", err
	}
	tmpl, err := t.Parse(content)
	if err != nil {
		return "", err
	}
	var buffer strings.Builder
	if err := tmpl.Execute(&buffer, nil); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
