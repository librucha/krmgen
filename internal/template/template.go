package template

import (
	"github.com/Masterminds/goutils"
	"github.com/Masterminds/sprig/v3"
	"github.com/librucha/krmgen/internal/template/argocd"
	azcert "github.com/librucha/krmgen/internal/template/azure/cert"
	azkey "github.com/librucha/krmgen/internal/template/azure/key"
	azsec "github.com/librucha/krmgen/internal/template/azure/sec"
	azstorage "github.com/librucha/krmgen/internal/template/azure/storage"
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
	funcs[azsec.SecFunc] = azsec.GetSecret
	funcs[azsec.ToPemFunc] = azsec.ToPemBlock
	// Add Azure key vault certificates
	funcs[azcert.CertFunc] = azcert.ResolveCert
	// Add Azure key vault keys
	funcs[azkey.KeyFunc] = azkey.ResolveKey
	// Add Azure storage key
	funcs[azstorage.StoreKeyFunc] = azstorage.GetStoreKey

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
