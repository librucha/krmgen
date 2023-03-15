package template

import (
	"github.com/Masterminds/goutils"
	"github.com/Masterminds/sprig/v3"
	"github.com/librucha/krmgen/internal/template/argocd"
	"github.com/librucha/krmgen/internal/template/azure"
	"github.com/librucha/krmgen/internal/template/files"
	"github.com/librucha/krmgen/internal/template/kube"
	"strings"
	"text/template"
)

func initFuncs(t *template.Template) {
	funcs := sprig.FuncMap()
	// Deleted for security reasons
	delete(funcs, "env")
	delete(funcs, "expandenv")

	// Add Azure key vault secrets
	funcs[azure.SecFunc] = azure.ResolveSecret

	// Add ArgoCD env function
	funcs[argocd.EnvFunc] = argocd.ResolveArgocdEnv

	// Add ArgoCD Kube env function
	funcs[kube.EnvFunc] = kube.ResolveKubeEnv

	// Add files func
	funcs[files.ReadFileFunc] = files.ReadFile

	t.Funcs(funcs)
}

func EvalGoTemplates(content string) (string, error) {
	if goutils.IsBlank(content) {
		return content, nil
	}
	t := template.New("krmgen")
	initFuncs(t)
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
