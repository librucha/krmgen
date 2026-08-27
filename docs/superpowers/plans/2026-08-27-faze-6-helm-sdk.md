# Fáze 6 — helm přes SDK, s binárkou jako opt-in

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Renderovat helm charty knihovnou zakompilovanou do binárky, s externím
`helm` zachovaným natrvalo jako opt-in přes `KRMGEN_HELM_EXECUTABLE` — beze změny
jediného bajtu vyrenderovaného výstupu.

**Architecture:** Přesná obdoba fáze 4. Volání `helm template` zmizí za rozhraní
`Renderer` se dvěma implementacemi; výběr dělá jediná funkce podle
`KRMGEN_HELM_EXECUTABLE`. Goldeny, které vznikly proti binárce, jsou měřítkem:
pokud je knihovna reprodukuje bajt po bajtu a ani jeden se nepohne, je výměna
prokazatelně zachovávající chování.

**Tech Stack:** Go 1.26.3, `helm.sh/helm/v4` v4.2.4 (stejná verze, proti které
jsou goldeny ukotvené), cobra, `sigs.k8s.io/kustomize/api`.

**Spec:** [`docs/specification.md`](../../specification.md) — sekce 5 (matice
podpory externích nástrojů) a 6 (výjimky z parity) popisují dnešní rozdělení pro
kustomize; tahle fáze do nich přidává helm.

**Design:** [`docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`](../specs/2026-08-20-krmgen-refaktoring-design.md)
— R1 (obě cesty existují natrvalo) a R2 (výchozí je embedded).

---

## Rozhodnutí jít/nejít: naměřeno, nikoli odhadnuto

Tahle fáze byla od začátku vedená jako jediná s reálnou možností „ne". Před
psaním tohoto plánu proběhla měření, která to rozhodnutí uzavírají.

### 1. Knihovna produkuje bajtově identický výstup

Spike proti té samé demo chartě, kterou používá golden sada, servírované přes
`httptest` — jednou přes `helm template`, jednou přes SDK:

```
binary bytes: 808
sdk bytes:    808
identical:    true
```

Zahrnuje `--include-crds` i `--namespace`. **Klíčové zjištění:** stačí
`new(action.Configuration)` bez jakékoli inicializace — `DryRunClient` nesahá
na kubeconfig ani na cluster. Ověřený kód je celý v úloze 2.

### 2. Cena je 3,1× větší binárka

| | dnes | s helm SDK |
|---|---|---|
| binárka (stripped) | **20,7 MB** | **63,7 MB** |
| moduly v grafu | 103 | 286 |

Pozor na past, do které jsem při měření spadl: build, který z helmu odkazuje jen
`action.NewInstall`, váží 45,3 MB, protože linker zbytek odřeže. Skutečná
implementace potřebuje i `LocateChart`, `loader.Load`, `getter.All` a
`registry.NewClient` — teprve s nimi vyjde 63,7 MB. Kdyby někdo v průběhu fáze
měřil znovu, musí odkazovat celou sadu.

### 3. Docker image se naopak zmenší

Tohle rozhodnutí obrací. `Dockerfile` dnes dělá `apk add git helm kubectl`:

| | dnes | po fázi 6 |
|---|---|---|
| krmgen | 20,7 MB | 63,7 MB |
| helm | ~59 MB | **0** |
| kubectl | ~60 MB | **0** (už od fáze 4) |
| **celkem binárek** | **~140 MB** | **~64 MB** |

Image tedy zhubne zhruba o 76 MB. Prodělají jen samostatné archivy z
goreleaseru, které rostou 3,1×.

### Závěr

**Jdeme do toho.** Parita je prokázaná, cluster není potřeba, image se zmenší.
Cena je velikost samostatné binárky — a `KRMGEN_HELM_EXECUTABLE` zůstává pro
kohokoli, komu vadí, nebo kdo potřebuje helm pluginy a post-renderery, které
knihovní cesta neumí a nikdy umět nebude.

---

## Co je na tomhle jiné než u kustomize

Fáze 4 byla snazší, než se čekalo. Tady jsou čtyři věci, které tam nebyly:

1. **Helm neexportuje převod výsledku.** `client.RunWithContext` vrací
   `release.Releaser`, což je `any`. `*release/v1.Release` z něj dostane
   `releaserToV1Release` v `pkg/cmd/root.go` — jenže ta je **neexportovaná**.
   Musíme si ten type switch napsat sami. Je triviální, ale nikdo ho za nás
   neudělá a při upgradu helmu je to první místo, které se rozbije.
2. **Sestavení výstupu není `rel.Manifest`.** `helm template` píše
   `strings.TrimSpace(rel.Manifest)` a **za to** ještě všechny hooky, každý s
   hlavičkou `---\n# Source: <path>\n`. Kdo vezme jen `Manifest`, přijde o hooky
   a golden se rozejde.
3. **Banner přestane existovat.** `stripHelmBanner` v
   `internal/helm/processor.go` řeší, že helm v4 píše `Pulled:`/`Digest:` na
   stdout. Knihovna žádný stdout nemá, takže na embedded cestě je ta funkce
   bezpředmětná. **Nesmí se smazat** — externí cesta ji potřebuje dál.
4. **OCI je jediná cesta, kterou goldeny nechrání.** Sedm scénářů jede helm přes
   lokální `httptest` repo bez sítě; `oci-public` je za build tagem `oci`,
   protože sahá na `ghcr.io`. Parita OCI se tedy měří ručně a zapisuje, ne
   dokazuje v CI. Úloha 5 to řeší explicitně.

## Global Constraints

- Go 1.26.3; veškerý kód, komentáře a anglická dokumentace **anglicky** (tento
  plán a design doc jsou česky — to je správně)
- `helm.sh/helm/v4` **v4.2.4** — tatáž verze, proti které jsou ukotvené goldeny
  (`test/golden/versions_test.go`, `anchorHelmVersion = "v4.2.4+g3900f43"`).
  Jiná verze v `go.mod` je chyba, ne upgrade.
- **Ani jeden golden se nesmí regenerovat.** Nikdy nespouštět test s `-update`.
- Žádný test nesmí na síť. OCI testy zůstávají za build tagem `oci`.
- Chybové hlášky se vrací, nefatalují (fáze 5). Exit kód při chybě zůstává 1.
- Stabilní podřetězce, na kterých stojí `test/golden/errors_test.go`, musí zůstat
- Po každé úloze musí projít: `golangci-lint run ./...`,
  `go test -race -count=1 ./...`, `gofmt -l .`, `go vet ./...`
- Commituje se výčtem cest, nepushuje se

## Struktura souborů

Zrcadlí `internal/kustomize` z fáze 4 — schválně, ať se to čte stejně.

| Soubor | Odpovědnost |
|---|---|
| `internal/helm/renderer.go` (nový) | rozhraní `Renderer`, `selectRenderer()` podle `KRMGEN_HELM_EXECUTABLE` |
| `internal/helm/renderer_binary.go` (nový) | dnešní cesta: sestaví argumenty, spustí `helm`, ořízne banner |
| `internal/helm/renderer_sdk.go` (nový) | embedded cesta přes `helm.sh/helm/v4/pkg/action` |
| `internal/helm/processor.go` | zůstává orchestrátorem; `templateHelm` deleguje na `Renderer` |
| `internal/helm/generator.go` a spol. | beze změny — identifikace repa a údaje jsou společné oběma cestám |
| `test/golden/harness_test.go` | diferenciální testy obou rendererů |
| `Dockerfile` | `apk add` bez helmu a kubectlu |
| `docs/specification.md` | helm v matici podpory a ve výjimkách parity |

---

### Task 1: Rozhraní `Renderer` s dnešním chováním za ním

Čistý refaktor: nic se nesmí chovat jinak. Cílem je mít šev, do kterého se v
úloze 2 dá vsunout druhá implementace.

**Files:**
- Create: `internal/helm/renderer.go`, `internal/helm/renderer_binary.go`
- Modify: `internal/helm/processor.go`
- Test: `internal/helm/renderer_test.go` (nový)

**Interfaces:**
- Produces:
  - `type Renderer interface { Render(cfg *types.HelmChart, g generator, workDir string) (string, error); Name() string }`
  - `func selectRenderer() Renderer`
- Consumes: `generator` a `getValuesArgs` z tohoto balíčku beze změny

- [ ] **Step 1: Napiš failující test na výběr rendereru**

`internal/helm/renderer_test.go`:

```go
func TestSelectRenderer(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		setEnv   bool
		wantName string
	}{
		{name: "unset selects the binary for now", setEnv: false, wantName: "helm binary"},
		{name: "empty is treated as unset", env: "", setEnv: true, wantName: "helm binary"},
		{name: "a path selects the binary", env: "/usr/local/bin/helm", setEnv: true, wantName: "helm binary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(cons.EnvHelmExecutable, tt.env)
			} else {
				t.Setenv(cons.EnvHelmExecutable, "")
			}
			if got := selectRenderer().Name(); got != tt.wantName {
				t.Errorf("selectRenderer().Name() = %q, want %q", got, tt.wantName)
			}
		})
	}
}
```

Pozn.: v téhle úloze vrací `selectRenderer` vždycky binárku — druhá
implementace ještě neexistuje. Tabulka se doplní v úloze 4.

- [ ] **Step 2: Spusť a ověř selhání překladu**

```bash
go test ./internal/helm/ 2>&1 | head -3
```
Expected: `undefined: selectRenderer`.

- [ ] **Step 3: Zaveď rozhraní**

`internal/helm/renderer.go`:

```go
// Renderer turns one chart declaration into rendered YAML. Two implementations
// exist for the whole lifetime of the project: the helm binary on the host, and
// the helm library compiled into krmgen. See docs/specification.md, section 5.
type Renderer interface {
	// Render returns the manifests for one chart, with no trailing newline
	// normalisation - the caller concatenates results verbatim.
	Render(cfg *types.HelmChart, g generator, workDir string) (string, error)
	// Name identifies the backend in errors and tests.
	Name() string
}

func selectRenderer() Renderer {
	return newBinaryRenderer()
}
```

- [ ] **Step 4: Přesuň dnešní tělo `templateHelm` do binárního rendereru**

`internal/helm/renderer_binary.go` dostane **beze změny** logiku, která dnes žije
v `templateHelm`: sestavení `args`, `login()`, `getValuesArgs`, `runCommand`,
`stripHelmBanner`. Znění chybové hlášky
`"run command %q finished with error %v. Error output %v"` zachovej doslova.

`templateHelm` se scvrkne na:

```go
func templateHelm(generator generator, workDir string) (string, error) {
	return selectRenderer().Render(generator.getConfig(), generator, workDir)
}
```

- [ ] **Step 5: Spusť celou sadu**

```bash
go test -race -count=1 ./...
```
Expected: PASS, **žádný golden se nehne** — je to přesun kódu, ne změna chování.

- [ ] **Step 6: Commit**

```bash
git add internal/helm/renderer.go internal/helm/renderer_binary.go internal/helm/renderer_test.go internal/helm/processor.go
git commit -m "refactor: put helm rendering behind an interface"
```

---

### Task 2: Embedded renderer pro HTTP repozitáře

Jádro fáze. Kód níž je **ověřený spikem** — přeložil se a vyrobil bajtově
identický výstup proti `helm template`. Neimprovizuj kolem něj.

**Files:**
- Create: `internal/helm/renderer_sdk.go`
- Modify: `go.mod`, `go.sum`
- Test: `internal/helm/renderer_sdk_test.go` (nový)

**Interfaces:**
- Produces: `func newSDKRenderer() Renderer`, `Name()` vrací `"helm library"`
- Consumes: `Renderer` z úlohy 1

- [ ] **Step 1: Přidej závislost v ukotvené verzi**

```bash
go get helm.sh/helm/v4@v4.2.4
go mod tidy
```
Ověř, že v `go.mod` je opravdu `v4.2.4` a ne novější — goldeny jsou ukotvené
proti ní (`test/golden/versions_test.go`).

- [ ] **Step 2: Napiš failující test proti lokálnímu repu**

Test si postaví chart repo stejně jako golden harness (`helm package` +
`helm repo index` + `httptest`), vyrenderuje ho oběma renderery a porovná:

```go
func TestSDKRendererMatchesTheBinary(t *testing.T) {
	repoURL := localChartRepo(t) // helper: package + index + httptest, no network

	cfg := &types.HelmChart{
		Name: "demo", RepoUrl: repoURL, ReleaseName: "rel",
		Version: "0.1.0", Namespace: "default", IgnoreCredentials: true,
	}
	g, err := newGenerator(cfg)
	if err != nil {
		t.Fatal(err)
	}

	viaBinary, err := newBinaryRenderer().Render(cfg, g, t.TempDir())
	if err != nil {
		t.Fatalf("binary renderer: %v", err)
	}
	viaSDK, err := newSDKRenderer().Render(cfg, g, t.TempDir())
	if err != nil {
		t.Fatalf("sdk renderer: %v", err)
	}
	if viaBinary != viaSDK {
		t.Errorf("renderers disagree\nbinary:\n%s\nsdk:\n%s", viaBinary, viaSDK)
	}
}
```

- [ ] **Step 3: Spusť, ověř selhání**

```bash
go test -race -run TestSDKRendererMatchesTheBinary ./internal/helm/ 2>&1 | head -3
```
Expected: `undefined: newSDKRenderer`.

- [ ] **Step 4: Napiš embedded renderer**

Tohle je ověřený tvar. Tři místa, kde se nejsnáz chybuje, jsou okomentovaná:

```go
func (sdkRenderer) Render(cfg *types.HelmChart, _ generator, workDir string) (string, error) {
	// DryRunClient never touches a kubeconfig or a cluster, so an empty
	// Configuration is enough - verified by spike before this phase started.
	client := action.NewInstall(new(action.Configuration))
	client.DryRunStrategy = action.DryRunClient
	client.ReleaseName = cfg.ReleaseName
	client.Replace = true // skip the name check, as `helm template` does
	client.IncludeCRDs = true
	client.Namespace = cfg.Namespace
	client.RepoURL = cfg.RepoUrl
	client.Version = cfg.Version
	client.Username = username(cfg)
	client.Password = password(cfg)

	settings := cli.New()
	chartPath, err := client.ChartPathOptions.LocateChart(cfg.Name, settings)
	if err != nil {
		return "", fmt.Errorf("locating chart %q failed error: %w", cfg.Name, err)
	}
	loaded, err := loader.Load(chartPath)
	if err != nil {
		return "", fmt.Errorf("loading chart %q failed error: %w", chartPath, err)
	}

	vals, err := (&values.Options{ValueFiles: valueFiles(cfg, workDir)}).MergeValues(getter.All(settings))
	if err != nil {
		return "", fmt.Errorf("merging values failed error: %w", err)
	}

	result, err := client.RunWithContext(context.Background(), loaded, vals)
	if err != nil {
		return "", fmt.Errorf("rendering chart %q failed error: %w", cfg.Name, err)
	}

	// helm does not export a converter for this: pkg/cmd/root.go keeps
	// releaserToV1Release unexported, so we repeat its type switch. This is
	// the first thing that breaks on a helm upgrade.
	rel, ok := result.(*releasev1.Release)
	if !ok {
		return "", fmt.Errorf("unexpected release type %T from helm", result)
	}

	// `helm template` writes the trimmed manifest and THEN every hook, each
	// behind its own "# Source:" header. Taking rel.Manifest alone drops the
	// hooks and the goldens diverge.
	var out bytes.Buffer
	fmt.Fprintln(&out, strings.TrimSpace(rel.Manifest))
	for _, h := range rel.Hooks {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", h.Path, h.Manifest)
	}
	return out.String(), nil
}
```

Importy, které to potřebuje:

```go
"helm.sh/helm/v4/pkg/action"
"helm.sh/helm/v4/pkg/chart/loader"
"helm.sh/helm/v4/pkg/cli"
"helm.sh/helm/v4/pkg/cli/values"
"helm.sh/helm/v4/pkg/getter"
releasev1 "helm.sh/helm/v4/pkg/release/v1"
```

`username`/`password`/`valueFiles` jsou malé pomocné funkce, které vytáhnou
totéž, co dnes skládá `credentialsArgs` a `getValuesArgs` do argumentů —
včetně fallbacku na `KRMGEN_HELM_USERNAME`/`KRMGEN_HELM_PASSWORD` a včetně
`valuesInline`, které se dnes zapisuje do dočasného souboru. **Nezaváděj druhou
cestu pro čtení údajů**; refaktoruj stávající tak, aby vracela hodnoty, a
argumenty z nich sestav až v binárním rendereru.

- [ ] **Step 5: Spusť porovnání**

```bash
go test -race -count=1 -run TestSDKRendererMatchesTheBinary ./internal/helm/ -v
```
Expected: PASS. Pokud ne, rozdíl si vytiskni celý — nejčastější příčinou budou
chybějící hooky (bod 4 výše) nebo přebytečná/chybějící koncová nová řádka.

- [ ] **Step 6: Commit**

```bash
git add internal/helm/renderer_sdk.go internal/helm/renderer_sdk_test.go go.mod go.sum
git commit -m "feat: add an embedded helm renderer"
```

---

### Task 3: Diferenciální test nad celou golden sadou

Jeden chart v unit testu je málo. Tahle úloha požaduje shodu na **každém**
scénáři, který helm používá.

**Files:**
- Modify: `test/golden/harness_test.go`

**Interfaces:**
- Consumes: `runScenario`, `minimalEnv` z harnessu

- [ ] **Step 1: Zjisti, které scénáře používají helm**

```bash
grep -l 'charts:' test/golden/fixtures/*/krmgen.yaml
```
Expected: sedm scénářů plus `bad-repo-scheme` (ten selže před renderem, takže do
porovnání nepatří — ověř to, než ho vyřadíš).

- [ ] **Step 2: Napiš diferenciální test**

Přesně po vzoru `TestGolden_BothBackendsAgree` z fáze 4: každý scénář se pustí
dvakrát — jednou s výchozím prostředím, jednou s
`KRMGEN_HELM_EXECUTABLE=<cesta k helmu>` — a stdout se musí shodovat bajt po
bajtu. Použij `exec.LookPath("helm")` a `t.Fatalf`, když helm není; parita se
bez něj měřit nedá.

**Pozor na vakuózní test:** ověř, že `minimalEnv()` `KRMGEN_HELM_EXECUTABLE`
nenastavuje, jinak by „embedded" běh nebyl embedded. Fáze 4 na tomhle stála a
fáze 5 ukázala, že se to snadno přehlédne.

- [ ] **Step 3: Spusť**

```bash
go test -race -count=1 -run TestGolden_BothHelmRenderersAgree ./test/golden/ -v
```
Expected: PASS na všech scénářích. Jakýkoli rozdíl je **nález, ne úklid** —
zapiš ho a řekni si o rozhodnutí, nepřepisuj golden.

- [ ] **Step 4: Commit**

```bash
git add test/golden/harness_test.go
git commit -m "test: require both helm renderers to agree"
```

---

### Task 4: Přepni výchozí backend na embedded

Až teď, když je parita naměřená.

**Files:**
- Modify: `internal/helm/renderer.go`, `internal/helm/renderer_test.go`

- [ ] **Step 1: Rozšiř tabulku v `TestSelectRenderer`**

```go
{name: "unset selects the library", setEnv: false, wantName: "helm library"},
{name: "empty is treated as unset", env: "", setEnv: true, wantName: "helm library"},
{name: "a path selects the binary", env: "/usr/local/bin/helm", setEnv: true, wantName: "helm binary"},
```

- [ ] **Step 2: Spusť, ověř selhání**

Expected: FAIL, `= "helm binary", want "helm library"` u prvních dvou.

- [ ] **Step 3: Otoč `selectRenderer`**

```go
func selectRenderer() Renderer {
	if executable, found := os.LookupEnv(cons.EnvHelmExecutable); found && executable != "" {
		return newBinaryRenderer(executable)
	}
	return newSDKRenderer()
}
```

Prázdná hodnota se chová jako nenastavená — stejně jako u
`selectBuilder` (`internal/kustomize/builder.go`) a jak to od fáze 5 dělá
`helmExecutable`.

- [ ] **Step 4: Brána fáze — spusť celou sadu a zkontroluj goldeny**

```bash
go test -race -count=1 ./...
git diff --stat -- test/golden/fixtures/
```
Expected: PASS a **prázdný** výstup druhého příkazu. Tohle je měření, kvůli
kterému fáze existuje: goldeny vznikly proti binárce, teď je vyrábí knihovna a
ani jeden se nepohnul.

- [ ] **Step 5: Commit**

```bash
git add internal/helm/renderer.go internal/helm/renderer_test.go
git commit -m "feat: default to the embedded helm, keep the binary opt-in"
```

---

### Task 5: OCI registry na embedded cestě

Jediná cesta, kterou goldeny nechrání, protože potřebuje síť.

**Files:**
- Modify: `internal/helm/renderer_sdk.go`, `internal/helm/oci-generator.go`
- Test: `test/golden/oci_test.go`

- [ ] **Step 1: Doplň registry klienta do embedded rendereru**

Pro `oci://` reference se `RepoURL` nepoužívá — chart se adresuje přímo. Podívej
se, jak to dělá `pkg/cmd/template.go` (`newRegistryClient` + `SetRegistryClient`,
řádky ~88-93 v v4.2.4) a udělej totéž. Přihlášení, které dnes dělá
`helm registry login` jako samostatný proces, se na embedded cestě řeší
předáním údajů registry klientovi — **je to jiný mechanismus, ne jen jiné
volání.** Externí cesta zapisuje do konfiguračního souboru helmu, embedded drží
autentizaci v paměti. Ten rozdíl patří do specifikace.

- [ ] **Step 2: Rozšiř OCI testy za build tagem**

`test/golden/oci_test.go` je za `//go:build oci`. Přidej do něj porovnání obou
rendererů na scénáři `oci-public`, stejné povahy jako úloha 3.

- [ ] **Step 3: Spusť s tagem a zapiš výsledek**

```bash
go test -race -count=1 -tags=oci ./test/golden/ -v
```
Do reportu vlož **skutečný výstup**. Pokud se cesty rozejdou, je to výjimka do
specifikace, ne důvod cokoli přepisovat.

- [ ] **Step 4: Commit**

```bash
git add internal/helm/renderer_sdk.go internal/helm/oci-generator.go test/golden/oci_test.go
git commit -m "feat: render OCI charts through the embedded helm too"
```

---

### Task 6: Docker image bez helmu a kubectlu

Odměna za fáze 4 a 6 dohromady.

**Files:**
- Modify: `Dockerfile`
- Modify: `README.md`

- [ ] **Step 1: Zúžit `apk add`**

`Dockerfile:10` dnes: `RUN apk add git helm kubectl --no-cache`.
Zůstane `git`. Helm i kubectl jsou od téhle fáze potřeba jen tomu, kdo si
externí backend vyžádá — a ten si je do image doinstaluje sám, nebo použije
vlastní.

Doplň nad ten řádek komentář, který říká **proč** tam ty dva nástroje nejsou,
ať je někdo „opravou" nevrátí.

- [ ] **Step 2: Ověř, že image funguje**

```bash
task docker-build
```
Pak v kontejneru vyrenderuj scénář, který používá helm i kustomize, a porovnej
s tím, co dá lokální binárka. Do reportu napiš **naměřenou velikost image před
a po** (`docker images`).

- [ ] **Step 3: Srovnej README**

Sekce o Dockeru a o předpokladech nesmí dál tvrdit, že image obsahuje helm a
kubectl, ani že jsou potřeba.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile README.md
git commit -m "build: drop helm and kubectl from the image, both are embedded now"
```

---

### Task 7: Dokumentace, uzavření fáze a zbylá mezera z fáze 5

**Files:**
- Modify: `docs/specification.md`, `CLAUDE.md`, `AGENTS.md`, `README.md`
- Modify: `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`
- Test: `cmd/generate_test.go`

- [ ] **Step 1: Zapiš helm do matice podpory**

`docs/specification.md`, sekce 5 dnes popisuje rozdělení jen pro kustomize.
Doplň helm do téže struktury: co umí jen externí cesta (**pluginy a
post-renderery — natrvalo**), jaká verze se použije na které cestě, a jak se
backend vybírá. Sekce 6 (výjimky z parity) dostane rozdíl v OCI autentizaci
z úlohy 5.

Nepiš žádné tvrzení o paritě, které nemá za sebou test z úlohy 3 nebo naměřený
výsledek z úlohy 5. **Čtyři dokumentační opravy v tomhle projektu už zavedly
novou nepravdu** — každou větu ověř proti kódu nebo testu.

- [ ] **Step 2: Srovnej `CLAUDE.md`, `AGENTS.md` a `README.md`**

Všechny tři popisují `KRMGEN_HELM_EXECUTABLE` a předpoklady běhu. Po téhle fázi
platí totéž co pro kubectl od fáze 4: helm není potřeba, dokud si ho uživatel
nevyžádá. Zkontroluj i architektonickou mapu v `CLAUDE.md` — přibyly tři soubory.

- [ ] **Step 3: Uzavři fázi v design docu**

Řádek `| 6 | helm → SDK | ... |` v tabulce fází dostane výsledek včetně
naměřených čísel (63,7 MB, 286 modulů, image −76 MB). Zapiš i to, že
`stripHelmBanner` zůstává živý pro externí cestu — jinak to bude vypadat jako
mrtvý kód a někdo ho smaže.

- [ ] **Step 4: Zavři mezeru z fáze 5, pokud to jde bez ohýbání kódu**

Fáze 5 zavedla varování na stderr, když se nepodaří smazat pracovní adresář
(`cmd/generate.go`), a **nechala ho bez testu** — spolehlivě to vyvolat chce
read-only přípojný bod. Zkus to zavřít: nabízí se vytáhnout to volání
`os.RemoveAll` do proměnné balíčku jako šev, po vzoru `runCommand` v
`internal/helm/processor.go`, a v testu ho nahradit funkcí vracející chybu.

Pokud to jde takhle čistě, udělej to. **Pokud by to znamenalo přestavovat
produkční kód kvůli testu, nech to být a napiš proč** — je to zapsaná mezera,
ne skrytá vada.

- [ ] **Step 5: Závěrečná kontrola**

```bash
golangci-lint run ./...
go test -race -count=1 ./...
gofmt -l . && go vet ./...
git diff --stat -- test/golden/fixtures/
```
Expected: 0 issues, vše PASS, poslední dva příkazy prázdné.

- [ ] **Step 6: Commit**

```bash
git add docs/ CLAUDE.md AGENTS.md README.md cmd/
git commit -m "docs: record the helm backend split and close phase 6"
```

---

## Co tahle fáze vědomě nedělá

- **Neodstraňuje externí cestu.** `KRMGEN_HELM_EXECUTABLE` je natrvalo
  podporovaná volba (R1), a pluginy s post-renderery jinak než přes ni nejdou.
- **Nezvyšuje verzi helmu.** v4.2.4 je verze, proti které jsou ukotvené goldeny.
  Upgrade je samostatná práce s vlastním měřením parity.
- **Nemění, co krmgen vyrenderuje.** Jediné povolené změny výstupu jsou ty,
  které projdou úlohou 3 jako shoda, a případná zapsaná výjimka z úlohy 5.
- **Neřeší `cmd/only-test/run.go`.** Vývojová utilita mimo produkt.

## Sebekontrola plánu

- **Pokrytí:** rozhodnutí jít/nejít je doložené měřením (výše), rozhraní (úloha 1),
  embedded implementace (2), důkaz parity (3), přepnutí výchozí cesty (4), OCI (5),
  image (6), dokumentace a zbylá mezera z fáze 5 (7).
- **Typová konzistence:** `Renderer.Render(cfg *types.HelmChart, g generator, workDir string) (string, error)` je zavedená
  v úloze 1 a stejně konzumovaná v úlohách 2, 4 a 5. `newBinaryRenderer` dostává
  v úloze 4 parametr s cestou — v úloze 1 je bezparametrický, což je jediná
  změna podpisu napříč fází a je záměrná.
- **Pořadí:** každá úloha končí přeložitelným stromem se zelenou sadou. Výchozí
  backend se přepíná až v úloze 4, tedy až po naměřené paritě v úloze 3.
- **Nepokryté riziko:** OCI parita se neměří v CI, protože sahá na síť. Úloha 5
  ji měří ručně a zapisuje výsledek — vědomý kompromis, ne opomenutí.
