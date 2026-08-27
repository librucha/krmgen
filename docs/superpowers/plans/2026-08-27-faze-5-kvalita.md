# Fáze 5 — kvalita: vracené chyby, jeden logger, práva, úklid temp adresáře

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Odstranit `log.Fatal` z produkčního kódu, sjednotit logování, přestat
psát vyrenderované secrets do souborů s právy `0777`/`0666` a zajistit, že se
dočasný adresář smaže i při chybě — beze změny toho, co krmgen vyrenderuje.

**Architecture:** Chyby se přestanou hlásit voláním `os.Exit` uvnitř knihovních
balíčků a začnou bublat návratovou hodnotou až do cobry, která je vypíše a
ukončí proces. Tím zmizí důvod pro druhý logger i pro testovací lešení
`captureFatal`, a `defer os.RemoveAll(workDir)` konečně doběhne, protože ho
nikdo nepřeskočí přes `os.Exit`.

**Tech Stack:** Go 1.26.3, cobra, sigs.k8s.io/kustomize/api. Žádná nová
závislost; naopak `github.com/sirupsen/logrus` z produkčního kódu odchází.

**Spec:** [`docs/specification.md`](../../specification.md) — produktový kontrakt.
Sekce 1 (CLI) popisuje exit kódy a chybový výstup; tuto fázi je nutné do ní
promítnout, protože formát stderr se **záměrně mění**.

**Design:** [`docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`](../specs/2026-08-20-krmgen-refaktoring-design.md)

---

## Kontext: proč tahle fáze a proč právě teď

Fáze 4 uzavřela výměnu kustomize za knihovnu. Do fáze 5 se přitom postupně
odložily čtyři položky kvality, které se všechny točí kolem téhož: krmgen se
chová jako skript, ne jako knihovna.

Design doc mluvil o **27** místech s `log.Fatal`. Skutečnost naměřená před
psaním tohoto plánu je **16**, z toho **14 v produkčním kódu** (zbylé dvě jsou
v `cmd/only-test/run.go`, což je vývojová utilita, kterou goreleaser nesestavuje
— `.goreleaser.yaml` má `main: .`). Rozložení:

| Soubor | `log.Fatal*` |
|---|---|
| `internal/kustomize/processor.go` | 10 |
| `cmd/generate.go` (přes seam `fatalf`) | 1 volání, ale 9 míst ho používá |
| `internal/helm/processor.go` | 1 |
| `internal/helm/oci-generator.go` | 1 |
| `krmgen.go` | 1 (`log.Fatal(err)` na výsledku `Execute()`) |
| `cmd/only-test/run.go` | 2 (mimo produkt, neřešíme) |

### Co se tím rozbije, když si nedáme pozor

**Formát stderr se změní a je to vidět.** Dnes existují dva:

```
time="2026-08-27T00:23:33+02:00" level=fatal msg="found multiple kustomization files under: /tmp/krmgen428709607"
2026/08/27 00:23:33 processing config file /tmp/krmgen136117636/krmgen.yaml failed error: helm repo "ftp://..." is not supported by any generator
```

První je logrus z `internal/kustomize/processor.go`, druhý stdlib `log`.
Po této fázi bude jeden, cobří:

```
Error: found multiple kustomization files under: /tmp/krmgen428709607
```

To je **změna chování** a patří do specifikace jako záměrná, ne jako odchylka.

**Bezpečnostní síť to unese.** `test/golden/errors_test.go` neasertuje celé
řádky, ale stabilní podřetězce (`"multiple kustomization files"`,
`"not supported by any generator"`, `"already registered id"`,
`"unable to find one of 'kustomization.yaml'"`). Ty musí přežít beze změny —
jsou to jediné body, kde je chybový kontrakt skutečně přibitý. Exit kód 1 musí
přežít taky.

**Testovací lešení zmizí.** `captureFatal` v
`internal/kustomize/processor_test.go:78` existuje jen proto, že produkční kód
volá `os.Exit` — přesměrovává `logrus.StandardLogger().ExitFunc` na panic, aby
`log.Fatalf` shodil volanou funkci místo testovací binárky. Jakmile funkce
vrací chybu, je celý ten trik zbytečný. Jeho odstranění je měřitelný důkaz, že
fáze splnila účel.

## Global Constraints

- Go 1.26.3; veškerý kód, komentáře a dokumentace **anglicky** (tento plán a
  design doc jsou česky — to je správně, jsou to pracovní dokumenty)
- **Ani jeden golden se nesmí regenerovat.** Nikdy nespouštět test s `-update`.
  Fáze nemění, co krmgen vyrenderuje; jediná povolená změna výstupu je formát
  stderr při chybě.
- Žádný test nesmí na síť (OCI testy zůstávají za build tagem `oci`)
- Exit kód při chybě zůstává **1**
- Stabilní podřetězce, na kterých stojí `test/golden/errors_test.go`, musí
  zůstat v chybovém textu doslova
- Po každé úloze musí projít: `golangci-lint run ./...`, `go test -race -count=1 ./...`,
  `gofmt -l .`, `go vet ./...`
- Commituje se výčtem cest, nepushuje se

## Struktura souborů

| Soubor | Odpovědnost po fázi |
|---|---|
| `krmgen.go` | jen `os.Exit(1)` na chybu z `Execute()`; nic netiskne (tiskne cobra) |
| `cmd/root.go` | `SilenceUsage: true`, aby běhová chyba nevysypala nápovědu |
| `cmd/generate.go` | `RunE` místo `Run`; `copySrcDir`/`copyDir`/`processWorkDir` vrací chybu; seam `fatalf` mizí; vlastnictví temp adresáře na jednom místě |
| `internal/kustomize/processor.go` | `FindKustomizeFile`/`BuildKustomize`/`prepareKustomizeFile` vrací chybu; logrus pryč |
| `internal/kustomize/processor_test.go` | `captureFatal` pryč, testy asertují vrácenou chybu |
| `internal/helm/processor.go`, `oci-generator.go` | `helmExecutable`/`login` vrací chybu |
| `internal/utils/perm.go` (nový) | pojmenované konstanty práv pro soubory a adresáře s citlivým obsahem |
| `docs/specification.md` | nová sekce o chybovém výstupu; záznam změny formátu stderr a práv |

**Pořadí úloh je dané závislostmi:** listy stromu (kustomize, helm) první,
`cmd` až potom, protože až tam se chyby slévají. Práva a úklid temp adresáře
až po `cmd`, protože oba potřebují, aby už chyby bublaly.

---

### Task 1: Přibít chybový kontrakt, než se ho dotkneme

Než se změní jediné `log.Fatal`, musí existovat test, který selže, kdyby fáze
změnila *které* chyby se hlásí nebo *s jakým exit kódem* — ne jen ty čtyři
cesty, které dnes `errors_test.go` náhodou pokrývá.

**Files:**
- Modify: `test/golden/errors_test.go`

**Interfaces:**
- Consumes: `runScenario`, `runBinary`, `result` z `test/golden/harness_test.go`
- Produces: nic, co by další úlohy volaly

- [ ] **Step 1: Napiš tabulkový test chybového kontraktu**

Do `test/golden/errors_test.go` přidej:

```go
// TestError_Contract pins what every failing scenario must keep doing across
// the phase-5 refactoring: exit with code 1, name the cause in stderr, and
// (where nothing has been emitted yet) leave stdout empty. The message format
// around the substring is free to change - the substring is not.
func TestError_Contract(t *testing.T) {
	cases := []struct {
		scenario     string
		stableSubstr string
		wantEmptyOut bool
	}{
		{scenario: "two-kustomizations", stableSubstr: "multiple kustomization files", wantEmptyOut: true},
		{scenario: "bad-repo-scheme", stableSubstr: "not supported by any generator", wantEmptyOut: true},
		{scenario: "nested-kustomization", stableSubstr: "unable to find one of 'kustomization.yaml'", wantEmptyOut: true},
		// multi-config-kustomize has already written the first config's block
		// to stdout by the time it fails - see the deviation in the specification.
		{scenario: "multi-config-kustomize", stableSubstr: "already registered id", wantEmptyOut: false},
	}

	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			res := runScenario(t, tc.scenario)
			if res.exitCode != 1 {
				t.Errorf("exit code = %d, want 1", res.exitCode)
			}
			if !strings.Contains(res.stderr, tc.stableSubstr) {
				t.Errorf("stderr = %q, want it to contain %q", res.stderr, tc.stableSubstr)
			}
			if tc.wantEmptyOut && res.stdout != "" {
				t.Errorf("stdout = %q, want nothing on a failure", res.stdout)
			}
			if !tc.wantEmptyOut && res.stdout == "" {
				t.Error("want the partial output that preceded the failure")
			}
		})
	}
}
```

- [ ] **Step 2: Spusť a ověř, že prochází na dnešním kódu**

```bash
go test -race -run TestError_Contract ./test/golden/ -v
```
Expected: PASS, 4 subtesty. Prochází, protože jen popisuje současný stav —
to je záměr. Kdyby neprošel, máš špatně podřetězec, ne kód.

- [ ] **Step 3: Commit**

```bash
git add test/golden/errors_test.go
git commit -m "test: pin the error contract before phase 5 touches it"
```

---

### Task 2: `internal/kustomize/processor.go` — 10 fatálů na vracené chyby

Největší jediný zdroj. Balíček už má `Builder`, který chyby vrací
(`builder_krusty.go`, `builder_kubectl.go`) — `processor.go` je poslední místo
v `internal/kustomize`, které místo toho končí proces.

**Files:**
- Modify: `internal/kustomize/processor.go`
- Modify: `internal/kustomize/processor_test.go`
- Modify: `internal/config/processor.go` (jediný produkční volající)

**Interfaces:**
- Produces:
  - `FindKustomizeFile(workDir string) (string, error)`
  - `BuildKustomize(kustomizeFile string, workDir string, resources string) (string, error)`
  - `prepareKustomizeFile(kustomizeFile string, resourcesFile string, workDir string) error` (neexportované)
- Consumes: `Builder` a `selectBuilder()` z `internal/kustomize/builder.go` beze změny

- [ ] **Step 1: Uprav testy tak, aby čekaly vrácenou chybu (a selhaly)**

V `internal/kustomize/processor_test.go` **smaž celou funkci `captureFatal`**
(řádky 78 a dál) i import `log "github.com/sirupsen/logrus"`. Každé místo,
které dnes vypadá takto:

```go
if !captureFatal(t, func() { FindKustomizeFile(dir) }) {
	t.Error("want a fatal")
}
```

přepiš na:

```go
if _, err := FindKustomizeFile(dir); err == nil {
	t.Error("want an error")
}
```

a u úspěšných cest rozbal druhou návratovou hodnotu:

```go
got, err := FindKustomizeFile(dir)
if err != nil {
	t.Fatalf("FindKustomizeFile: %v", err)
}
```

- [ ] **Step 2: Spusť a ověř, že to neprojde kompilací**

```bash
go test ./internal/kustomize/ 2>&1 | head
```
Expected: chyby překladu typu `assignment mismatch: 2 variables but FindKustomizeFile returns 1 value`.

- [ ] **Step 3: Převeď `FindKustomizeFile`**

```go
// FindKustomizeFile tries to find a kustomization file to build (see
// selectBuilder for which backend does the building). It returns an empty
// path and no error when the directory holds none, and an error when it
// holds more than one or cannot be walked.
func FindKustomizeFile(workDir string) (string, error) {
	var kustomizeFile string
	err := filepath.Walk(workDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		_, ok := allowedFileNames[strings.ToLower(filepath.Base(path))]
		if ok {
			if kustomizeFile != "" {
				return fmt.Errorf("found multiple kustomization files under: %s", workDir)
			}
			kustomizeFile = path
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search kustomize files failed. error: %w", err)
	}
	return kustomizeFile, nil
}
```

Pozor na past: dřív `log.Fatalf` uvnitř `filepath.Walk` proces rovnou ukončil.
Teď se chyba vrací z callbacku a `filepath.Walk` ji propaguje ven — proto ji
`if err != nil` níž **zabalí do "search kustomize files failed"**. Text
`"found multiple kustomization files under: ..."` tak zůstane v řetězci
(`errors_test.go` na něj asertuje `strings.Contains`), jen s prefixem navíc.
Kdyby ti to vadilo, rozliš vlastním sentinel typem — ale kontrakt to nevyžaduje.

- [ ] **Step 4: Převeď `BuildKustomize` a `prepareKustomizeFile`**

Každé `log.Fatalf("...", args...)` nahraď `return fmt.Errorf("...", args...)`
se **zachovaným textem** a `%w` tam, kde se balí `err`. `BuildKustomize` už
volá `selectBuilder().Build(workDir)`, který chybu vrací — jen ji přestaň
polykat a vrať ji dál. Podpis:

```go
func BuildKustomize(kustomizeFile string, workDir string, resources string) (string, error) {
	if kustomizeFile == "" {
		return "", fmt.Errorf("no given kustomizeFile parameter")
	}
	...
	if err := prepareKustomizeFile(kustomizeFile, resourcesFile, workDir); err != nil {
		return "", err
	}
	return selectBuilder().Build(workDir)
}
```

Smaž import `log "github.com/sirupsen/logrus"`.

- [ ] **Step 5: Sroluj volajícího v `internal/config/processor.go`**

`ProcessConfig` už vrací `(string, error)`, takže jde jen o propsání:

```go
kustomizeFile, err := kustomize.FindKustomizeFile(workDir)
if err != nil {
	return "", err
}
if kustomizeFile != "" {
	return kustomize.BuildKustomize(kustomizeFile, workDir, helmOutput)
}
```

- [ ] **Step 6: Spusť testy**

```bash
go test -race -count=1 ./internal/... ./test/golden/
```
Expected: PASS. Golden sada musí projít **beze změny goldenů** — chybový
formát se zatím nemění, protože `cmd/generate.go` pořád fataluje, jen o patro
výš.

- [ ] **Step 7: Commit**

```bash
git add internal/kustomize/processor.go internal/kustomize/processor_test.go internal/config/processor.go
git commit -m "refactor: return errors from the kustomize processor"
```

---

### Task 3: `internal/helm` — dva fatály na vracené chyby

**Files:**
- Modify: `internal/helm/processor.go`
- Modify: `internal/helm/oci-generator.go`
- Modify: `internal/helm/generator.go` (rozhraní `authenticator`)
- Modify: `internal/helm/repo-generator.go` (druhá implementace `login`)
- Test: `internal/helm/processor_test.go`, `internal/helm/generator_test.go`

**Interfaces:**
- Produces:
  - `helmExecutable() (string, error)`
  - `authenticator.login() error` — mění se rozhraní, takže **obě** implementace (`oci-generator.go`, `repo-generator.go`) musí podpis srovnat
- Consumes: `FindKustomizeFile`/`BuildKustomize` z úlohy 2 se tu nepoužívají

- [ ] **Step 1: Napiš failující test na `helmExecutable`**

```go
func TestHelmExecutableMissing(t *testing.T) {
	t.Setenv(cons.EnvHelmExecutable, "")
	t.Setenv("PATH", t.TempDir()) // no helm anywhere
	if _, err := helmExecutable(); err == nil {
		t.Error("want an error when helm is not on PATH")
	}
}
```

- [ ] **Step 2: Spusť a ověř selhání překladu**

```bash
go test ./internal/helm/ 2>&1 | head -3
```
Expected: `assignment mismatch: 2 variables but helmExecutable returns 1 value`.

- [ ] **Step 3: Převeď `helmExecutable`**

```go
func helmExecutable() (string, error) {
	if executable, found := os.LookupEnv(cons.EnvHelmExecutable); found && executable != "" {
		return executable, nil
	}
	path, err := exec.LookPath("helm")
	if err != nil {
		return "", fmt.Errorf("helm executable not found in OS")
	}
	return path, nil
}
```

Text `"helm executable not found in OS"` zachovej doslova — je to jediná
hláška, kterou uživatel na chybějícím helmu uvidí.

- [ ] **Step 4: Převeď `login` v obou generátorech**

`oci-generator.go`:

```go
func (g *ociHelmGenerator) login() error {
	args := []string{"registry", "login", g.chartIdShort()}
	args = g.addCredentials(args)
	executable, err := helmExecutable()
	if err != nil {
		return err
	}
	if _, _, err := runCommand(executable, args...); err != nil {
		return fmt.Errorf("login to helm registry %q failed reason: %q", g.chartIdShort(), err.Error())
	}
	return nil
}
```

`repo-generator.go` má `login()` prázdné — jen změň podpis na `error` a vrať `nil`.
V `generator.go` uprav rozhraní `authenticator` na `login() error`.

- [ ] **Step 5: Sroluj `templateHelm`**

```go
if credentialsProvided(config) {
	if err := generator.login(); err != nil {
		return "", err
	}
	args = generator.addCredentials(args)
}

executable, err := helmExecutable()
if err != nil {
	return "", err
}
stdOut, stdErr, err := runCommand(executable, args...)
if err != nil {
	return "", fmt.Errorf("run command %q finished with error %v. Error output %v", executable, err, stdErr)
}
return stripHelmBanner(stdOut), nil
```

Pozn.: `helmExecutable()` se dnes volá **dvakrát** na jednom řádku
(`runCommand(helmExecutable(), ...)` a znovu v chybové hlášce). Po převodu
si výsledek ulož do proměnné, jinak budeš muset ošetřovat chybu dvakrát.

- [ ] **Step 6: Spusť testy**

```bash
go test -race -count=1 ./internal/... ./test/golden/
```
Expected: PASS, goldeny beze změny.

- [ ] **Step 7: Commit**

```bash
git add internal/helm/
git commit -m "refactor: return errors from the helm generators"
```

---

### Task 4: `cmd` — poslední patro, cobra tiskne chyby

Tady se chyby slévají a tady se **mění formát stderr**. Úloha končí zápisem
té změny do specifikace, ne jen kódem.

**Files:**
- Modify: `cmd/generate.go`
- Modify: `cmd/root.go`
- Modify: `krmgen.go`
- Modify: `docs/specification.md`
- Test: `cmd/generate_test.go`

**Interfaces:**
- Consumes: vše z úloh 2 a 3
- Produces:
  - `copySrcDir(srcDir string, skipPatterns []string) (string, error)`
  - `copyDir(srcDir, dstDir, baseDir string, skipPatterns []string) error`
  - `processWorkDir(workDir string) error`
  - seam `var fatalf = log.Fatalf` **zaniká** — testy, které ho nahrazovaly, budou asertovat vrácenou chybu

- [ ] **Step 1: Převeď `copyDir`, `copySrcDir`, `processWorkDir` na vracené chyby**

Mechanické: každé `fatalf(...)` → `return fmt.Errorf(...)` se zachovaným
textem, rekurzivní volání `copyDir` propaguje:

```go
if entry.IsDir() {
	if err := os.MkdirAll(dstPath, 0750); err != nil {
		return fmt.Errorf("creating directory %s failed error: %w", dstPath, err)
	}
	if err := copyDir(srcPath, dstPath, baseDir, skipPatterns); err != nil {
		return err
	}
	continue
}
```

Překlep `"crating directory"` oprav na `"creating directory"` — je na seznamu
průběžných oprav v design docu a žádný test na něj neasertuje (ověř grepem,
než ho změníš).

- [ ] **Step 2: Přepni příkaz na `RunE` a dej temp adresáři jednoho vlastníka**

```go
RunE: func(cmd *cobra.Command, args []string) error {
	srcDir, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	configPatterns := config.ReadSkipPatterns(srcDir)
	merged := mergeSkipPatterns(configPatterns, skipPatterns)
	return generate(srcDir, merged)
},
```

```go
// generate owns the working directory for its whole lifetime: the deferred
// removal is registered immediately after the directory exists, so it runs on
// every path out - including a failure part-way through processing, which
// used to leave a directory full of rendered secrets behind.
func generate(srcDir string, skipPatterns []string) (err error) {
	workDir, err := copySrcDir(srcDir, skipPatterns)
	if workDir != "" {
		defer func() {
			if rmErr := os.RemoveAll(workDir); rmErr != nil && err == nil {
				err = fmt.Errorf("removing working dir %s failed error: %w", workDir, rmErr)
			}
		}()
	}
	if err != nil {
		return err
	}
	return processWorkDir(workDir)
}
```

Dvě věci, na kterých to stojí, a obě jsou dnes rozbité:
1. `defer` se registruje **hned** po vzniku adresáře, ne až po `processWorkDir`
   jako dnes (`cmd/generate.go:38-41`).
2. `copySrcDir` může selhat *až po* `os.MkdirTemp` — proto se `defer` váže na
   neprázdný `workDir`, ne na `err == nil`.

- [ ] **Step 3: Umlč nápovědu u běhových chyb**

V `cmd/root.go` i v `NewGenerateCommand` nastav:

```go
SilenceUsage: true,
```

Bez toho cobra na každou běhovou chybu vysype celou nápovědu, což je pro
`krmgen generate` v ArgoCD logu nepoužitelné. `SilenceErrors` **nenastavuj** —
chceme, aby chybu vytiskla cobra.

- [ ] **Step 4: `krmgen.go` už netiskne, jen končí**

```go
func main() {
	if err := cmd.NewRootCommand(version).Execute(); err != nil {
		os.Exit(1)
	}
}
```

Cobra chybu vytiskla sama; `log.Fatal(err)` by ji vytiskl podruhé. Import
`log` odstraň.

- [ ] **Step 5: Spusť golden sadu a zkontroluj chybový kontrakt**

```bash
go test -race -run 'TestError' ./test/golden/ -v
go test -race -count=1 ./...
```
Expected: PASS. Podřetězce sedí, exit kód 1 sedí. Ručně si prohlédni, jak
stderr teď vypadá:

```bash
go build -o /tmp/k . && /tmp/k generate test/golden/fixtures/two-kustomizations
```
Expected: `Error: found multiple kustomization files under: /tmp/krmgen...`,
žádné `time=`, žádné `level=fatal`, žádná nápověda.

- [ ] **Step 6: Zapiš změnu formátu do specifikace**

Do `docs/specification.md`, sekce 1 (CLI), přidej odstavec — **anglicky**:

```markdown
### Error output

A failing run writes a single line to stderr and exits with code 1:

    Error: <what failed>

Nothing else is printed: no timestamp, no log level, and no usage text.
Before phase 5 the format depended on which package raised the error - the
kustomize processor used logrus (`time=... level=fatal msg="..."`) and the
rest used the standard library's `log` (`2026/08/27 00:23:33 ...`). The
message text itself did not change, only what surrounds it.

Callers matching on stderr should match on the message, not on the line shape.
```

- [ ] **Step 7: Commit**

```bash
git add cmd/ krmgen.go docs/specification.md
git commit -m "refactor: let errors reach cobra instead of exiting mid-call"
```

---

### Task 5: Práva souborů s vyrenderovanými secrets

Do pracovního adresáře se zapisují vyhodnocené šablony — tedy i to, co vrátil
`azSec`. Dnes jdou na disk s `os.ModePerm` (0777) a `0666`.

**Files:**
- Create: `internal/utils/perm.go`
- Modify: `cmd/generate.go` (2 zápisy), `internal/kustomize/processor.go` (2 zápisy), `internal/helm/processor.go` (1 zápis)
- Test: `cmd/generate_test.go`

**Interfaces:**
- Produces: `cons.FilePerm` (`os.FileMode` = 0600), `cons.DirPerm` (0700)

- [ ] **Step 1: Napiš failující test**

```go
func TestCopiedFilesAreNotWorldReadable(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "krmgen.yaml"), []byte("kind: KrmGen\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workDir, err := copySrcDir(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	info, err := os.Stat(filepath.Join(workDir, "krmgen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("copied file mode = %04o, want 0600 - it may hold rendered secrets", perm)
	}
}
```

- [ ] **Step 2: Spusť, ověř selhání**

```bash
go test -run TestCopiedFilesAreNotWorldReadable ./cmd/ -v
```
Expected: FAIL, `copied file mode = 0777, want 0600` (přesná hodnota závisí
na umask — proto se asertuje na `0600` až po opravě, ne na dnešní hodnotě).

- [ ] **Step 3: Zaveď konstanty**

`internal/utils/perm.go`:

```go
package utils

import "os"

// FilePerm and DirPerm are the modes krmgen creates working files and
// directories with. Everything krmgen writes to its working directory is a
// rendered template, which may hold secrets pulled from a key vault, so
// nothing is readable outside the owning user.
const (
	FilePerm os.FileMode = 0600
	DirPerm  os.FileMode = 0700
)
```

- [ ] **Step 4: Nahraď všech pět zápisů**

`os.ModePerm` → `cons.FilePerm`, `0666` → `cons.FilePerm`, `os.MkdirAll(..., 0750)`
→ `cons.DirPerm`. Ověř grepem, že v produkčním kódu nezbyl žádný literál:

```bash
grep -rn 'ModePerm\|0666\|0777\|0750' --include=*.go . | grep -v _test
```
Expected: prázdný výstup.

- [ ] **Step 5: Spusť testy**

```bash
go test -race -count=1 ./...
```
Expected: PASS. Goldeny se nemění — práva neovlivňují vyrenderovaný obsah.

- [ ] **Step 6: Zapiš do specifikace**

Doplň do sekce o pracovním adresáři, **anglicky**, že krmgen vytváří pracovní
adresář s `0700` a soubory v něm s `0600`, protože jde o vyhodnocené šablony,
které mohou nést secrets. Zmiň, že do fáze 5 to bylo `0777`/`0666`.

- [ ] **Step 7: Commit**

```bash
git add internal/utils/perm.go cmd/ internal/kustomize/processor.go internal/helm/processor.go docs/specification.md
git commit -m "fix: stop writing rendered secrets with world-readable modes"
```

---

### Task 6: Doložit, že temp adresář po chybě nezůstává

Úloha 4 to opravila; tahle to **prokáže**. Bez testu je to jen tvrzení — a
tvrzení bez testu je přesně to, co tenhle projekt v předchozích fázích
opakovaně odmítl.

**Files:**
- Modify: `test/golden/errors_test.go`
- Modify: `docs/specification.md` (odchylka mizí)

**Interfaces:**
- Consumes: `runScenario` s vlastním `TMPDIR`

- [ ] **Step 1: Napiš test, který sleduje vlastní TMPDIR**

```go
// TestError_WorkingDirectoryRemovedOnFailure covers what used to be a
// documented deviation: log.Fatal skipped the deferred cleanup, so a failing
// run left a working directory full of rendered templates behind. The run is
// given a TMPDIR of its own so nothing else on the host can be mistaken for
// krmgen's leftovers.
func TestError_WorkingDirectoryRemovedOnFailure(t *testing.T) {
	tmp := t.TempDir()
	res := runScenario(t, "two-kustomizations", "TMPDIR="+tmp)
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "krmgen") {
			t.Errorf("working directory %q survived a failed run", e.Name())
		}
	}
}
```

Ověř v `harness_test.go`, že `runScenario` předané `TMPDIR` skutečně propíše
do prostředí podprocesu a že `minimalEnv()` ho nepřebije — pokud ho helper
nastavuje sám, předej ho tak, aby vyhrála hodnota z testu.

- [ ] **Step 2: Spusť a ověř, že prochází**

```bash
go test -race -run TestError_WorkingDirectoryRemovedOnFailure ./test/golden/ -v
```
Expected: PASS (díky úloze 4). Kdyby FAIL, je oprava v úloze 4 neúplná —
nejspíš cesta, kde se selže dřív, než `generate` převezme vlastnictví.

- [ ] **Step 3: Ověř, že test opravdu něco hlídá**

Dočasně zakomentuj `defer` v `generate` a spusť test znovu. Musí selhat.
Pak `defer` **vrať** a spusť znovu — musí projít. Bez tohohle kroku nevíš,
jestli test není vakuózní. Výsledek obou běhů zapiš do reportu.

- [ ] **Step 4: Vyškrtni odchylku ze specifikace**

`docs/specification.md`, sekce „Working directory lifecycle" (~ř. 233) dnes říká:

> **Known deviation:** when rendering fails, the process exits before cleanup runs,
> leaving the working directory — including any rendered secrets — on disk. This is
> recorded as current behaviour; it is fixed in phase 3 of the refactoring plan.

Nahraď ji popisem současného chování s odkazem na test z kroku 1. Nemaž ji beze
stopy — je to změna kontraktu, ne oprava překlepu. Všimni si, že věta navíc
odkazuje na **fázi 3**, což nikdy neplatilo; nepřepisuj číslo, celou tu větu
nahradíš.

Ve stejné sekci stojí „created … with mode 0700", což je pravda jen o kořenovém
adresáři (`os.MkdirTemp` ho tak zakládá sám). O právech souborů uvnitř mlčí —
to doplňuje úloha 5, ověř, že se ty dvě formulace neperou.

- [ ] **Step 5: Commit**

```bash
git add test/golden/errors_test.go docs/specification.md
git commit -m "test: prove the working directory is removed when a run fails"
```

---

### Task 7: Jeden logger a úklid

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `CLAUDE.md`
- Modify: `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`

- [ ] **Step 1: Ověř, že logrus zmizel z produkčního kódu**

```bash
grep -rn 'logrus' --include=*.go . | grep -v _test
```
Expected: prázdný výstup. Pokud ne, zbylé místo doplň do úlohy 2 nebo 3.

- [ ] **Step 2: Zkontroluj, co ze stdlib `log` ještě zbývá**

```bash
grep -rn '"log"' --include=*.go . | grep -v _test
```
Legitimní zbytky jsou jen informativní `log.Println` (např.
`internal/config/parser.go` při nečitelném souboru). Žádné `log.Fatal*`
v produkčním kódu zbýt nesmí:

```bash
grep -rn 'log\.Fatal' --include=*.go . | grep -v _test | grep -v cmd/only-test
```
Expected: prázdný výstup.

- [ ] **Step 3: Vyhoď logrus ze závislostí**

```bash
go mod tidy
git diff go.mod
```
Expected: `github.com/sirupsen/logrus` mizí z přímých závislostí. Pokud tam
zůstane jako nepřímá (táhne si ji něco jiného), je to v pořádku — zapiš to do
reportu, nevynucuj.

Pozn.: `test/golden/harness_test.go` a `internal/kustomize/processor_test.go`
na logrus jen odkazují v komentářích. Ty komentáře uprav, ať nepopisují svět,
který už neexistuje.

- [ ] **Step 4: Srovnej dokumentaci**

- `CLAUDE.md`, sekce „Code conventions": řádek
  „`log.Fatal` used throughout (process exits on any error — intentional for a CLI tool)"
  už neplatí. Nahraď popisem současného stavu: chyby se vrací, tiskne je cobra,
  exit kód 1.
- Design doc, tabulka „Kvalita kódu — přiřazení k fázím": čtyři řádky přiřazené
  fázi 5 přepiš na **hotovo 5** s odkazem na to, co je drží. Řádek o
  `log.Fatal` uveď na pravou míru: bylo jich 14 v produkčním kódu, ne 27.

- [ ] **Step 5: Plná kontrola**

```bash
golangci-lint run ./...
go test -race -count=1 ./...
gofmt -l . && go vet ./...
git diff --stat -- test/golden/fixtures/
```
Expected: 0 issues, vše PASS, prázdný výstup obou posledních příkazů
(**ani jeden golden se nesmí hnout**).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum CLAUDE.md docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md internal/ test/
git commit -m "chore: drop the second logger and record phase 5 in the docs"
```

---

## Co tahle fáze vědomě nedělá

- **Nesahá na helm.** Výměna `helm template` za SDK je samostatné rozhodnutí
  s naměřenou cenou: binárka 20,7 MB → ~62 MB, 103 → 265 modulů v grafu.
  Rozhoduje se o ní až nad kódem, který umí vracet chyby — což je přesně to,
  co tahle fáze dodává.
- **Nemění, co krmgen vyrenderuje.** Jediná povolená změna výstupu je formát
  stderr při chybě, a ta je zapsaná ve specifikaci.
- **Neřeší `cmd/only-test/run.go`.** Je to vývojová utilita mimo produkt
  (`.goreleaser.yaml` staví `main: .`); jeho dva `log.Fatal` jsou tam na místě.
- **Nezavádí strukturované logování ani úrovně.** krmgen na stdout tiskne YAML;
  cokoli dalšího na stderr je šum. Kdyby to někdy bylo potřeba, je to nová fáze
  s vlastním zadáním.

## Sebekontrola plánu

- **Pokrytí:** čtyři odložené položky kvality mají každá svou úlohu — `log.Fatal`
  (úlohy 2–4), práva (5), únik temp adresáře (4 opravuje, 6 dokazuje), dva
  loggery (7).
- **Typová konzistence:** `FindKustomizeFile` vrací `(string, error)` v úloze 2
  a stejně se konzumuje v `internal/config/processor.go`; `login() error` se
  mění v rozhraní i v obou implementacích současně (úloha 3), jinak balíček
  nepřeloží.
- **Pořadí:** každá úloha končí přeložitelným stromem se zelenou sadou. Úloha 4
  závisí na 2 i 3 (jinak by `cmd` volalo funkce, které ještě fatalují); 5 a 6
  závisí na 4.
