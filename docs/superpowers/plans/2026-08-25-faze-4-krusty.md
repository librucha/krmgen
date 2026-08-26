# Fáze 4: Kustomize přes knihovnu, kubectl jako opt-in — implementační plán

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Nahradit volání `kubectl kustomize` zabudovanou knihovnou `krusty`, aniž by se změnil jediný bajt výstupu, a ponechat externí kubectl jako podporovanou volbu.

**Architecture:** Vzniká rozhraní `Builder` se dvěma implementacemi — externí binárka a zabudovaná knihovna. Volba je jediné místo, které čte `KRMGEN_KUBECTL_EXECUTABLE`: když je nastavená, jede externí nástroj; když ne, jede knihovna. **Příprava pracovního adresáře se nemění** — zápis resources do souboru a jeho vepsání do kustomizace na disku zůstává, protože na tom stojí dvě dokumentované odchylky, které golden sada jmenovitě hlídá.

**Tech Stack:** Go 1.26, `sigs.k8s.io/kustomize/api/krusty`, `sigs.k8s.io/kustomize/kyaml/filesys`, kubectl (nadále podporovaný)

**Spec:** `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md` (rozhodnutí R1, R2, R3), produktová specifikace `docs/specification.md`

## Global Constraints

- Go 1.26.0; `.tool-versions` uvádí `golang 1.26.3`
- Kód, komentáře i dokumentace **anglicky**
- **Golden sada musí zůstat zelená bez regenerace jediného goldenu.** To je brána fáze
- **Žádný test nesmí na síť.** Výjimkou zůstávají testy za build tagem `oci`
- Verze nástrojů jsou ukotvené v `test/golden/versions_test.go`; harness je kontroluje před porovnáním
- Commituje se výčtem cest, nikdy `git add -A`
- **Nepushovat.** Push je rozhodnutí uživatele

---

## Změřeno předem, 2026-08-25

Tahle fáze stojí na třech měřeních. Neověřuj je znovu, ale **věř jim jen pro tuhle dvojici verzí**: `sigs.k8s.io/kustomize/api v0.21.1` proti kubectl v1.36.3 s vestavěnou Kustomize v5.8.1.

| Otázka | Výsledek |
|---|---|
| Shoduje se výstup `krusty` a `kubectl kustomize`? | **Bajtově ano** — s `Reorder: ReorderOptionLegacy` |
| A s výchozími volbami? | **Ne.** `MakeDefaultOptions()` má `Reorder: ReorderOptionNone`, což dá jiné pořadí resourců |
| Sedí chybové hlášky? | **Ano, doslova.** kubectl jen obaluje tutéž knihovní chybu prefixem `error: ` |

Druhý řádek je past téhle fáze. S výchozím nastavením diffnou úplně všechny goldeny a je snadné to schválit jako „nevyhnutelný rozdíl verzí". Není. Je to jeden přepínač.

Ověřeno na dvou případech: prostá kustomizace s `namespace`, a složitá s `commonLabels`, `namePrefix`, `configMapGenerator`, CRD a Service dohromady. Ostatní výchozí volby `krusty` už se s kubectl shodují — `AddManagedbyLabel: false`, `LoadRestrictions: LoadRestrictionsRootOnly`, pluginy vypnuté.

### Co se vědomě nemění

`BuildKustomize` dnes zapíše resources do souboru s náhodným jménem, **vepíše ho do kustomizace na disku** a teprve pak staví. Zůstává to tak. Na tom chování stojí dvě odchylky, které specifikace popisuje a golden sada jmenovitě testuje:

- víc `kind: KrmGen` souborů se sdílenou kustomizací → druhý průchod naakumuluje soubor z prvního a build selže (`TestError_MultiConfigWithKustomization`)
- kustomizace jen v podadresáři se najde, ale staví se z kořene (`TestError_KustomizationOnlyInSubdirectory`)

Přechod na in-memory filesystem by obojí změnil. Je to lákavé zlepšení a nepatří do fáze, jejíž brána zní „nic se nezměnilo".

### Co v téhle fázi není

Designový dokument k fázi 4 přiřazuje dvě položky z tabulky kvality: `log.Fatal` → návratové chyby (26 míst) a neuklizený pracovní adresář se secrets. **Tenhle plán je neřeší.** Je to jiná práce s jiným rizikem — mění exit kódy a texty hlášek, které golden sada hlídá — a zaslouží si vlastní plán. Poslední úloha to zapíše do designového dokumentu, aby se ty položky neztratily podruhé.

---

## File Structure

| Soubor | Zodpovědnost |
|---|---|
| `internal/kustomize/builder.go` | Vytvořit. Rozhraní `Builder`, volba backendu podle prostředí. |
| `internal/kustomize/builder_kubectl.go` | Vytvořit. Externí kubectl — dnešní chování beze změny. |
| `internal/kustomize/builder_krusty.go` | Vytvořit. Zabudovaná knihovna. |
| `internal/kustomize/builder_test.go` | Vytvořit. Volba backendu, shoda obou. |
| `internal/kustomize/processor.go` | Upravit. `BuildKustomize` deleguje na `Builder`; příprava adresáře beze změny. |
| `internal/kustomize/processor_test.go` | Upravit. Stávající testy zůstávají; `runCommand` seam se přesune k externímu backendu. |
| `test/golden/fixtures/kustomize-features/` | Vytvořit. Scénář pokrývající víc než dnešní `namespace` a `commonLabels`. |
| `test/golden/harness_test.go` | Upravit. Diferenciální běh: každý scénář oběma backendy. |
| `test/golden/versions_test.go` | Upravit. Ukotvit i verzi zabudované knihovny. |
| `docs/specification.md` | Upravit. Sekce 5 a 6 — matice nástrojů a výjimky z parity. |
| `CLAUDE.md` | Upravit. Strom architektury a proměnné prostředí. |
| `go.mod`, `go.sum` | Upravit. Přidat `sigs.k8s.io/kustomize/api`. |

---

### Task 1: Rozšířit goldeny o kustomize funkce, které dnes nikdo netestuje

Brána téhle fáze zní „goldeny beze změny". Dnešní čtyři scénáře s kustomizací používají jen `namespace` a `commonLabels` — to je na výměnu backendu tenká pojistka. Tahle úloha ji zesílí **dřív**, než se cokoli mění.

**Files:**
- Create: `test/golden/fixtures/kustomize-features/{krmgen.yaml,kustomization.yaml,base.yaml,patch.yaml,golden.yaml}`
- Modify: `test/golden/harness_test.go`

**Interfaces:**
- Consumes: `runScenario(t, name, extraEnv ...string) result`, `assertGolden(t, name, got string)` z fáze 2
- Produces: scénář `kustomize-features`, na kterém úloha 5 měří shodu obou backendů

- [ ] **Step 1: Vytvořit fixture**

`test/golden/fixtures/kustomize-features/krmgen.yaml` — jen kustomize, žádný helm:

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
```

`test/golden/fixtures/kustomize-features/base.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: demo
spec:
  ports:
    - port: 80
  selector:
    app: demo
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
data:
  mode: base
```

`test/golden/fixtures/kustomize-features/patch.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
data:
  mode: patched
```

`test/golden/fixtures/kustomize-features/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: features
namePrefix: pre-
nameSuffix: -suf
labels:
  - pairs:
      managed-by: krmgen
    includeSelectors: true
resources:
  - base.yaml
patches:
  - path: patch.yaml
configMapGenerator:
  - name: generated
    literals:
      - key=value
```

Pozn.: `labels` s `includeSelectors: true` je moderní náhrada za `commonLabels`, který je deprecated. Scénář `helm-with-kustomize` používá starý tvar schválně — pinuje dnešní chování. Tenhle používá nový, aby fáze 4 měřila obojí.

- [ ] **Step 2: Přidat test**

Do `test/golden/harness_test.go`:

```go
// TestGolden_KustomizeFeatures covers the transformers the other scenarios
// never touch. The phase that swaps kustomize for a library is measured
// against these goldens, and namespace plus labels alone would be a thin
// thing to measure against.
func TestGolden_KustomizeFeatures(t *testing.T) {
	res := runScenario(t, "kustomize-features")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "kustomize-features", res.stdout)

	for _, want := range []string{
		"name: pre-demo-suf",       // namePrefix and nameSuffix
		"namespace: features",      // namespace transformer
		"mode: patched",            // the patch replaced the base value
		"managed-by: krmgen",       // labels transformer
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("output is missing %q", want)
		}
	}
	if !strings.Contains(res.stdout, "pre-generated-suf") {
		t.Error("the generated ConfigMap is missing or was not renamed")
	}
}
```

- [ ] **Step 3: Vygenerovat golden a přečíst ho celý**

```bash
go test ./test/golden/ -run TestGolden_KustomizeFeatures -update
cat test/golden/fixtures/kustomize-features/golden.yaml
```

Ověř, že tam je: Service i ConfigMap s prefixem i suffixem, namespace `features`, `mode: patched` (ne `base`), label na obou i v selectoru Service, a vygenerovaná ConfigMap s hashem v názvu. **Hash v názvu generované ConfigMapy je deterministický** — počítá se z obsahu, ne z času; kdyby se mezi běhy měnil, zastav se a nahlas to.

- [ ] **Step 4: Ověřit stabilitu a citlivost**

```bash
go test ./test/golden/ -count=1 >/dev/null && echo "beh 1 OK"
go test ./test/golden/ -count=1 >/dev/null && echo "beh 2 OK"
sed -i '' 's/mode: base/mode: BASE/' test/golden/fixtures/kustomize-features/base.yaml
go test ./test/golden/ -run TestGolden_KustomizeFeatures -count=1 2>&1 | grep -c FAIL | xargs -I{} echo "po poskozeni FAIL: {} (1 = spravne)"
sed -i '' 's/mode: BASE/mode: base/' test/golden/fixtures/kustomize-features/base.yaml
go test ./test/golden/ -run TestGolden_KustomizeFeatures -count=1 >/dev/null && echo "po obnove OK"
```

- [ ] **Step 5: Commit**

```bash
git add test/golden/fixtures/kustomize-features test/golden/harness_test.go
git commit -m "test: cover the kustomize transformers no scenario touched

Patches, generators, name prefixes and the modern labels form were all
unmeasured. The phase that replaces kubectl kustomize with a library is
gated on these goldens not moving, so the gate should be worth passing."
```

---

### Task 2: Rozhraní Builder a externí backend

Tahle úloha **nemění chování ani o bajt**. Vytáhne dnešní volání kubectl za rozhraní a nic víc.

**Files:**
- Create: `internal/kustomize/builder.go`, `internal/kustomize/builder_kubectl.go`
- Modify: `internal/kustomize/processor.go`, `internal/kustomize/processor_test.go`

**Interfaces:**
- Consumes: `var runCommand = tool.RunCommand` z fáze 2
- Produces: `type Builder interface { Build(dir string) (string, error); Name() string }`, `func newKubectlBuilder(executable string) Builder` — úlohy 3 až 5 na nich staví

- [ ] **Step 1: Napsat padající test**

Do `internal/kustomize/builder_test.go`:

```go
package kustomize

import (
	"errors"
	"reflect"
	"testing"
)

func TestKubectlBuilder_InvokesTheBinaryWithTheDirectory(t *testing.T) {
	var gotName string
	var gotArgs []string
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		gotName, gotArgs = name, arg
		return "kind: ConfigMap\n", "", nil
	}

	got, err := newKubectlBuilder("kubectl").Build("/work")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got != "kind: ConfigMap\n" {
		t.Errorf("Build() = %q, want the command output", got)
	}
	if gotName != "kubectl" {
		t.Errorf("invoked %q, want %q", gotName, "kubectl")
	}
	if !reflect.DeepEqual(gotArgs, []string{"kustomize", "/work"}) {
		t.Errorf("args = %v, want [kustomize /work]", gotArgs)
	}
}

func TestKubectlBuilder_HonoursAnExplicitExecutable(t *testing.T) {
	var gotName string
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		gotName = name
		return "", "", nil
	}

	if _, err := newKubectlBuilder("/opt/bin/kubectl").Build("/work"); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if gotName != "/opt/bin/kubectl" {
		t.Errorf("invoked %q, want the configured path", gotName)
	}
}

func TestKubectlBuilder_CarriesStderrIntoTheError(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		return "", "unable to find one of 'kustomization.yaml'", errors.New("exit status 1")
	}

	_, err := newKubectlBuilder("kubectl").Build("/work")
	if err == nil {
		t.Fatal("Build() error = nil, want the failure to propagate")
	}
	if !strings.Contains(err.Error(), "unable to find one of 'kustomization.yaml'") {
		t.Errorf("error = %v, want it to carry the tool's stderr", err)
	}
}
```

Do importů doplň `errors`, `reflect`, `strings` a `testing`.

- [ ] **Step 2: Pustit a ověřit RED**

```bash
go test ./internal/kustomize/ -run TestKubectlBuilder 2>&1 | tail -3
```

Očekávané: FAIL, `undefined: newKubectlBuilder`.

- [ ] **Step 3: Napsat rozhraní**

`internal/kustomize/builder.go`:

```go
package kustomize

// Builder renders a prepared kustomization directory into YAML.
//
// Two implementations exist: one shells out to kubectl, one uses the
// kustomize library directly. They are expected to produce identical output;
// where they cannot, the difference is recorded in docs/specification.md
// rather than treated as a defect.
type Builder interface {
	// Build renders the kustomization rooted at dir.
	Build(dir string) (string, error)
	// Name identifies the backend in diagnostics.
	Name() string
}
```

- [ ] **Step 4: Napsat externí backend**

`internal/kustomize/builder_kubectl.go`:

```go
package kustomize

import "fmt"

// kubectlBuilder renders by running the kubectl binary, which embeds its own
// copy of kustomize. Which version that is depends on the installed kubectl -
// see docs/specification.md, "Kustomize version follows kubectl".
type kubectlBuilder struct {
	executable string
}

func newKubectlBuilder(executable string) Builder {
	return kubectlBuilder{executable: executable}
}

func (b kubectlBuilder) Name() string { return "kubectl " + b.executable }

func (b kubectlBuilder) Build(dir string) (string, error) {
	stdOut, stdErr, err := runCommand(b.executable, "kustomize", dir)
	if err != nil {
		return "", fmt.Errorf("run kubectl kustomize failed error: %s reason: %s", err, stdErr)
	}
	return stdOut, nil
}
```

Hlášku ponech **znak po znaku** jako dnes v `BuildKustomize` — golden sada na ni tvrdí.

- [ ] **Step 5: Přepojit `BuildKustomize`**

V `internal/kustomize/processor.go` nahraď blok, který sestavuje argumenty a volá `runCommand`, voláním builderu:

```go
	out, err := newKubectlBuilder("kubectl").Build(workDir)
	if err != nil {
		log.Fatalf("%s", err)
	}
	return out
```

`log` je tu logrus, jak ho balíček importuje dnes — nepřejmenovávej ho ani neměň způsob ukončení. Celá hláška se teď skládá v builderu, takže `log.Fatalf` jen předá, co dostal; výsledný text na stderr musí zůstat znak po znaku stejný. Příprava adresáře (`prepareKustomizeFile`) zůstává přesně jak je.

- [ ] **Step 6: Pustit testy a ověřit, že se nic nezměnilo**

```bash
go test ./internal/kustomize/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
make build && go test ./test/golden/ -count=1 -v 2>&1 | grep -cE '^--- PASS'
```

Očekávané: balíček zelený, všech 15 golden testů PASS **bez regenerace**.

- [ ] **Step 7: Commit**

```bash
git add internal/kustomize/builder.go internal/kustomize/builder_kubectl.go internal/kustomize/builder_test.go internal/kustomize/processor.go internal/kustomize/processor_test.go
git commit -m "refactor: put the kustomize build behind an interface

No behaviour change: the same binary is invoked with the same arguments and
fails with the same message. This only creates the seam the library backend
plugs into."
```

---

### Task 3: Zabudovaný backend přes krusty

**Files:**
- Create: `internal/kustomize/builder_krusty.go`
- Modify: `internal/kustomize/builder_test.go`, `go.mod`, `go.sum`

**Interfaces:**
- Consumes: `Builder` z úlohy 2
- Produces: `func newKrustyBuilder() Builder`

- [ ] **Step 1: Přidat závislost**

```bash
go get sigs.k8s.io/kustomize/api@v0.21.1
go mod tidy
grep -E 'kustomize/(api|kyaml)' go.mod | grep -v indirect
```

Očekávané: `sigs.k8s.io/kustomize/api v0.21.1`. Verzi **nezvyšuj** — je spárovaná s tou, kterou embeduje ukotvený kubectl, a shoda výstupu byla ověřena právě pro tuhle dvojici.

- [ ] **Step 2: Napsat padající test**

Do `internal/kustomize/builder_test.go`:

```go
func TestKrustyBuilder_RendersADirectory(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("kustomization.yaml", "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nnamespace: ns\nresources:\n  - cm.yaml\n")
	write("cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n")

	got, err := newKrustyBuilder().Build(dir)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(got, "namespace: ns") {
		t.Errorf("Build() = %q, want the namespace transformer applied", got)
	}
}

// The library defaults to no reordering; kubectl applies the legacy order.
// Leaving that unset would reorder every rendered document and move every
// golden file, which is easy to mistake for an unavoidable version
// difference. It is not - it is one option.
func TestKrustyBuilder_UsesLegacyResourceOrdering(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Deliberately listed service-first; legacy ordering emits the CRD first.
	write("kustomization.yaml", "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - res.yaml\n")
	write("res.yaml", "apiVersion: v1\nkind: Service\nmetadata:\n  name: svc\nspec:\n  ports:\n    - port: 80\n---\napiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: demos.krmgen.test\nspec:\n  group: krmgen.test\n  scope: Namespaced\n  names:\n    plural: demos\n    singular: demo\n    kind: Demo\n  versions:\n    - name: v1\n      served: true\n      storage: true\n      schema:\n        openAPIV3Schema:\n          type: object\n")

	got, err := newKrustyBuilder().Build(dir)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	crd := strings.Index(got, "kind: CustomResourceDefinition")
	svc := strings.Index(got, "kind: Service")
	if crd < 0 || svc < 0 {
		t.Fatalf("Build() = %q, want both documents", got)
	}
	if crd > svc {
		t.Error("the CustomResourceDefinition must come first - legacy ordering is not in effect")
	}
}

func TestKrustyBuilder_ReportsAMissingKustomization(t *testing.T) {
	_, err := newKrustyBuilder().Build(t.TempDir())
	if err == nil {
		t.Fatal("Build() error = nil, want an error for a directory with no kustomization")
	}
	if !strings.Contains(err.Error(), "unable to find one of 'kustomization.yaml'") {
		t.Errorf("error = %v, want the same wording kubectl reports", err)
	}
}
```

Do importů doplň `os`, `path/filepath`, `strings`.

- [ ] **Step 3: Pustit a ověřit RED**

```bash
go test ./internal/kustomize/ -run TestKrustyBuilder 2>&1 | tail -3
```

Očekávané: FAIL, `undefined: newKrustyBuilder`.

- [ ] **Step 4: Napsat backend**

`internal/kustomize/builder_krusty.go`:

```go
package kustomize

import (
	"fmt"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// krustyBuilder renders with the kustomize library compiled into krmgen, so
// the version is pinned in go.mod rather than being whatever kubectl the host
// happens to have.
type krustyBuilder struct{}

func newKrustyBuilder() Builder { return krustyBuilder{} }

func (krustyBuilder) Name() string { return "embedded kustomize" }

func (krustyBuilder) Build(dir string) (string, error) {
	opts := krusty.MakeDefaultOptions()
	// kubectl kustomize applies the legacy resource ordering; the library
	// defaults to none. Without this every rendered document would be
	// reordered relative to what krmgen has always produced.
	opts.Reorder = krusty.ReorderOptionLegacy

	result, err := krusty.MakeKustomizer(opts).Run(filesys.MakeFsOnDisk(), dir)
	if err != nil {
		return "", fmt.Errorf("run kustomize failed: %w", err)
	}
	out, err := result.AsYaml()
	if err != nil {
		return "", fmt.Errorf("rendering kustomize output failed: %w", err)
	}
	return string(out), nil
}
```

Ten obal nemusí napodobovat kubectl. Chybové testy golden sady tvrdí na podřetězce
`unable to find one of 'kustomization.yaml'` a `already registered id`, a ty pocházejí
z chyby **knihovny**, kterou kubectl jen přeposílá — změřeno předem, obě cesty je vypisují
doslova stejně. Stačí tedy chybu nezahodit.

- [ ] **Step 5: Pustit testy**

```bash
go test ./internal/kustomize/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Očekávané: vše PASS, včetně testu na pořadí.

- [ ] **Step 6: Commit**

```bash
git add internal/kustomize/builder_krusty.go internal/kustomize/builder_test.go go.mod go.sum
git commit -m "feat: add an embedded kustomize backend

Pins the kustomize version in go.mod instead of inheriting whatever kubectl
the host has. Legacy resource ordering is set explicitly: the library
defaults to none, which would reorder every document krmgen renders."
```

---

### Task 4: Volba backendu a přepnutí výchozího chování

**Files:**
- Modify: `internal/kustomize/builder.go`, `internal/kustomize/builder_test.go`, `internal/kustomize/processor.go`

**Interfaces:**
- Consumes: `newKubectlBuilder`, `newKrustyBuilder`
- Produces: `func selectBuilder() Builder`

- [ ] **Step 1: Napsat padající test**

```go
func TestSelectBuilder_EmbeddedByDefault(t *testing.T) {
	t.Setenv(cons.EnvKubectlExecutable, "")
	os.Unsetenv(cons.EnvKubectlExecutable)

	if got := selectBuilder().Name(); got != "embedded kustomize" {
		t.Errorf("selectBuilder() = %q, want the embedded backend when no executable is configured", got)
	}
}

func TestSelectBuilder_ExternalWhenConfigured(t *testing.T) {
	t.Setenv(cons.EnvKubectlExecutable, "/opt/bin/kubectl")

	got := selectBuilder().Name()
	if !strings.Contains(got, "/opt/bin/kubectl") {
		t.Errorf("selectBuilder() = %q, want the configured binary", got)
	}
}

func TestSelectBuilder_EmptyValueMeansEmbedded(t *testing.T) {
	// An exported-but-empty variable is a configuration accident, not a
	// request for the external tool.
	t.Setenv(cons.EnvKubectlExecutable, "")

	if got := selectBuilder().Name(); got != "embedded kustomize" {
		t.Errorf("selectBuilder() = %q, want the embedded backend for an empty value", got)
	}
}
```

Do importů doplň `os` a `cons "github.com/librucha/krmgen/internal/utils"`.

- [ ] **Step 2: Pustit a ověřit RED**

```bash
go test ./internal/kustomize/ -run TestSelectBuilder 2>&1 | tail -3
```

Očekávané: FAIL, `undefined: selectBuilder`.

- [ ] **Step 3: Napsat volbu**

Do `internal/kustomize/builder.go`:

```go
// selectBuilder decides which backend renders. This is the only place that
// reads KRMGEN_KUBECTL_EXECUTABLE: setting it opts into the external tool on
// the host, which is a supported choice for anyone who needs to pin the
// exact kustomize their environment ships. Leaving it unset uses the version
// compiled into krmgen.
//
// Until this phase the variable was declared and never read - the binary was
// always taken from PATH.
func selectBuilder() Builder {
	if executable, found := os.LookupEnv(cons.EnvKubectlExecutable); found && executable != "" {
		return newKubectlBuilder(executable)
	}
	return newKrustyBuilder()
}
```

a v `processor.go` nahraď `newKubectlBuilder("kubectl")` za `selectBuilder()`.

- [ ] **Step 4: Pustit testy a golden sadu**

```bash
go test ./internal/kustomize/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
make build && go test ./test/golden/ -count=1 -v 2>&1 | grep -E '^--- ' | sed 's/(.*//'
```

Očekávané: všechny golden testy PASS **bez regenerace jediného goldenu** — a to je ta chvíle, kdy fáze dokazuje svůj smysl. Goldeny vznikly proti kubectl; teď je vyrábí knihovna a nesmí se hnout ani bajt.

Pokud se pohnou, **nepřegeneruj je.** Přečti diff: skoro jistě to bude pořadí resourců, tedy chybějící `Reorder`.

- [ ] **Step 5: Commit**

```bash
git add internal/kustomize/builder.go internal/kustomize/builder_test.go internal/kustomize/processor.go
git commit -m "feat: default to the embedded kustomize, keep kubectl opt-in

KRMGEN_KUBECTL_EXECUTABLE was declared and never read; it now selects the
external tool. Unset means the version compiled into krmgen. This is a
behaviour change for anyone who relied on their host kubectl being used
implicitly, and it is what the refactoring exists to deliver."
```

---

### Task 5: Diferenciální test — oba backendy, jeden golden

Tohle je vlastnost, kterou designový dokument chtěl od fáze 2 a která je teprve teď možná: **každý scénář projít oběma backendy a porovnat.**

**Files:**
- Modify: `test/golden/harness_test.go`, `test/golden/versions_test.go`

**Interfaces:**
- Consumes: `runScenario`, `assertGolden`, scénáře z fází 2 a 3 a z úlohy 1

- [ ] **Step 1: Napsat diferenciální test**

```go
// TestGolden_BothBackendsAgree runs every scenario that involves kustomize
// through both backends and requires byte-identical output. This is the
// measurement the whole phase exists to make: the goldens prove krmgen still
// renders what it always did, and this proves the choice of backend does not
// change what a user gets.
func TestGolden_BothBackendsAgree(t *testing.T) {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		t.Fatalf("kubectl is required to compare backends: %v", err)
	}

	for _, name := range []string{
		"kustomize-only",
		"helm-with-kustomize",
		"kustomize-features",
	} {
		t.Run(name, func(t *testing.T) {
			embedded := runScenario(t, name)
			if embedded.exitCode != 0 {
				t.Fatalf("embedded backend exit code = %d\nstderr: %s", embedded.exitCode, embedded.stderr)
			}
			external := runScenario(t, name, "KRMGEN_KUBECTL_EXECUTABLE="+kubectlPath)
			if external.exitCode != 0 {
				t.Fatalf("external backend exit code = %d\nstderr: %s", external.exitCode, external.stderr)
			}
			if embedded.stdout != external.stdout {
				t.Errorf("the two backends disagree:\n%s", diff(external.stdout, embedded.stdout))
			}
		})
	}
}

// TestGolden_ExternalBackendMatchesTheGoldens keeps the external path honest:
// the goldens are generated with whichever backend is default, so without
// this the opt-in path could drift unnoticed.
func TestGolden_ExternalBackendMatchesTheGoldens(t *testing.T) {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		t.Fatalf("kubectl is required: %v", err)
	}
	res := runScenario(t, "kustomize-features", "KRMGEN_KUBECTL_EXECUTABLE="+kubectlPath)
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "kustomize-features", res.stdout)
}
```

Do importů doplň `os/exec`.

- [ ] **Step 2: Pustit**

```bash
go test ./test/golden/ -run 'TestGolden_BothBackendsAgree|TestGolden_ExternalBackendMatchesTheGoldens' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- |ok|FAIL)' | head -20
```

Očekávané: vše PASS. Pokud některý scénář nesouhlasí, **nepřegeneruj golden** — zapiš přesný rozdíl do reportu; je to buď chybějící volba backendu, nebo skutečná výjimka z parity, která patří do specifikace.

- [ ] **Step 3: Ukotvit verzi zabudované knihovny**

V `test/golden/versions_test.go` doplň vedle stávajících kotev:

```go
// anchorKustomizeAPIVersion is the sigs.k8s.io/kustomize/api version compiled
// into krmgen. Output was verified byte-identical between this version and
// the kubectl anchored above; a different pair may render differently, and
// then a golden diff is a tooling change, not a product regression.
const anchorKustomizeAPIVersion = "v0.21.1"

func TestEmbeddedKustomizeMatchesTheAnchor(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Skip("build info is unavailable")
	}
	for _, dep := range info.Deps {
		if dep.Path == "sigs.k8s.io/kustomize/api" {
			if dep.Version != anchorKustomizeAPIVersion {
				t.Errorf("kustomize/api is %s, goldens were generated against %s - "+
					"this is a tooling change, not a product regression; verify the "+
					"output before updating the anchor", dep.Version, anchorKustomizeAPIVersion)
			}
			return
		}
	}
	t.Error("sigs.k8s.io/kustomize/api is not among the build dependencies")
}
```

Do importů doplň `runtime/debug`.

Pozn.: `ReadBuildInfo` čte závislosti **testovací binárky**, ne postavené `build/krmgen`. Pro účel kotvy to stačí — obojí se překládá ze stejného `go.mod`.

- [ ] **Step 4: Plná sada a commit**

```bash
go test -race -count=1 ./... 2>&1 | grep -E 'FAIL|^ok'
git add test/golden/harness_test.go test/golden/versions_test.go
git commit -m "test: require both kustomize backends to agree

The design called for differential goldens from the start; until there was
a second backend it could not be written. Every kustomize scenario now runs
through both and must come out byte-identical."
```

---

### Task 6: Dokumentace a přesun položek, které ztratily domov

**Files:**
- Modify: `docs/specification.md`, `CLAUDE.md`, `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`

- [ ] **Step 1: Přepsat matici nástrojů ve specifikaci**

V `docs/specification.md` sekci 5 přepiš tabulku proměnných tak, aby popisovala skutečnost:

| Variable | Effect when set | Effect when unset |
|---|---|---|
| `KRMGEN_HELM_EXECUTABLE` | That path is used as helm | `helm` is looked up in `PATH` |
| `KRMGEN_KUBECTL_EXECUTABLE` | That path is used as kubectl, and kustomize rendering goes through it | The kustomize version compiled into krmgen renders |

a smaž větu, která říká, že proměnná je deklarovaná, ale nepoužitá — od téhle fáze to neplatí.

Doplň, kterou verzi krmgen embeduje a proti které byla shoda ověřena.

- [ ] **Step 2: Doplnit výjimky z parity**

Do sekce 6 přidej řádek k tabulce a odstavec:

```markdown
| Kustomize version | Whatever kubectl ships | Pinned in `go.mod` |
```

```markdown
Both backends are required to render byte-identical output, and a test
enforces it for every scenario the golden suite covers. The one thing that
differs is the version: the external path renders with whatever kustomize the
installed kubectl embeds, which on an older host may be years behind the
pinned one. That is the trade the option exists to make.
```

- [ ] **Step 3: Narovnat `CLAUDE.md`**

Ve stromu architektury doplň nové soubory `internal/kustomize/builder*.go` a v tabulce proměnných prostředí oprav popis `KRMGEN_KUBECTL_EXECUTABLE` ze „declared but not implemented" na to, co dělá.

- [ ] **Step 4: Zapsat výsledek fáze a přesunout, co nemá domov**

V `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`:
- u fáze 4 zapiš dosažený stav: oba backendy, shoda ověřená testem, goldeny beze změny
- v tabulce kvality **přesuň** `log.Fatal` → návratové chyby a neuklizený pracovní adresář se secrets **mimo fázi 4** — tenhle plán je vědomě neřeší. Dej jim vlastní řádek nebo je přiřaď fázi 5. Ať se neztratí podruhé.

- [ ] **Step 5: Plná kontrola a commit**

```bash
make build
go test -race -count=1 ./... 2>&1 | grep -E 'FAIL|^ok'
gofmt -l . | grep -v '^$' || echo "gofmt cisto"
go vet ./...
git status --porcelain test/golden/fixtures/ | head -1
git add docs/specification.md CLAUDE.md docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md
git commit -m "docs: record the kustomize backend split"
```

---

## Dokončení fáze

- [ ] **Brána fáze**

```bash
git diff <merge-base>..HEAD --stat -- test/golden/fixtures/
```

Očekávané: **prázdno u všech goldenů, které existovaly před touhle fází.** Jediný nový je `kustomize-features` z úlohy 1. Pokud se pohnul kterýkoli starý golden, fáze svou bránu nesplnila, i kdyby všechny testy svítily zeleně.

- [ ] **Ověřit obě cesty ručně**

```bash
make build
./build/krmgen generate test/golden/fixtures/kustomize-only > /tmp/embedded.yaml 2>/dev/null
KRMGEN_KUBECTL_EXECUTABLE=$(command -v kubectl) ./build/krmgen generate test/golden/fixtures/kustomize-only > /tmp/external.yaml 2>/dev/null
diff /tmp/embedded.yaml /tmp/external.yaml && echo "oba backendy souhlasi" && rm -f /tmp/embedded.yaml /tmp/external.yaml
```

- [ ] **Rozhodnutí k zaznamenání před fází 5**

1. Zůstane příprava kustomizace na disku, nebo přejde na in-memory filesystem? To druhé je čistší a odstraní dvě dokumentované odchylky — ale obě jsou dnes testované, takže je to změna produktu, ne úklid.
2. Kdy se zvedne `sigs.k8s.io/kustomize/api`? Zvednutí rozváže dvojici, proti které byla shoda ověřena.
