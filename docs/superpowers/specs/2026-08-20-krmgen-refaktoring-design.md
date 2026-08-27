# krmgen — strategie refaktoringu

*Datum: 2026-08-20 · Stav: k revizi*

## 1. Kontext

krmgen je CLI generující KRM YAML z Helm chartů a Kustomize konfigurací. Dnes deleguje
renderování na externí binárky `helm` a `kubectl` a šablonovací funkce (Azure Key Vault,
ArgoCD env, …) má zadrátované ve svém `internal/`.

Bezprostředním podnětem byla chyba, u které stojí za to se zastavit, protože určuje směr
celého návrhu: Helm v4 píše u OCI chartů banner `Pulled:` / `Digest:` na **stdout**, ne na
stderr. krmgen ten stdout ukládal beze změny pro kustomize, takže soubor začínal dvěma
řádky, které nejsou YAML → `missing Resource metadata`. V ArgoCD to fungovalo, lokálně ne,
protože každé prostředí mělo jinou verzi helmu.

Kořen problému nebyla chyba v parsování. Byl to **version drift externí binárky**.

### Cíle

1. Vyseparovat šablonovací funkce do samostatné knihovny po vzoru sprig
2. Nahradit volání `helm` a `kubectl` knihovnami
3. Zvýšit kvalitu kódu
4. Mít k produktu specifikaci

### Výchozí stav (měřeno 2026-08-20)

| Metrika | Hodnota |
|---|---|
| Produkční kód | 1 640 řádků |
| Testy | 1 331 řádků |
| Pokrytí | 29,5 % |
| Binárka | 17 MB |
| `log.Fatal` v kódu | 27 výskytů |

## 2. Naměřená data k rozhodnutí o knihovnách

Spike se skutečnými importy, ne odhad:

| Varianta | Binárka | Modulů v grafu |
|---|---|---|
| krmgen dnes | 17 MB | ~40 |
| + `sigs.k8s.io/kustomize/api` (krusty) | 22 MB | 62 |
| + `helm.sh/helm/v4/pkg/action` | 81 MB | 265 |

Velikost binárky je zavádějící metrika. Rozhoduje velikost image, protože primární
nasazení je ArgoCD CMP v kontejneru (odhad, host binárky darwin/arm64):

```
dnes:  alpine 8 + git ~15 + helm 59 + kubectl 59 + krmgen 17  ≈ 158 MB
cíl:   alpine 8 + git ~15 + krmgen ~85                        ≈ 108 MB
```

Embedded varianta je o třetinu **menší** a odstraňuje dva samostatně verzované balíky
s vlastním CVE profilem.

## 3. Rozhodnutí

### R1 — Dva backendy, natrvalo

`KRMGEN_HELM_EXECUTABLE` a `KRMGEN_KUBECTL_EXECUTABLE` zůstávají jako podporovaná funkce:
když je uživatel nastaví, krmgen volá **externí nástroj na hostu**. Když nastavené nejsou,
použije **zabudovanou knihovnu**.

Není to migrační berlička, ale vlastnost produktu — pro devops nástroj je možnost přibít se
na konkrétní nástroj v prostředí legitimní požadavek na stabilitu.

Cena, kterou to nese a kterou přijímáme:
- dva kódové toky navždy, včetně srovnání argumentů mezi knihovnou a CLI
- funkční nerovnost: **helm pluginy a post-renderery jdou jen externí cestou**
- `kubectl kustomize` nese často starší verzi kustomize než embedded krusty

### R2 — Výchozí je embedded

Bez nastavené proměnné jede zabudovaná knihovna. Pro stávající uživatele je to změna
chování (dnes se sáhne po `helm` v `PATH`), ale je to přesně ten bod, kvůli kterému se
přechod dělá. Musí být v release notes jako breaking change.

### R3 — Referencí parity je golden, ne jeden z backendů

Ani externí, ani embedded backend není „ten správný". Oba se měří proti stejným golden
souborům. Kde se shodnout nemohou, je to **sepsaná výjimka ve specifikaci**, ne chyba.

### R4 — Knihovna šablon: samostatné repo, krmgen API

Vzniká v existujícím repu `cloud-go-templates`. Ten dnes obsahuje náčrt, který **nemá
jediný commit, běží na Go 1.20 se závislostmi z prosince 2023 a nepřekládá se**
(`internal/types/types.go:22: declared and not used: method`). Nic tedy nesvazuje.

Náčrt počítal s koordinátovým API `{{ sec "az:vault:secret:ver" }}`. **Nepoužije se.**
Zvolen je styl, který krmgen má dnes: `{{ azSec "vault" "secret" "ver" }}`.

Důvody:
- je v produkci; změna rozbije každý existující `krmgen.yaml`
- Go šablony umí víc argumentů nativně — kódovat je do řetězce s dvojtečkami ruší kontrolu
  arity a koleduje si o kolizi, jakmile se dvojtečka objeví v hodnotě
- celá reflexní vrstva (`FuncDefMap`, `FuncMetadata.CreateFunc`) v náčrtu existuje **jen**
  kvůli dispatchi podle prefixu; s tímto rozhodnutím mizí
- multi-cloud ambici to neblokuje: `azSec` / `awsSec` / `gcpSec` je běžný způsob namespacování

Z náčrtu se přebírá: samotné repo, seskupení po providerech (`FuncMap()` vs `AzureFuncMap()`),
CLI na ad-hoc vyhodnocení šablony a Entra groups/apps jako pozdější přírůstek.

**Rozhodnuto 2026-08-21:** cesta modulu je `github.com/librucha/cloud-go-templates`.

Nešlo o volbu vkusu. Remote toho repozitáře už byl `github.com/librucha/cloud-go-templates`,
zatímco modul se jmenoval `github.com/librucha/cgt` — a Go překládá cestu modulu na URL
repozitáře, takže `go get github.com/librucha/cgt` by nikdy nikomu nefungoval. Přejmenováno
včetně sedmi importů a `MODULE_NAME` v Taskfile; ověřeno, že jediná zbývající chyba překladu
je ta původní (`internal/types/types.go:22`), nyní hlášená pod novou cestou. Zapsáno prvním
commitem toho repozitáře (`93ebeda`).

### R5 — Kvalita kódu se rozpouští do fází, nedělá se zvlášť

Samostatná „fáze úklidu" by se buď odložila donekonečna, nebo by měnila chování bez sítě.
Každá položka se váže na fázi, ve které stejně dochází k zásahu do daného kódu.

## 4. Cílová architektura

```
cloud-go-templates (samostatný modul)
  pkg/            FuncMap(), AzureFuncMap(), …  ← veřejné API
  azure/          azSec, azCert, azKey, azStoreKey, azUaIdClientId
  cli/            ad-hoc vyhodnocení šablony

krmgen
  cmd/            cobra, žádná logika
  internal/
    render/       Renderer  ← rozhraní, 2 implementace
      helmlib/      helm.sh/helm/v4/pkg/action
      helmexec/      externí binárka
    build/        Builder   ← rozhraní, 2 implementace
      krusty/       sigs.k8s.io/kustomize/api
      kubectlexec/  externí binárka
    config/       parsování + validace proti JSON Schema
```

Volba implementace je jediné místo, které čte `KRMGEN_*_EXECUTABLE`. Zbytek kódu vidí
jen rozhraní.

## 5. Fáze

| # | Fáze | Výstup | Brána |
|---|---|---|---|
| 1 | Specifikace kontraktu | spec + JSON Schema | odsouhlasená |
| 2 | Golden-master + unit testy | testovací síť | **hotovo 2026-08-25**: 8 scénářů, pokrytí 71,4 % (viz níže) |
| 3 | Knihovna šablon ven | `cloud-go-templates` v1 | **hotovo 2026-08-25**: goldeny beze změny (viz níže) |
| 4 | kustomize → krusty | `Builder` + 2 impl. | **hotovo 2026-08-26**: goldeny beze změny, parita ověřena testem (viz níže) |
| 5 | Kvalita kódu | `log.Fatal` → návratové chyby, sjednocené stderr, práva souborů | **hotovo 2026-08-27**: viz „Kvalita kódu — přiřazení k fázím" níže |
| 6 | helm → SDK | `Renderer` + 2 impl. | **hotovo 2026-08-27**: rozhodnutí jít — goldeny beze změny, parita ověřena testem, embedded je výchozí (viz níže) |

Fáze 6 je jediná s reálnou možností „ne". Rozhodne se podle naměřené parity, ne dopředu.

### Fáze 1 — co spec pokryje

- **CLI**: `generate <path>`, `--skip`, exit kódy, striktní dělba stdout = YAML / stderr = logy
- **Config**: JSON Schema pro `kind: KrmGen`. Doplní soubor, který dnes hledá zakomentovaná
  validace v `parser.go` a který v repu vůbec neexistuje.
- **Pipeline**: pořadí `template eval → helm → kustomize`, pravidla slučování
- **Šablonovací funkce**: jména, arita, chování při chybě, sémantika cache — stanou se
  veřejným API knihovny, musí být přibité dřív, než se vyseparují
- **Matice podporovaných verzí** externích nástrojů + detekce verze při startu
- **Dokumentované výjimky z parity** mezi backendy
- **Ne-cíle**

### Fáze 2 — výsledek

Dokončeno 2026-08-25, 14 commitů. Pokrytí **36,9 % → 71,4 %**, 8 golden scénářů
(helm sám, kustomize sám, oba, skip patterns, šablonovací funkce, dvě kustomizace,
nepodporované schéma repozitáře, OCI za build tagem `oci`).

Tři věci, které se během fáze ukázaly jinak, než designový dokument předpokládal:

- **Hermetické fixtures z lokálního chartu nejdou.** `newGenerator` přijímá jen `oci://`
  a `http(s)://`, takže chart z adresáře krmgenu nepředáš. Náhrada je lokální HTTP chart
  repozitář z `httptest`; adresa se do fixture dostane přes `argocdEnv "CHART_REPO"`.
- **OCI hermeticky nejde vůbec.** Helm chce pro HTTP registry `--plain-http`, pro
  self-signed `--insecure-skip-tls-verify`, a krmgen nepředává ani jedno. OCI proto zůstává
  na síťovém testu za build tagem, bez goldenu.
- **Goldeny jsou verzně ukotvené.** Výstup se mezi helmem 3 a 4 liší (prázdný řádek před
  `---`), takže goldeny platí pro jednu referenční dvojici nástrojů, zapsanou
  v `test/golden/versions_test.go`, a harness verzi kontroluje před porovnáním. Matice
  podpory 3.8+/4.x tím není dotčená — popisuje, co krmgen podporuje, ne proti čemu vznikly
  artefakty.

CI bylo od května 2026 červené (`make build` bez `Makefile`) a nespouštělo se na pull
requestech. Obojí opraveno; bez toho by síť nehlídala nic.

### Fáze 2 — testovací strategie

Tři vrstvy:

1. **Čisté funkce** — bez I/O, přímé unit testy
2. **Balíčkové s fake runnerem** — chybové větve, které přes E2E nejdou rozumně vyvolat
3. **Diferenciální golden-master** — každý fixture se pustí **oběma backendy** a musí padnout
   na stejný golden

Třetí vrstva je jádro. Hermetická: lokální chart fixtures, **žádná síť** — helm i SDK umí
renderovat chart z adresáře. Síťová OCI cesta zůstane na pár testech za build tagem.
Flag `-update` na regeneraci goldenů.

Scénáře: helm sám, kustomize sám, oba, skip patterns, šablonovací funkce, chybové cesty
(rozbitý config, dvě kustomizace). Azure zůstává na unit testech s mock transportem
(vzor už existuje v `azsec`) — do goldenů se netahá.

Determinismus: dnešní kustomize cesta generuje soubor s náhodným UUID. Do výstupu
neprosakuje, ale test to musí explicitně hlídat.

CI dnes testy vůbec nespouští, jen `make build`. Přibude `go test -race ./...` a instalace
helmu a kubectl, protože externí backend je podporovaná cesta a musí se testovat.

### Fáze 3 — výsledek

Dokončeno 2026-08-25. Azure šablonovací funkce (`azSec`, `toPem`, `azPfxKey`,
`azPfxCrt`, `azCert`, `azKey`, `azStoreKey`, `azUserIdentityClientId`) se
přestěhovaly do `github.com/librucha/cloud-go-templates` (balíček `azure`);
krmgen je bere jako závislost přes `replace` na lokální adresář a slučuje
jejich `FuncMap()` do vlastní registrace funkcí v
`internal/template/template.go`.

Z krmgenu odešlo **1504 řádků**, přibylo 67 (přepojení v `template.go` a
smazání celého podstromu `internal/template/azure/...`) — čistý úbytek přes
1400 řádků. Pokrytí:

- krmgen: **73,9 %** celkem přes celý modul — vzrostlo z 71,4 % (fáze 2),
  protože odstraněné Azure balíčky měly pokrytí 61–77 %, tedy pod průměrem
  zbytku; jejich odchodem se průměr zvedl. Balíček `internal/template` sám
  o sobě naopak klesl, 96,4 % → 87,9 % — `azureFuncs()`/`sync.Once` obálka
  má chybovou větev (selhání konstrukce provideru), kterou krmgenovy vlastní
  testy nepokrývají, protože tahle větev je otestovaná v knihovně. Ani jedno
  není regrese, obojí je očekávaný důsledek přesunu kódu.
- `cloud-go-templates`: **85,9 %**

Goldeny beze změny (`git status --porcelain test/golden/fixtures/` prázdný) —
Azure funkce goldeny nepokrývají, takže jedinou pojistkou proti rozbité
registraci je nový test `TestEvalGoTemplates_RegistersEveryDocumentedFunction`
(`internal/template/template_test.go`), který přes `{{ if false }}{{ <jméno> }}{{ end }}`
ověří, že je zaregistrované každé zdokumentované jméno včetně deprecated
aliasu `azUaIdClientId`.

**Vědomé změny chování — čtyři, ne jedna.** Tenhle záznam původně tvrdil
"jedinou vědomou změnu chování"; revize fáze 3 to opravila, protože to
neplatilo. Plné znění a testy pro všechny čtyři jsou v
`docs/specification.md` (sekce 4, "Error behaviour of unusual Azure
responses"); shrnutí:

1. `azStoreKey` s prázdným `Keys` (schváleno v původním plánu) — původní kód
   indexoval první storage account key bez kontroly délky pole, takže účet
   bez klíčů shodil celý proces; knihovna vrací chybu
   (`cloud-go-templates/azure/storage.go`,
   `TestStorageKeyFunc_NoKeysIsAnErrorNotAPanic`).
2. `azStoreKey` s `Keys[0].Value == nil` — dřív panika, teď chyba
   (`TestStorageKeyFunc_NilFirstKeyValueIsAnError`).
3. `azSec` s `secret.Value == nil` — dřív panika, teď chyba
   (`TestSecret_NilValueIsAnError`).
4. `azUserIdentityClientId` (`azUaIdClientId`) — `Properties == nil` byla
   panika, teď je to chyba; **a `ClientID == nil`, který se dřív vyrenderoval
   jako `<nil>`, je teď tvrdá chyba.** Tohle je jediná ze čtyř změn, která
   mění výstup úspěšného renderu, ne jen chování paniky — původní tvrzení
   plánu "výstup se nemění" bylo pro tenhle nil-`ClientID` případ chybné (viz
   `docs/specification.md`, kde je to opravené).

Všechny čtyři jsou vylepšení oproti pádu procesu a žádná se nevrací zpět.

`azUaIdClientId` bylo přejmenováno na `azUserIdentityClientId` (staré jméno
zůstává jako deprecated alias v krmgenu, knihovna sama exponuje jen nové) —
narovnáno v `CLAUDE.md` a `docs/specification.md`.

Rozhodnutí, které zbývá uživateli: `replace` v `go.mod` krmgenu ukazuje na
lokální adresář `../cloud-go-templates`. Dokud knihovna nebude pushnutá a
otagovaná (rozhodnutí uživatele, implementeři nepushují), krmgen nejde
postavit nikde jinde než na tomto stroji.

### Fáze 4 — výsledek

Dokončeno 2026-08-26. `internal/kustomize` dostal rozhraní `Builder`
(`builder.go`) se dvěma implementacemi: `krustyBuilder`
(`builder_krusty.go`), postavená na `sigs.k8s.io/kustomize/api` v0.21.1
pinnuté v `go.mod`, a `kubectlBuilder` (`builder_kubectl.go`), která dál
spouští `kubectl kustomize` jako subprocess. `selectBuilder()` volí podle
`KRMGEN_KUBECTL_EXECUTABLE` — to naplnilo R1 i R2 zároveň: obě cesty
existují natrvalo (R1) a výchozí je od tohoto commitu embedded (R2).
`KRMGEN_KUBECTL_EXECUTABLE`, dřív deklarovaná a nikde nečtená, je teď
skutečně jediné místo, které o volbu backendu rozhoduje.

Parita mezi backendy je naměřená, ne předpokládaná:
`TestGolden_BothBackendsAgree` (`test/golden/harness_test.go`) pustí každý
scénář, který kustomize vyrenderuje úspěšně (`kustomize-only`,
`helm-with-kustomize`, `kustomize-features`), oběma backendy a vyžaduje
bajtově identický stdout; `TestGolden_BothBackendsAgreeOnErrors` totéž udělal
pro dvě chybové cesty, které backend vůbec dosáhnou
(`multi-config-kustomize`, `nested-kustomization`), kde bajtová shoda nejde —
stderr nese cestu do dočasného adresáře a externí backend chybu z kustomize
obaluje jinak ("run kubectl kustomize failed: ..." vs. "run kustomize
failed: ..."). Tam se porovnává exit kód a stabilní podřetězec stderr. Oba
backendy hlásí stejnou podkladovou chybu kustomize v obou scénářích — žádný
rozdíl nebyl potřeba zapisovat jako výjimku.

Třetí chybový scénář, `two-kustomizations`, do tohoto diferenciálního testu
záměrně nepatří: `FindKustomizeFile` (`internal/kustomize/processor.go`)
selže na nalezení více kustomization souborů dřív, než `BuildKustomize` vůbec
zavolá `selectBuilder()`, takže tenhle scénář se k žádnému backendu nikdy
nedostane a o paritě backendů nic nevypovídá. Pokrývá ho
`TestError_TwoKustomizations` (`test/golden/errors_test.go`).

Goldeny beze změny (`git diff adde3be..HEAD --stat -- test/golden/fixtures/`
ukazuje jen nový scénář `kustomize-features` z úlohy 1, žádný existující
golden se nepohnul).

Jediná zaznamenaná odchylka mezi backendy zůstává verze kustomize — externí
cesta renderuje tím, co embeduje nainstalovaný kubectl (naměřeno v této fázi:
kubectl v1.36.3 / Kustomize v5.8.1), embedded cesta pinnutou verzí v
`go.mod`. Zapsáno v `docs/specification.md`, sekce 5 a 6.

### Kvalita kódu — přiřazení k fázím

| Nález | Fáze |
|---|---|
| `log.Fatal` na 14 místech v produkčním kódu (mimo `cmd/only-test`, vývojovou utilitu; naměřeno 16 celkem) → návratové chyby (v knihovně nepřípustný) | **hotovo 5** (`internal/kustomize/processor.go`, `internal/helm/oci-generator.go`, `internal/helm/processor.go`, `cmd/generate.go` a `krmgen.go` teď vrací chyby; `cmd.NewRootCommand(...).Execute()` je nechá vytisknout cobrou a `main` skončí `os.Exit(1)`. `grep -rn 'log\.Fatal' --include=*.go . \| grep -v _test \| grep -v cmd/only-test` je prázdný) |
| Dva loggery: stdlib `log` v 6 souborech, logrus v jednom | **hotovo 5** (`github.com/sirupsen/logrus` odstraněn z `go.mod`/`go.sum` úlohou 7 — `go mod tidy` ho smazal úplně, nezůstal ani jako nepřímá závislost. Zůstal jen stdlib `log`: informativní `log.Println` v `internal/config/parser.go` a dva `log.Fatal` ve vývojové utilitě `cmd/only-test/run.go`, mimo produkt) |
| `os.ModePerm` (0777) a `0666` na souborech s vyrenderovanými secrets | **hotovo 5** (`internal/utils/perm.go` zavádí `FilePerm 0600` / `DirPerm 0700`, použité v `cmd/generate.go`, `internal/kustomize/processor.go` a `internal/helm/processor.go`) |
| Při `log.Fatal` v `processWorkDir` se nespustí `defer os.RemoveAll` → temp adresář se secrets zůstane na disku | **hotovo 5** (`generate` v `cmd/generate.go` registruje `defer os.RemoveAll(workDir)` hned po vytvoření adresáře, takže poběží na každé cestě ven včetně chyby uprostřed zpracování; dokázáno `TestError_WorkingDirectoryRemovedOnFailure` v `test/golden/errors_test.go`) |
| `KRMGEN_KUBECTL_EXECUTABLE` deklarovaná, dokumentovaná, nikde nepoužitá | **hotovo 4** (R1 ji zavedla doopravdy — `internal/kustomize/builder.go`, `selectBuilder`) |
| Zakomentovaná validace schématu ukazuje na neexistující soubor | 1 |
| Překlepy v chybových hláškách (`failerd`, `crating`, `unwraping`) | průběžně |

Dopad práv souborů je omezený tím, že `os.MkdirTemp` vytvoří adresář s 0700 — není to díra,
je to nedbalost.

### Rozpory dokumentace a kódu k narovnání ve fázi 1

| `CLAUDE.md` tvrdí | Kód dělá |
|---|---|
| `azClientId <sub> <group> <name>` | funkce je `azUaIdClientId`, bere 2 argumenty (`rgName`, `idName`) |
| `azStoreKey <account> <group>` | bere 3 argumenty (`subscriptionID`, `resourceGroup`, `accountName`) |
| `KRMGEN_KUBECTL_EXECUTABLE` funguje | není nikde použitá |

Spec rozhodne, která strana se přizpůsobí. U `azUaIdClientId` je k úvaze přejmenování na
`azClientId` s ponecháním starého jména jako alias.

### Fáze 6 — výsledek

Dokončeno 2026-08-27, rozhodnutí **jít**. `internal/helm` dostal rozhraní
`Renderer` (`renderer.go`) se dvěma implementacemi: `sdkRenderer`
(`renderer_sdk.go`), postavená na `helm.sh/helm/v4/pkg/action` v4.2.4
pinnuté v `go.mod`, a `binaryRenderer` (`renderer_binary.go`), která dál
spouští `helm template` jako subprocess. `selectRenderer()` volí podle
`KRMGEN_HELM_EXECUTABLE` stejným vzorem jako `selectBuilder()` ve fázi 4
(prázdný řetězec = nenastaveno) a od úlohy 4 této fáze je výchozí embedded —
to naplnilo R1 i R2 pro helm stejně, jako to fáze 4 udělala pro kustomize.

Parita je naměřená, ne předpokládaná: `TestGolden_BothHelmRenderersAgree`
(`test/golden/harness_test.go`) pustí sedm scénářů, které helm chart
vyrenderují (`helm-hooks`, `helm-only`, `helm-with-kustomize`,
`nested-kustomization`, `skip-patterns`, `template-functions`,
`values-file`), oběma backendy a vyžaduje bajtově identický stdout a shodné
exit kódy. `oci-public` má vlastní síťový sourozenec za build tagem `oci` —
`TestOci_BothHelmRenderersAgree` (`test/golden/oci_test.go`) — protože
hermetická sada nesmí sahat na síť. `bad-repo-scheme` do žádného z testů
nepatří: selže v `newGenerator` dřív, než se vůbec vybere renderer, takže o
paritě backendů nic nevypovídá; pokrývá ho `errors_test.go`. Goldeny beze
změny po celou fázi (`git diff --stat -- test/golden/fixtures/` prázdný na
konci každé úlohy).

Skutečný nalezený bug, ne jen golden shoda: první pokus o OCI cestu předával
`ociHelmGenerator.chartId()` (repo URL + `"/"` + jméno chartu) jako referenci
chartu pro `LocateChart`. Proti reálnému `oci-public` fixture
(`oci://ghcr.io/stakater/charts/reloader`, jméno `reloader`) to zdvojilo
poslední segment cesty a 404lo na ghcr.io. Oprava: předat `cfg.RepoUrl`
samotné, přesně jak to dělá `binaryRenderer` (`ociHelmGenerator.addRepoArgs`).
Nalezeno až proti skutečné síti (úloha 5) — hermetické testy ho neodhalily,
protože `chartId()` se jinde v produkčním kódu používá jen na odvození
registry hostu pro `helm registry login`, kde je zdvojené jméno neviditelné.

**OCI autentizace se mezi backendy liší mechanismem, ne jen voláním.**
Externí binárka spouští `helm registry login` jako subprocess, který
credentials zapíše na disk do helmova registry configu — tam přežijí i po
skončení krmgenu a sebere je jakékoli další `helm`. Embedded cesta předává
stejné credentials `registry.NewClient` přes `registry.ClientOptBasicAuth`,
drží je jen v paměti po dobu jednoho renderu a na disk nic nezapisuje.
Vyrenderovaný výstup se neliší (`TestOci_BothHelmRenderersAgree` prochází),
liší se jen kde credentials skončí. Zapsáno jako výjimka v
`docs/specification.md`, sekce 5 a 6.

**Riziko `.Capabilities` (viz sekce 6 níže) se ukázalo jako vyřešené
konstrukcí, ne jako něco, co potřebovalo vlastní golden scénář.** Krmgen na
žádné cestě nepředává `--kube-version`/`--api-versions`, takže `KubeVersion`/
`APIVersions` zůstávají na `action.Install` nenastavené na obou cestách;
protože `helm template` (`pkg/cmd/template.go`) i `sdkRenderer.Render` volají
tutéž `RunWithContext` (`pkg/action/install.go`), ne dvě nezávislé
implementace, defaulty se shodují automaticky, dokud je verze helmu stejná.
Ověřeno čtením zdrojů `helm.sh/helm/v4@v4.2.4`, ne testem — žádný demo chart
nečte `.Capabilities`.

**Velikost.** Plánovaná čísla (spike z 2026-08-20, výše) byla odhad na
darwin hostu; skutečné naměřené hodnoty (úloha 6, po ořezání
`goreleaser`/Dockerfile) je třeba číst takhle:

| platforma | binárka před | binárka po | násobek |
|---|---|---|---|
| linux/amd64 | 19,0 MB | 49,1 MB | 2,6× |
| linux/arm64 | 17,6 MB | 45,7 MB | 2,6× |
| darwin/arm64 | 20,7 MB | 63,6 MB | 3,1× |

`goreleaser` staví primárně pro Linux, takže distribuovaná cena je **2,6×**,
ne 3,1×; 63,7 MB (tento plán, výše, i dřívější verze tohoto dokumentu) je
darwin číslo, ne cena, kterou platí uživatel binárky nebo Docker image.
Modulů v grafu: 103 → 286. Docker image (`librucha/krmgen`, `apk add git
helm kubectl` → `apk add git`): **283 MB → 97,2 MB (−66 %)**, ne −76 MB, jak
plán odhadoval — alpine balíčky `helm`+`kubectl` (159 MB nekomprimovaně)
váží víc, než plán počítal, takže úspora z jejich odstranění je větší, ne
menší, než odhad; kompenzuje ji zase o něco větší vlastní binárka. Embedded
varianta je pořád výrazně menší, jen ne v poměru, který plán čekal.

`stripHelmBanner` (`internal/helm/renderer_binary.go`) zůstává živý — je to
vlastnost jen externí cesty (embedded knihovna banner na stdout nikdy
nepíše), ne mrtvý kód z doby před přechodem. Neodstraňovat.

## 6. Rizika a vědomé změny chování

| Riziko | Ošetření |
|---|---|
| Jiná verze kustomize v krusty než v `kubectl` → jiný výstup | goldeny ukážou diff, schvaluje se ručně |
| Helm SDK nenastaví výchozí `KubeVersion` a API verze jako CLI → charty s `.Capabilities` tiše změní výstup | **hotovo 6**, jinak než plán čekal: nevzniklo vlastní golden, riziko odpadlo konstrukcí — SDK i CLI volají tutéž `RunWithContext`, ne dvě implementace (viz „Fáze 6 — výsledek" výše) |
| Reimplementace registry auth, OCI pull a repo indexu pro SDK | **hotovo 6** — `ociHelmGenerator.registryClient` + `client.SetRegistryClient`, měřeno proti reálné síti (`TestOci_BothHelmRenderersAgree`); OCI autentizace se přesto liší mechanismem mezi backendy, zapsáno jako výjimka (viz výše) |
| Změna výchozího backendu (R2) je breaking change | release notes, major verze |
| Banner z Helmu v4 na stdout | strip zůstává natrvalo pro externí cestu |

## 7. Otevřené otázky

Před fází 3 (z fáze 2):

A. Zůstane `test/golden` jako umístění scénářů, nebo se sloučí s `test/resources`?
B. Balit demo chart za běhu testu (dnešní volba, vyžaduje helm i pro jednotkový běh),
   nebo commitnout hotový tarball?


1. ~~Potvrdit cestu modulu (R4)~~ — **vyřešeno 2026-08-21**, viz R4
2. ~~Rozsah matice podporovaných verzí~~ — **vyřešeno 2026-08-21**: spodní hranice je
   helm **3.8.0**, kde se OCI stalo GA. Na 3.7.2 krmgen měřitelně selže (`HELM_EXPERIMENTAL_OCI`),
   na 3.8.2 projde end-to-end. helm 2 se nepodporuje (Tiller, EOL 11/2020). U kubectl hranici
   neurčuje krmgen, ale kustomizace uživatele — krmgen volá jen `kubectl kustomize`, dostupné
   od 1.14. Zapsáno v sekci 5 specifikace.
3. ~~Přejmenovat `azUaIdClientId`~~ — **vyřešeno 2026-08-21**: nové jméno je
   `azUserIdentityClientId`, ne `azClientId`. Důvod je symetrie s budoucím
   `azSystemIdentityClientId`: obě identity vrací stejnou trojici (client/principal/tenant ID),
   ale adresují se různě (`Get(rg, name)` vs `GetByScope(scope)`, a system-assigned klient
   nebere subscription), takže jedna funkce pro obojí by musela rozlišovat podle počtu
   argumentů. Krmgen si staré jméno ponechá jako deprecated alias, knihovna vystaví jen nové.
   Zapsáno v sekci 4 specifikace.
