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

**Předpoklad k potvrzení:** cesta modulu se sjednotí z `github.com/librucha/cgt` na
`github.com/librucha/cloud-go-templates`. Před prvním commitem je to zdarma.

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
| 2 | Golden-master + unit testy | testovací síť | goldeny zelené, pokrytí ~80 % |
| 3 | Knihovna šablon ven | `cloud-go-templates` v1 | goldeny beze změny |
| 4 | kustomize → krusty | `Builder` + 2 impl. | goldeny beze změny nebo schválený diff |
| 5 | helm → SDK | `Renderer` + 2 impl. | naměřená parita → rozhodnutí jít/nejít |

Fáze 5 je jediná s reálnou možností „ne". Rozhodne se podle naměřené parity, ne dopředu.

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

### Kvalita kódu — přiřazení k fázím

| Nález | Fáze |
|---|---|
| `log.Fatal` na 27 místech → návratové chyby (v knihovně nepřípustný) | 3 |
| Dva loggery: stdlib `log` v 6 souborech, logrus v jednom | 4 |
| `os.ModePerm` (0777) a `0666` na souborech s vyrenderovanými secrets | 4 |
| Při `log.Fatal` v `processWorkDir` se nespustí `defer os.RemoveAll` → temp adresář se secrets zůstane na disku | 3 |
| `KRMGEN_KUBECTL_EXECUTABLE` deklarovaná, dokumentovaná, nikde nepoužitá | 4 (R1 ji zavádí doopravdy) |
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

## 6. Rizika a vědomé změny chování

| Riziko | Ošetření |
|---|---|
| Jiná verze kustomize v krusty než v `kubectl` → jiný výstup | goldeny ukážou diff, schvaluje se ručně |
| Helm SDK nenastaví výchozí `KubeVersion` a API verze jako CLI → charty s `.Capabilities` tiše změní výstup | explicitní dorovnání ve fázi 5, vlastní golden scénář |
| Reimplementace registry auth, OCI pull a repo indexu pro SDK | za rozhraním; při neúspěchu se fáze 5 nenasadí |
| Změna výchozího backendu (R2) je breaking change | release notes, major verze |
| Banner z Helmu v4 na stdout | strip zůstává natrvalo pro externí cestu |

## 7. Otevřené otázky

1. Potvrdit cestu modulu `github.com/librucha/cloud-go-templates` (R4)
2. Rozsah matice podporovaných verzí: jen helm v3 + v4, nebo i starší?
3. Přejmenovat `azUaIdClientId` → `azClientId` s aliasem?
