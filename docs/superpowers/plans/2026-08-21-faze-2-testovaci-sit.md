# Fáze 2: Golden-master harness a testovací síť — implementační plán

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Postavit záchrannou síť, která zachytí každou změnu výstupu krmgenu, aby se pozdější výměna helmu a kustomize za knihovny dala měřit, ne odhadovat.

**Architecture:** Tři vrstvy. Golden-master testy pouštějí **postavenou binárku jako podproces** proti hermetickým fixtures a porovnávají celý stdout proti uloženému souboru — to je kontrakt nezávislý na implementaci. Unit testy s podvrženým spouštěčem příkazů pokrývají chybové větve, které přes E2E rozumně vyvolat nejdou. Čisté funkce se testují přímo. Hermetičnost zajišťuje lokální HTTP chart repozitář z `httptest`; OCI cesta zůstává za build tagem, protože ji hermeticky obsloužit nelze.

**Tech Stack:** Go 1.26, `net/http/httptest`, helm ≥ 3.8.0, kubectl, Task, GitHub Actions

**Spec:** `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md` a produktová specifikace `docs/specification.md`

## Global Constraints

- Go 1.26.0 (`go.mod`); `.tool-versions` uvádí `golang 1.26.3`
- Podporované verze nástrojů podle specifikace: **helm 3.8.0 a novější včetně 4.x**, kubectl s podporou `kubectl kustomize`
- Komentáře a dokumentace v kódu **anglicky**; interní dokumenty v `docs/superpowers/` česky
- **Testy nesmí chodit na síť.** Jedinou výjimkou jsou testy za build tagem `oci`, které se v běžném `go test ./...` nespouštějí
- **Fáze nemění chování.** Povolené zásahy do produkčního kódu jsou pouze injektovatelné body (`var runCommand`, `var fatalf`) — stejné hodnoty, stejné hlášky, stejné exit kódy
- Golden soubory jsou kontrakt. `-update` je regeneruje, ale **každý diff se čte a schvaluje**; slepě přegenerovaný golden je zahozená záchranná síť
- Cíl pokrytí ~80 % celkově; výchozí stav je **36,9 %**
- Nové soubory se přidávají do gitu, commituje se na konci každé úlohy

---

## Výchozí stav, ověřený 2026-08-21

Tahle čísla a fakta jsou změřená, ne odhadnutá. Neplýtvej časem jejich ověřováním znovu.

| Fakt | Hodnota |
|---|---|
| Celkové pokrytí | 36,9 % |
| `cmd/generate.go` | `copySrcDir`, `copyDir`, `processWorkDir`, `NewGenerateCommand` — vše 0 % |
| `internal/kustomize/processor.go` | všechny funkce 0 % |
| `internal/helm/processor.go` | `TemplateHelmCharts`, `templateHelm`, `getValuesArgs` — 0 % |
| `internal/config/processor.go` | `ProcessConfig` 0 % |
| `internal/tool/tool.go` | `RunCommand` 0 % |
| CI | **červené od května 2026** — `make: *** No rule to make target 'build'` |

### Dvě věci, které vyvracejí předpoklad v designovém dokumentu

**1. krmgen neumí chart z lokálního adresáře.** `newGenerator` (`internal/helm/generator.go:36`) přijímá jen `oci://` a `http(s)://`, jinak vrací chybu. Designový dokument počítal s hermetickými fixtures z adresáře, protože „helm i SDK umí renderovat chart z adresáře" — helm ano, krmgen mu to ale neumí říct.

Náhrada, ověřená spuštěním: **lokální HTTP chart repozitář**. Zabalený chart plus `index.yaml` servírovaný přes `httptest`, a `repo: http://127.0.0.1:<port>`. krmgen proti němu vyrenderoval 4 manifesty s `rc=0`.

**2. OCI hermeticky nepůjde.** Helm pro registry bez TLS vyžaduje `--plain-http`, pro registry s vlastním certifikátem `--insecure-skip-tls-verify`. krmgen nepředává ani jedno, a přidat to by bylo změnou chování. OCI proto zůstane na síťovém testu za build tagem — přesně jak designový dokument předpokládal.

---

## File Structure

| Soubor | Zodpovědnost |
|---|---|
| `Makefile` | Vytvořit. Tenká obálka nad Taskfile, aby CI přestalo padat. |
| `.github/workflows/go.yml` | Upravit. Přidat job, který pouští testy s helmem a kubectl. |
| `internal/tool/tool_test.go` | Vytvořit. `RunCommand` proti skutečným binárkám. |
| `internal/kustomize/processor.go` | Upravit. Přidat `var runCommand`. |
| `internal/kustomize/processor_test.go` | Přepsat. Dnes obsahuje jen `package kustomize`. |
| `internal/helm/processor.go` | Upravit. Přidat `var runCommand`. |
| `internal/helm/processor_test.go` | Rozšířit o `templateHelm`, `TemplateHelmCharts`, `getValuesArgs`. |
| `cmd/generate.go` | Upravit. Přidat `var fatalf`. |
| `cmd/generate_test.go` | Rozšířit o `copyDir` a `processWorkDir`. |
| `internal/config/processor_test.go` | Vytvořit. `ProcessConfig`. |
| `internal/template/{argocd,kube,krmgen}/*_test.go` | Vytvořit. Čisté funkce nad proměnnými prostředí. |
| `test/golden/harness_test.go` | Vytvořit. Spouštěč scénářů, HTTP chart repozitář, porovnání goldenů, `-update`. |
| `test/golden/charts/demo/` | Vytvořit. Minimální deterministický chart. |
| `test/golden/fixtures/<scénář>/` | Vytvořit. Vstupy plus `golden.yaml`. |
| `test/golden/oci_test.go` | Vytvořit. Síťový OCI test za build tagem `oci`. |

**Proč E2E přes podproces, ne in-process:** golden testy pouštějí postavenou binárku. Tím se testuje skutečná dělba stdout/stderr a skutečné exit kódy — přesně to, co specifikace slibuje — a chybové scénáře nezabijí testovací proces navzdory `log.Fatal`. Pokrytí `cmd/` doháníme unit testy v úloze 6, ne přes E2E.

---

### Task 1: Zelené CI

CI padá od května na chybějícím `Makefile`. Dokud to neopravíme, žádný test z téhle fáze v CI nepoběží.

**Files:**
- Create: `Makefile`
- Modify: `.github/workflows/go.yml`

**Interfaces:**
- Produces: cíl `make build` a `make test`; job `Test` v CI, na kterém závisí smysl všech dalších úloh

- [ ] **Step 1: Ověřit, že problém je pořád aktuální**

```bash
gh run list --limit 3 --workflow="Build KrmGen" --json conclusion,createdAt
```

Očekávané: `"conclusion":"failure"`. Kdyby už bylo zeleno, přeskoč na krok 4 a jen přidej testovací job.

- [ ] **Step 2: Vytvořit `Makefile`**

Projekt používá Taskfile; `Makefile` je jen most pro CI a pro lidi, co píšou `make` ze zvyku.

```makefile
# Thin bridge over Taskfile so `make` works for CI and habit.
# The real task definitions live in Taskfile.yaml.

.PHONY: build test lint check install

build:
	go build -o build/krmgen .

test:
	go test -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run ./...

check: build test

install:
	go install .
```

- [ ] **Step 3: Ověřit lokálně**

```bash
make build && ls -l build/krmgen
make test 2>&1 | tail -5
```

Očekávané: binárka vznikne, testy projdou.

- [ ] **Step 4: Přidat testovací job do CI**

V `.github/workflows/go.yml` přidej za stávající `build` job:

```yaml
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      # krmgen shells out to both binaries; the specification's support
      # matrix pins the helm floor at 3.8.0
      - name: Set up helm
        uses: azure/setup-helm@v4
        with:
          version: v3.21.4

      - name: Set up kubectl
        uses: azure/setup-kubectl@v4

      - name: Verify tool versions
        run: |
          helm version --short
          kubectl version --client | grep -i kustomize

      - name: Test
        run: go test -race -coverprofile=coverage.out ./...

      - name: Coverage summary
        run: go tool cover -func=coverage.out | tail -1
```

- [ ] **Step 5: Commit**

```bash
git add Makefile .github/workflows/go.yml
git commit -m "ci: fix the build target and run tests

The workflow called 'make build' but the repository has no Makefile - it
uses Taskfile - so every run since May failed on 'No rule to make target'.
Adds a thin Makefile bridge and a test job with helm and kubectl, which
krmgen shells out to."
```

---

### Task 2: `internal/tool` — spouštěč příkazů

Nejmenší netestovaný kus a základ všeho ostatního: 16 řádků, 0 % pokrytí.

**Files:**
- Create: `internal/tool/tool_test.go`

**Interfaces:**
- Consumes: nic
- Produces: nic; ověřuje `tool.RunCommand(name string, arg ...string) (stdOut, stdErr string, err error)`

- [ ] **Step 1: Napsat padající test**

Test používá skutečné systémové binárky, ne mocky — `RunCommand` je tenká obálka nad `exec.Command` a testovat ji přes mock by netestovalo nic.

```go
package tool

import (
	"runtime"
	"strings"
	"testing"
)

func TestRunCommand_CapturesStdout(t *testing.T) {
	stdOut, stdErr, err := RunCommand("echo", "hello")
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if strings.TrimSpace(stdOut) != "hello" {
		t.Errorf("stdOut = %q, want %q", strings.TrimSpace(stdOut), "hello")
	}
	if stdErr != "" {
		t.Errorf("stdErr = %q, want empty", stdErr)
	}
}

func TestRunCommand_CapturesStderrSeparately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	stdOut, stdErr, err := RunCommand("sh", "-c", "echo out; echo err >&2")
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if strings.TrimSpace(stdOut) != "out" {
		t.Errorf("stdOut = %q, want %q", strings.TrimSpace(stdOut), "out")
	}
	if strings.TrimSpace(stdErr) != "err" {
		t.Errorf("stdErr = %q, want %q", strings.TrimSpace(stdErr), "err")
	}
}

func TestRunCommand_ReportsExitFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	_, stdErr, err := RunCommand("sh", "-c", "echo boom >&2; exit 3")
	if err == nil {
		t.Fatal("RunCommand() error = nil, want a non-zero exit error")
	}
	if strings.TrimSpace(stdErr) != "boom" {
		t.Errorf("stdErr = %q, want %q", strings.TrimSpace(stdErr), "boom")
	}
}

func TestRunCommand_ReportsMissingBinary(t *testing.T) {
	_, _, err := RunCommand("krmgen-no-such-binary-xyz")
	if err == nil {
		t.Fatal("RunCommand() error = nil, want an error for a missing binary")
	}
}
```

- [ ] **Step 2: Pustit test**

```bash
go test ./internal/tool/ -v
```

Očekávané: PASS. Tenhle test popisuje existující chování, takže neprochází fází RED — a to je v pořádku, je to charakterizační test, ne nová funkce. Kdyby něco spadlo, **neopravuj test, ale nahlas to** — znamenalo by to, že `RunCommand` nedělá, co si myslíme.

- [ ] **Step 3: Ověřit pokrytí**

```bash
go test -coverprofile=/tmp/c.out ./internal/tool/ && go tool cover -func=/tmp/c.out
```

Očekávané: `RunCommand 100.0%`.

- [ ] **Step 4: Commit**

```bash
git add internal/tool/tool_test.go
git commit -m "test: cover the external command wrapper"
```

---

### Task 3: `internal/kustomize` — seam a unit testy

Balíček má 122 řádků a nulové pokrytí. Deset míst v něm volá `log.Fatalf` z logrusu, což jde v testu zachytit přes `logrus.StandardLogger().ExitFunc` — bez zásahu do produkčního kódu. Volání kubectl se podvrhne přes nový seam.

**Files:**
- Modify: `internal/kustomize/processor.go`
- Modify: `internal/kustomize/processor_test.go` (dnes obsahuje jen `package kustomize`)

**Interfaces:**
- Consumes: `tool.RunCommand`
- Produces: `var runCommand = tool.RunCommand` v balíčku `kustomize` — stejný tvar seamu použije úloha 4 v balíčku `helm`

- [ ] **Step 1: Napsat padající testy pro čisté funkce a vyhledávání**

```go
package kustomize

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestUnwrapResources(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		want    []string
		wantErr bool
	}{
		{name: "empty list", in: []any{}, want: []string{}},
		{name: "string items", in: []any{"a.yaml", "b.yaml"}, want: []string{"a.yaml", "b.yaml"}},
		{name: "not a list", in: "a.yaml", wantErr: true},
		{name: "non-string item", in: []any{"a.yaml", 42}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unwrapResources(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unwrapResources() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unwrapResources() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindKustomizeFile(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantBase string
	}{
		{name: "no kustomization", files: []string{"a.yaml"}, wantBase: ""},
		{name: "yaml at top level", files: []string{"kustomization.yaml"}, wantBase: "kustomization.yaml"},
		{name: "yml variant", files: []string{"kustomization.yml"}, wantBase: "kustomization.yml"},
		{name: "extensionless variant", files: []string{"kustomization"}, wantBase: "kustomization"},
		{name: "case insensitive", files: []string{"Kustomization.YAML"}, wantBase: "Kustomization.YAML"},
		{name: "found in a subdirectory", files: []string{"nested/kustomization.yaml"}, wantBase: "kustomization.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				path := filepath.Join(dir, f)
				if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("kind: Kustomization\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got := FindKustomizeFile(dir)
			if tt.wantBase == "" {
				if got != "" {
					t.Errorf("FindKustomizeFile() = %q, want empty", got)
				}
				return
			}
			if filepath.Base(got) != tt.wantBase {
				t.Errorf("FindKustomizeFile() = %q, want basename %q", got, tt.wantBase)
			}
		})
	}
}

// captureFatal redirects logrus' exit so a log.Fatalf in production code
// aborts the call under test instead of the test binary.
func captureFatal(t *testing.T, call func()) (fatal bool) {
	t.Helper()
	original := log.StandardLogger().ExitFunc
	t.Cleanup(func() { log.StandardLogger().ExitFunc = original })
	log.StandardLogger().ExitFunc = func(int) { panic("log.Fatal") }
	defer func() {
		if r := recover(); r != nil {
			fatal = true
		}
	}()
	call()
	return false
}

func TestFindKustomizeFile_MultipleFilesIsFatal(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"kustomization.yaml", "nested/kustomization.yaml"} {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("kind: Kustomization\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !captureFatal(t, func() { FindKustomizeFile(dir) }) {
		t.Error("expected two kustomization files to be fatal, but the call returned")
	}
}

func TestBuildKustomize_AppendsResourcesAndInvokesKubectl(t *testing.T) {
	dir := t.TempDir()
	kustomizeFile := filepath.Join(dir, "kustomization.yaml")
	if err := os.WriteFile(kustomizeFile, []byte("kind: Kustomization\nresources:\n  - existing.yaml\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var gotName string
	var gotArgs []string
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		gotName, gotArgs = name, arg
		return "rendered: true\n", "", nil
	}

	got := BuildKustomize(kustomizeFile, dir, "kind: ConfigMap\n")

	if got != "rendered: true\n" {
		t.Errorf("BuildKustomize() = %q, want the kubectl output", got)
	}
	if gotName != "kubectl" {
		t.Errorf("invoked %q, want %q", gotName, "kubectl")
	}
	if !reflect.DeepEqual(gotArgs, []string{"kustomize", dir}) {
		t.Errorf("args = %v, want %v", gotArgs, []string{"kustomize", dir})
	}

	updated, err := os.ReadFile(kustomizeFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "existing.yaml") {
		t.Error("the pre-existing resource entry was dropped")
	}
	if !strings.Contains(string(updated), ".yml") {
		t.Error("the generated resources file was not appended to the kustomization")
	}
}
```

- [ ] **Step 2: Pustit a ověřit, že to nejde přeložit**

```bash
go test ./internal/kustomize/ -run TestBuildKustomize -v
```

Očekávané: FAIL, `undefined: runCommand`.

- [ ] **Step 3: Přidat seam**

V `internal/kustomize/processor.go` přidej pod deklaraci `allowedFileNames`:

```go
// runCommand is a seam: tests replace it to observe the kubectl invocation
// without running the binary.
var runCommand = tool.RunCommand
```

a v `BuildKustomize` nahraď `tool.RunCommand(` za `runCommand(`.

- [ ] **Step 4: Pustit testy**

```bash
go test ./internal/kustomize/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Očekávané: všechny PASS.

- [ ] **Step 5: Ověřit, že se chování nezměnilo**

```bash
go build -o build/krmgen . && ./build/krmgen generate test/resources/kustomization-only >/dev/null 2>&1; echo "rc=$?"
```

Očekávané: `rc=0` — stejně jako před úlohou.

- [ ] **Step 6: Commit**

```bash
git add internal/kustomize/processor.go internal/kustomize/processor_test.go
git commit -m "test: cover the kustomize package

Adds a runCommand seam so the kubectl invocation can be observed without
running the binary, and uses logrus' ExitFunc to test the fatal paths."
```

---

### Task 4: `internal/helm` — seam a unit testy

**Files:**
- Modify: `internal/helm/processor.go`
- Modify: `internal/helm/processor_test.go`

**Interfaces:**
- Consumes: vzor seamu z úlohy 3
- Produces: `var runCommand = tool.RunCommand` v balíčku `helm`

- [ ] **Step 1: Napsat padající testy**

Testy zachycují **přesné argumenty** předané helmu. Ty se ve fázi 5 změní — a to je záměr: až se argumenty přestanou skládat, tyhle testy se smažou spolu s kódem, který je vyráběl. Do té doby drží dnešní chování.

```go
func TestGetValuesArgs(t *testing.T) {
	workDir := t.TempDir()

	t.Run("no values", func(t *testing.T) {
		args, err := getValuesArgs(&types.HelmChart{}, workDir)
		if err != nil {
			t.Fatalf("getValuesArgs() error = %v", err)
		}
		if len(args) != 0 {
			t.Errorf("args = %v, want none", args)
		}
	})

	t.Run("values file is joined with the work dir", func(t *testing.T) {
		args, err := getValuesArgs(&types.HelmChart{ValuesFile: "values.yaml"}, workDir)
		if err != nil {
			t.Fatalf("getValuesArgs() error = %v", err)
		}
		want := []string{"--values", filepath.Join(workDir, "values.yaml")}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("args = %v, want %v", args, want)
		}
	})

	t.Run("inline values are written to a file", func(t *testing.T) {
		chart := &types.HelmChart{
			ReleaseName:  "rel",
			ValuesInline: map[string]any{"replicaCount": 2},
		}
		args, err := getValuesArgs(chart, workDir)
		if err != nil {
			t.Fatalf("getValuesArgs() error = %v", err)
		}
		if len(args) != 2 || args[0] != "--values" {
			t.Fatalf("args = %v, want a --values pair", args)
		}
		content, err := os.ReadFile(args[1])
		if err != nil {
			t.Fatalf("reading the generated values file: %v", err)
		}
		if !strings.Contains(string(content), "replicaCount: 2") {
			t.Errorf("generated values = %q, want it to contain replicaCount: 2", content)
		}
	})

	t.Run("both sources produce two --values in order", func(t *testing.T) {
		chart := &types.HelmChart{
			ReleaseName:  "rel",
			ValuesFile:   "values.yaml",
			ValuesInline: map[string]any{"a": 1},
		}
		args, err := getValuesArgs(chart, workDir)
		if err != nil {
			t.Fatalf("getValuesArgs() error = %v", err)
		}
		if len(args) != 4 || args[0] != "--values" || args[2] != "--values" {
			t.Fatalf("args = %v, want two --values pairs", args)
		}
		if args[1] != filepath.Join(workDir, "values.yaml") {
			t.Errorf("the values file must come first, got %v", args)
		}
	})
}

func TestTemplateHelmCharts_InvocationPerBackend(t *testing.T) {
	tests := []struct {
		name  string
		chart types.HelmChart
		want  []string
	}{
		{
			name: "oci backend passes the repo url positionally",
			chart: types.HelmChart{
				Name: "app", RepoUrl: "oci://reg.example.com/helm/app",
				ReleaseName: "rel", Version: "1.2.3", Namespace: "ns",
				IgnoreCredentials: true,
			},
			want: []string{"template", "rel", "--include-crds", "--version", "1.2.3", "--namespace", "ns", "oci://reg.example.com/helm/app"},
		},
		{
			name: "http backend passes --repo and the chart name",
			chart: types.HelmChart{
				Name: "app", RepoUrl: "https://charts.example.com",
				ReleaseName: "rel", Version: "1.2.3", Namespace: "ns",
				IgnoreCredentials: true,
			},
			want: []string{"template", "rel", "--include-crds", "--version", "1.2.3", "--namespace", "ns", "--repo", "https://charts.example.com", "--release-name", "app"},
		},
		{
			name: "version and namespace are omitted when unset",
			chart: types.HelmChart{
				Name: "app", RepoUrl: "oci://reg.example.com/helm/app",
				ReleaseName: "rel", IgnoreCredentials: true,
			},
			want: []string{"template", "rel", "--include-crds", "oci://reg.example.com/helm/app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs []string
			original := runCommand
			t.Cleanup(func() { runCommand = original })
			runCommand = func(name string, arg ...string) (string, string, error) {
				gotArgs = arg
				return "---\nkind: ConfigMap\n", "", nil
			}

			charts := []types.HelmChart{tt.chart}
			out, err := TemplateHelmCharts(&types.Helm{Charts: &charts}, t.TempDir())
			if err != nil {
				t.Fatalf("TemplateHelmCharts() error = %v", err)
			}
			if out != "---\nkind: ConfigMap\n" {
				t.Errorf("output = %q, want the helm output unchanged", out)
			}
			if !reflect.DeepEqual(gotArgs, tt.want) {
				t.Errorf("args =\n  %v\nwant\n  %v", gotArgs, tt.want)
			}
		})
	}
}

func TestTemplateHelmCharts_ConcatenatesInDeclarationOrder(t *testing.T) {
	calls := 0
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		calls++
		return fmt.Sprintf("---\nkind: Chart%d\n", calls), "", nil
	}

	charts := []types.HelmChart{
		{Name: "a", RepoUrl: "oci://reg.example.com/a", ReleaseName: "a", IgnoreCredentials: true},
		{Name: "b", RepoUrl: "oci://reg.example.com/b", ReleaseName: "b", IgnoreCredentials: true},
	}
	out, err := TemplateHelmCharts(&types.Helm{Charts: &charts}, t.TempDir())
	if err != nil {
		t.Fatalf("TemplateHelmCharts() error = %v", err)
	}
	want := "---\nkind: Chart1\n---\nkind: Chart2\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestTemplateHelmCharts_PropagatesFailure(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		return "", "chart not found", errors.New("exit status 1")
	}

	charts := []types.HelmChart{{Name: "a", RepoUrl: "oci://reg.example.com/a", ReleaseName: "a", IgnoreCredentials: true}}
	_, err := TemplateHelmCharts(&types.Helm{Charts: &charts}, t.TempDir())
	if err == nil {
		t.Fatal("TemplateHelmCharts() error = nil, want the helm failure to propagate")
	}
	if !strings.Contains(err.Error(), "chart not found") {
		t.Errorf("error = %v, want it to carry helm's stderr", err)
	}
}

func TestTemplateHelmCharts_StripsBannerBeforeConcatenating(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(name string, arg ...string) (string, string, error) {
		return "Pulled: reg/app:1\nDigest: sha256:abc\n---\nkind: ConfigMap\n", "", nil
	}

	charts := []types.HelmChart{
		{Name: "a", RepoUrl: "oci://reg.example.com/a", ReleaseName: "a", IgnoreCredentials: true},
		{Name: "b", RepoUrl: "oci://reg.example.com/b", ReleaseName: "b", IgnoreCredentials: true},
	}
	out, err := TemplateHelmCharts(&types.Helm{Charts: &charts}, t.TempDir())
	if err != nil {
		t.Fatalf("TemplateHelmCharts() error = %v", err)
	}
	if strings.Contains(out, "Pulled:") || strings.Contains(out, "Digest:") {
		t.Errorf("banner survived concatenation: %q", out)
	}
}
```

Do importů `processor_test.go` doplň `errors`, `fmt`, `os`, `path/filepath`, `reflect`, `strings` a `types "github.com/librucha/krmgen/internal"`.

- [ ] **Step 2: Pustit a ověřit selhání**

```bash
go test ./internal/helm/ -run 'TestTemplateHelmCharts|TestGetValuesArgs' -v 2>&1 | tail -5
```

Očekávané: FAIL, `undefined: runCommand`.

- [ ] **Step 3: Přidat seam**

V `internal/helm/processor.go` přidej pod `helmExecutable`:

```go
// runCommand is a seam: tests replace it to observe the helm invocation
// without running the binary.
var runCommand = tool.RunCommand
```

a v `templateHelm` nahraď `tool.RunCommand(` za `runCommand(`.

- [ ] **Step 4: Pustit testy**

```bash
go test ./internal/helm/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Očekávané: vše PASS. Poslední test je regresní pojistka na dnešní opravu banneru — dokazuje, že se strip děje **před** spojením chartů, ne po něm.

- [ ] **Step 5: Commit**

```bash
git add internal/helm/processor.go internal/helm/processor_test.go
git commit -m "test: cover helm invocation and values handling

Pins the exact argv for both backends. These assertions are deliberately
implementation-bound: when phase 5 stops building command lines, they get
deleted alongside the code that built them."
```

---

### Task 5: `internal/config` — ProcessConfig

**Files:**
- Create: `internal/config/processor_test.go`

**Interfaces:**
- Consumes: seamy z úloh 3 a 4 (nepřímo, přes chování)
- Produces: nic

- [ ] **Step 1: Napsat test**

`ProcessConfig` je 27 řádků lepidla, ale rozhoduje o pořadí helm → kustomize a o tom, že bez chartů se helm nevolá vůbec.

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/librucha/krmgen/internal"
)

func TestProcessConfig_NoHelmNoKustomization(t *testing.T) {
	got, err := ProcessConfig(&types.Config{Kind: "KrmGen"}, t.TempDir())
	if err != nil {
		t.Fatalf("ProcessConfig() error = %v", err)
	}
	if got != "" {
		t.Errorf("ProcessConfig() = %q, want empty output", got)
	}
}

func TestProcessConfig_KustomizationWithoutHelm(t *testing.T) {
	dir := t.TempDir()
	kustomization := filepath.Join(dir, "kustomization.yaml")
	content := "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - cm.yaml\n"
	if err := os.WriteFile(kustomization, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cm := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n"
	if err := os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(cm), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := ProcessConfig(&types.Config{Kind: "KrmGen"}, dir)
	if err != nil {
		t.Fatalf("ProcessConfig() error = %v", err)
	}
	if !strings.Contains(got, "kind: ConfigMap") {
		t.Errorf("ProcessConfig() = %q, want the kustomize output", got)
	}
}
```

- [ ] **Step 2: Pustit**

```bash
go test ./internal/config/ -run TestProcessConfig -v
```

Očekávané: PASS. Druhý test volá skutečný `kubectl kustomize`; když kubectl chybí, spadne — to je správně, specifikace ho uvádí jako požadavek.

- [ ] **Step 3: Commit**

```bash
git add internal/config/processor_test.go
git commit -m "test: cover config processing order"
```

---

### Task 6: `cmd` — seam a testy kopírování

**Files:**
- Modify: `cmd/generate.go`
- Modify: `cmd/generate_test.go`

**Interfaces:**
- Produces: `var fatalf = log.Fatalf` v balíčku `cmd`

- [ ] **Step 1: Napsat padající testy**

```go
// fatalSentinel marks a panic raised by the fake fatalf, so an unrelated
// panic inside the code under test is not silently reported as "fatal was
// called" - that would turn a real crash into a passing test.
type fatalSentinel struct{}

func captureFatalf(t *testing.T, call func()) (called bool, message string) {
	t.Helper()
	original := fatalf
	t.Cleanup(func() { fatalf = original })
	fatalf = func(format string, v ...any) {
		called = true
		message = fmt.Sprintf(format, v...)
		panic(fatalSentinel{})
	}
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(fatalSentinel); !ok {
			panic(r) // not our fatal - let the real failure surface
		}
	}()
	call()
	return
}

func TestCopySrcDir_EvaluatesTemplatesExceptSkipped(t *testing.T) {
	src := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("plain.yaml", `value: '{{ kubeEnv "TESTVAR" "fallback" }}'`)
	write("certs/keep.pfx", `raw: {{ this is not a template }}`)

	workDir := copySrcDir(src, []string{"*.pfx"})
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	evaluated, err := os.ReadFile(filepath.Join(workDir, "plain.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(evaluated), "fallback") {
		t.Errorf("template was not evaluated: %q", evaluated)
	}

	skipped, err := os.ReadFile(filepath.Join(workDir, "certs", "keep.pfx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skipped), "{{ this is not a template }}") {
		t.Errorf("skipped file was altered: %q", skipped)
	}
}

func TestCopySrcDir_WorkDirIsPrivate(t *testing.T) {
	workDir := copySrcDir(t.TempDir(), nil)
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	info, err := os.Stat(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("work dir mode = %o, want 0700 - it holds rendered secrets", perm)
	}
}

func TestCopyDir_BrokenTemplateIsFatal(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "bad.yaml"), []byte("{{ .Unclosed "), 0600); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()

	called, message := captureFatalf(t, func() { copyDir(src, dst, src, nil) })
	if !called {
		t.Fatal("expected a broken template to be fatal")
	}
	if !strings.Contains(message, "template evaluation") {
		t.Errorf("message = %q, want it to name template evaluation", message)
	}
}

func TestProcessWorkDir_PrintsOnlyKrmGenFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "not-krmgen.yaml"), []byte("kind: Other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "krmgen.yaml"), []byte("kind: KrmGen\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	processWorkDir(dir)
	_ = w.Close()
	os.Stdout = stdout

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	// A KrmGen file with neither charts nor a kustomization renders nothing,
	// so exactly one empty line is printed for it.
	if got := buf.String(); got != "\n" {
		t.Errorf("stdout = %q, want a single empty line", got)
	}
}
```

Do importů doplň `bytes`, `fmt`, `os`, `path/filepath`, `strings`.

- [ ] **Step 2: Pustit a ověřit selhání**

```bash
go test ./cmd/ -run 'TestCopy|TestProcessWorkDir' -v 2>&1 | tail -5
```

Očekávané: FAIL, `undefined: fatalf`.

- [ ] **Step 3: Přidat seam a použít ho**

V `cmd/generate.go` přidej nad `NewGenerateCommand`:

```go
// fatalf is a seam: tests replace it so a fatal path aborts the call under
// test instead of the test binary.
var fatalf = log.Fatalf
```

Pak nahraď **všech jedenáct** volání `log.Fatalf(` za `fatalf(` a `log.Fatal(err)` v `Run` za `fatalf("%s", err)`.

- [ ] **Step 4: Ověřit, že se hlášky nezměnily**

```bash
# zadne primo volani stdlib log uz v souboru nesmi zbyt
grep -n 'log\.Fatal' cmd/generate.go || echo "zadne log.Fatal - spravne"
go build -o build/krmgen . && ./build/krmgen generate /nonexistent 2>&1 | head -1
```

Očekávané: hláška se od dnešní neliší ničím než tím, že prošla `fatalf`.

- [ ] **Step 5: Pustit testy**

```bash
go test ./cmd/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Očekávané: vše PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate.go cmd/generate_test.go
git commit -m "test: cover source copying and work dir processing

Adds a fatalf seam so the eleven fatal paths can be exercised without
killing the test binary. Messages and exit behaviour are unchanged."
```

---

### Task 7: Zbývající šablonovací providery

Tři funkce nad proměnnými prostředí plus dva poslední Azure providery bez jediného testu.
Vzor pro Azure už v repu existuje dvakrát (`azcert`, `azkey`) — podvržený transport
a naplnění mapy klientů. Zkopíruj ho, nevymýšlej nový.

**Files:**
- Create: `internal/template/argocd/argocd-provider_test.go`
- Create: `internal/template/kube/kube-provider_test.go`
- Create: `internal/template/krmgen/krmgen-provider_test.go`
- Create: `internal/template/azure/storage/azure-storage-provider_test.go`
- Create: `internal/template/azure/identity/azure-identity-provider_test.go`

**Interfaces:**
- Consumes: nic
- Produces: nic

- [ ] **Step 1: Napsat testy pro argocd**

```go
package argocd

import "testing"

func TestResolveArgocdEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "ARGOCD_ENV_ wins", env: map[string]string{"ARGOCD_ENV_X": "env"}, args: []string{"X"}, want: "env"},
		{name: "ARGOCD_APP_ is the fallback", env: map[string]string{"ARGOCD_APP_X": "app"}, args: []string{"X"}, want: "app"},
		{name: "ARGOCD_ENV_ beats ARGOCD_APP_", env: map[string]string{"ARGOCD_ENV_X": "env", "ARGOCD_APP_X": "app"}, args: []string{"X"}, want: "env"},
		{name: "default when unset", args: []string{"X", "fallback"}, want: "fallback"},
		{name: "empty default is honoured", args: []string{"X", ""}, want: ""},
		{name: "error when unset without default", args: []string{"X"}, wantErr: true},
		{name: "no arguments is an error", args: []string{}, wantErr: true},
		{name: "three arguments is an error", args: []string{"X", "a", "b"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := ResolveArgocdEnv(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveArgocdEnv() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ResolveArgocdEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Napsat testy pro kube**

Stejná tabulka, ale prefix je jediný — `KUBE_`:

```go
package kube

import "testing"

func TestResolveKubeEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "reads KUBE_ prefix", env: map[string]string{"KUBE_X": "value"}, args: []string{"X"}, want: "value"},
		{name: "default when unset", args: []string{"X", "fallback"}, want: "fallback"},
		{name: "empty default is honoured", args: []string{"X", ""}, want: ""},
		{name: "error when unset without default", args: []string{"X"}, wantErr: true},
		{name: "no arguments is an error", args: []string{}, wantErr: true},
		{name: "three arguments is an error", args: []string{"X", "a", "b"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := ResolveKubeEnv(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveKubeEnv() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ResolveKubeEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 3: Napsat testy pro krmgen**

```go
package krmgen

import (
	"strings"
	"testing"

	appVer "github.com/librucha/krmgen/version"
)

func TestResolveKrmgenVersion(t *testing.T) {
	original := appVer.AppVersion
	t.Cleanup(func() { appVer.AppVersion = original })
	appVer.AppVersion = "1.2.3"

	got, err := ResolveKrmgenVersion()
	if err != nil {
		t.Fatalf("ResolveKrmgenVersion() error = %v", err)
	}
	if got != "1.2.3" {
		t.Errorf("ResolveKrmgenVersion() = %q, want %q", got, "1.2.3")
	}
}

func TestResolveKrmgenGenerated(t *testing.T) {
	original := appVer.AppVersion
	t.Cleanup(func() { appVer.AppVersion = original })
	appVer.AppVersion = "1.2.3"

	got, err := ResolveKrmgenGenerated()
	if err != nil {
		t.Fatalf("ResolveKrmgenGenerated() error = %v", err)
	}
	if !strings.Contains(got, "1.2.3") {
		t.Errorf("ResolveKrmgenGenerated() = %q, want it to contain the version", got)
	}
}
```

Než tenhle test napíšeš, přečti si `internal/template/krmgen/krmgen-provider.go` a ověř skutečný tvar návratové hodnoty i jméno proměnné s verzí. Pokud se liší, **uprav test podle kódu**, ne naopak.

- [ ] **Step 4: Napsat testy pro Azure storage**

`GetStoreKey` je jediný Azure provider, který bere tři argumenty, a spolu s `azUaIdClientId`
jediný, jehož cache prokazatelně funguje — což test zafixuje.

```go
package azstorage

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

type FakeCredential struct{}

func (f *FakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake_token", ExpiresOn: time.Now().Add(time.Hour).UTC()}, nil
}

type mockSender struct {
	doFunc func(r *http.Request) (*http.Response, error)
}

func (m mockSender) Do(r *http.Request) (*http.Response, error) { return m.doFunc(r) }

func newTestClient(t *testing.T, sender *mockSender) *armstorage.AccountsClient {
	t.Helper()
	options := &arm.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: sender}}
	client, err := armstorage.NewAccountsClient("sub-id", &FakeCredential{}, options)
	if err != nil {
		t.Fatalf("building the test client: %v", err)
	}
	return client
}

func TestGetStoreKey_ReturnsFirstKeyAndCaches(t *testing.T) {
	sender := &mockSender{}
	azureClients["sub-id"] = newTestClient(t, sender)
	cachedKeys = make(map[storageId]*armstorage.AccountKey, 10)

	calls := 0
	sender.doFunc = func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"keys":[{"keyName":"key1","value":"first-key"},{"keyName":"key2","value":"second-key"}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}

	first, err := GetStoreKey("sub-id", "rg", "account")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	second, err := GetStoreKey("sub-id", "rg", "account")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if first != "first-key" || second != "first-key" {
		t.Errorf("got %q and %q, want the first key both times", first, second)
	}
	if calls != 1 {
		t.Errorf("the account was queried %d times, want once - the cache is not holding", calls)
	}
}

func TestGetStoreKey_PropagatesFailure(t *testing.T) {
	sender := &mockSender{}
	azureClients["sub-id"] = newTestClient(t, sender)
	cachedKeys = make(map[storageId]*armstorage.AccountKey, 10)

	sender.doFunc = func(r *http.Request) (*http.Response, error) {
		body := `{"error":{"code":"ResourceNotFound","message":"account not found"}}`
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}

	if _, err := GetStoreKey("sub-id", "rg", "missing"); err == nil {
		t.Fatal("GetStoreKey() error = nil, want the Azure failure to propagate")
	}
}
```

Než to pustíš, ověř skutečná jména proměnných v `azure-storage-provider.go` — mapa klientů
i mapa výsledků. Test výše počítá s `azureClients` a `cachedKeys`; pokud se liší, uprav test.

- [ ] **Step 5: Napsat testy pro Azure identity**

```go
package azid

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
)

type FakeCredential struct{}

func (f *FakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake_token", ExpiresOn: time.Now().Add(time.Hour).UTC()}, nil
}

type mockSender struct {
	doFunc func(r *http.Request) (*http.Response, error)
}

func (m mockSender) Do(r *http.Request) (*http.Response, error) { return m.doFunc(r) }

func newTestClient(t *testing.T, sender *mockSender) *armmsi.UserAssignedIdentitiesClient {
	t.Helper()
	options := &arm.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: sender}}
	client, err := armmsi.NewUserAssignedIdentitiesClient("sub-id", &FakeCredential{}, options)
	if err != nil {
		t.Fatalf("building the test client: %v", err)
	}
	return client
}

func TestGetClientId_ReturnsClientIdAndCaches(t *testing.T) {
	const rg = "my-rg"
	sender := &mockSender{}
	azureClients[rg] = newTestClient(t, sender)
	cachedIdentities = make(map[ID]*armmsi.UserAssignedIdentitiesClientGetResponse, 10)

	calls := 0
	sender.doFunc = func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"id":"/subscriptions/sub-id/resourceGroups/my-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-id","properties":{"clientId":"11111111-1111-1111-1111-111111111111","principalId":"22222222-2222-2222-2222-222222222222","tenantId":"33333333-3333-3333-3333-333333333333"}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}

	first, err := GetClientId(rg, "my-id")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	second, err := GetClientId(rg, "my-id")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	got, ok := first.(*string)
	if !ok || got == nil {
		t.Fatalf("GetClientId() returned %T, want a non-nil *string", first)
	}
	if *got != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("client id = %q, want the value from the response", *got)
	}
	if first != second {
		t.Error("the second call returned a different value than the first")
	}
	if calls != 1 {
		t.Errorf("the identity was queried %d times, want once - the cache is not holding", calls)
	}
}
```

`GetClientId` vrací `any`, konkrétně `*string` z `identity.Properties.ClientID`. Ověř to
v `azure-identity-provider.go` a případně tvrzení uprav — návratový typ je součást kontraktu,
který se ve fázi 3 stěhuje do knihovny.

- [ ] **Step 6: Pustit**

```bash
go test ./internal/template/... -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Očekávané: vše PASS. Pokud některý Azure test spadne na jménu proměnné nebo na tvaru
odpovědi, uprav **test**; produkční kód se v téhle fázi nemění.

- [ ] **Step 7: Ověřit, že Azure testy nechodí na síť**

```bash
go test ./internal/template/azure/... -count=1 2>&1 | grep -E '^(ok|FAIL)'
```

Očekávané: proběhne rychle a projde i bez přihlášení do Azure. Kdyby test čekal na
`DefaultAzureCredential`, znamená to, že se mapa klientů nenaplnila a kód si sáhl pro
skutečného klienta — oprav naplnění mapy.

- [ ] **Step 8: Commit**

```bash
git add internal/template/argocd internal/template/kube internal/template/krmgen \
        internal/template/azure/storage internal/template/azure/identity
git commit -m "test: cover env, version and the last two Azure providers

azStoreKey and azUaIdClientId were the only template functions with no test
at all. Both assert that a repeated lookup hits the cache, which is the
behaviour the recent cache fix established."
```

---

### Task 8: Golden harness a první scénář

Jádro fáze. Harness musí být od začátku psaný tak, aby přidání druhého backendu ve fázi 4 a 5 byl **parametr, ne přepis**.

**Files:**
- Create: `test/golden/harness_test.go`
- Create: `test/golden/charts/demo/Chart.yaml`
- Create: `test/golden/charts/demo/values.yaml`
- Create: `test/golden/charts/demo/templates/configmap.yaml`
- Create: `test/golden/charts/demo/templates/service.yaml`
- Create: `test/golden/fixtures/helm-only/krmgen.yaml`
- Create: `test/golden/fixtures/helm-only/golden.yaml`

**Interfaces:**
- Produces: `runScenario(t *testing.T, name string) result` kde `type result struct { stdout, stderr string; exitCode int }`, `chartRepo(t *testing.T) string`, `binaryPath(t *testing.T) string`, `assertGolden(t *testing.T, name, got string)`, flag `-update` — úlohy 9 a 10 na nich staví

- [ ] **Step 1: Vytvořit deterministický chart**

Nepoužívej `helm create` — generuje desítky řádků, které se nemají co objevit v goldenu. Tenhle chart je záměrně nudný.

`test/golden/charts/demo/Chart.yaml`:

```yaml
apiVersion: v2
name: demo
description: Deterministic chart used by krmgen golden tests
type: application
version: 0.1.0
appVersion: "1.0.0"
```

`test/golden/charts/demo/values.yaml`:

```yaml
replicaCount: 1
message: hello
```

`test/golden/charts/demo/templates/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-demo
data:
  message: {{ .Values.message | quote }}
  replicas: {{ .Values.replicaCount | quote }}
```

`test/golden/charts/demo/templates/service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-demo
spec:
  ports:
    - port: 80
      targetPort: 8080
  selector:
    app: {{ .Release.Name }}-demo
```

Nic v chartu nesmí záviset na čase, náhodě ani na `.Capabilities` — golden by přestal být stabilní.

- [ ] **Step 2: Napsat harness**

`test/golden/harness_test.go`:

```go
package golden

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files with the current output")

// binaryPath builds krmgen once for the whole test run and returns the path.
// Golden scenarios run the real binary as a subprocess: that is the only way
// to observe the true stdout/stderr split and exit codes the specification
// guarantees, and it keeps a fatal path from killing the test binary.
//
// The build is behind a sync.Once - there are a dozen scenarios and rebuilding
// per scenario would dominate the suite's runtime.
var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

func binaryPath(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "krmgen-golden")
		if err != nil {
			buildErr = err
			return
		}
		bin := filepath.Join(dir, "krmgen")
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Dir = repoRoot(t)
		if out, cmdErr := build.CombinedOutput(); cmdErr != nil {
			buildErr = fmt.Errorf("building krmgen: %w\n%s", cmdErr, out)
			return
		}
		builtBin = bin
	})
	if buildErr != nil {
		t.Fatalf("%v", buildErr)
	}
	return builtBin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// chartRepo packages the demo chart into a temporary directory, serves it as
// a plain HTTP chart repository, and returns its base URL. No network is used.
//
// A local chart directory would be simpler, but krmgen cannot address one:
// newGenerator accepts only oci:// and http(s):// repositories.
func chartRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	pkg := exec.Command("helm", "package", filepath.Join(repoRoot(t), "test", "golden", "charts", "demo"), "-d", dir)
	if out, err := pkg.CombinedOutput(); err != nil {
		t.Fatalf("helm package failed: %v\n%s", err, out)
	}
	index := exec.Command("helm", "repo", "index", dir)
	if out, err := index.CombinedOutput(); err != nil {
		t.Fatalf("helm repo index failed: %v\n%s", err, out)
	}

	server := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(server.Close)
	return server.URL
}

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

// runScenario copies the fixture to a temp directory, points it at a local
// chart repository, and runs krmgen against it.
func runScenario(t *testing.T, name string) result {
	t.Helper()

	fixture := filepath.Join("fixtures", name)
	workDir := filepath.Join(t.TempDir(), name)
	if err := os.CopyFS(workDir, os.DirFS(fixture)); err != nil {
		t.Fatalf("copying fixture %s: %v", name, err)
	}
	// golden.yaml is the expectation, not an input
	_ = os.Remove(filepath.Join(workDir, "golden.yaml"))

	cmd := exec.Command(binaryPath(t), "generate", workDir)
	cmd.Env = append(os.Environ(), "ARGOCD_ENV_CHART_REPO="+chartRepo(t))

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return result{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCodeOf(t, err)}
}

// exitCodeOf turns cmd.Run's error into an exit code, failing the test on
// anything that is not a clean non-zero exit (a missing binary, for instance).
func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running krmgen: %v", err)
	}
	return exitErr.ExitCode()
}

// assertGolden compares output against the stored file, or rewrites it with
// -update. A diff here means the product's output changed: read it and decide
// whether that was intended before regenerating.
func assertGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("fixtures", name, "golden.yaml")

	if *update {
		if err := os.WriteFile(path, []byte(got), 0600); err != nil {
			t.Fatalf("updating golden %s: %v", path, err)
		}
		t.Logf("golden updated: %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s: %v (run with -update to create it)", path, err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s\n%s", path, diff(string(want), got))
	}
}

func diff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	var b strings.Builder
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			fmt.Fprintf(&b, "line %d:\n  want: %q\n  got:  %q\n", i+1, w, g)
		}
	}
	return b.String()
}

func TestGolden_HelmOnly(t *testing.T) {
	res := runScenario(t, "helm-only")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "helm-only", res.stdout)
}
```

- [ ] **Step 3: Vytvořit fixture**

`test/golden/fixtures/helm-only/krmgen.yaml`:

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
helm:
  charts:
    - name: demo
      repo: '{{ argocdEnv "CHART_REPO" }}'
      releaseName: rel
      version: 0.1.0
      namespace: default
      ignoreCredentials: true
```

Adresa repozitáře jde dovnitř přes `argocdEnv`, protože `httptest` přiděluje port až za běhu. Je to skutečná produkční funkce, ne testovací zadní vrátka — a scénář tím mimochodem pokrývá i vyhodnocení šablony v konfiguraci.

- [ ] **Step 4: Vygenerovat golden a přečíst si ho**

```bash
go test ./test/golden/ -run TestGolden_HelmOnly -update
cat test/golden/fixtures/helm-only/golden.yaml
```

**Přečti ten soubor celý.** Musí obsahovat ConfigMap a Service s prefixem `rel-`, nic jiného. Když tam najdeš UUID, absolutní cestu nebo časové razítko, máš nedeterminismus — zastav se a nahlas to; golden s náhodným obsahem je horší než žádný.

- [ ] **Step 5: Ověřit, že test bez `-update` prochází a je citlivý**

```bash
go test ./test/golden/ -run TestGolden_HelmOnly -v 2>&1 | tail -3
printf '\n# tampered\n' >> test/golden/fixtures/helm-only/golden.yaml
go test ./test/golden/ -run TestGolden_HelmOnly 2>&1 | tail -5
git checkout test/golden/fixtures/helm-only/golden.yaml
```

Očekávané: nejdřív PASS, po poškození goldenu FAIL s vypsaným rozdílem, po `git checkout` zase PASS. Ten prostřední krok je důkaz, že harness něco hlídá.

- [ ] **Step 6: Commit**

```bash
git add test/golden
git commit -m "test: add golden master harness with the first scenario

Scenarios run the built binary against a hermetic HTTP chart repository
served from httptest. A local chart directory would be simpler but krmgen
cannot address one - newGenerator accepts only oci:// and http(s)://."
```

---

### Task 9: Zbývající scénáře

**Files:**
- Create: `test/golden/fixtures/kustomize-only/{kustomization.yaml,cm.yaml,golden.yaml}`
- Create: `test/golden/fixtures/helm-with-kustomize/{krmgen.yaml,kustomization.yaml,golden.yaml}`
- Create: `test/golden/fixtures/skip-patterns/{krmgen.yaml,certs/raw.pfx,golden.yaml}`
- Create: `test/golden/fixtures/template-functions/{krmgen.yaml,golden.yaml}`
- Modify: `test/golden/harness_test.go`

**Interfaces:**
- Consumes: `runScenario`, `assertGolden`, `chartRepo` z úlohy 8

- [ ] **Step 1: Přidat scénář kustomize bez helmu**

`test/golden/fixtures/kustomize-only/krmgen.yaml`:

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
```

`test/golden/fixtures/kustomize-only/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: from-kustomize
resources:
  - cm.yaml
```

`test/golden/fixtures/kustomize-only/cm.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: standalone
data:
  key: value
```

- [ ] **Step 2: Přidat scénář helm plus kustomize**

`test/golden/fixtures/helm-with-kustomize/krmgen.yaml` je stejný jako u `helm-only`; `kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: overridden
commonLabels:
  managed-by: krmgen
```

Tenhle scénář je nejcennější z celé sady — pokrývá celou cestu i to místo, kde dnešní chyba s bannerem shodila kustomize.

- [ ] **Step 3: Přidat scénář se skip patterny**

`test/golden/fixtures/skip-patterns/krmgen.yaml`:

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
skip:
  - "*.pfx"
helm:
  charts:
    - name: demo
      repo: '{{ argocdEnv "CHART_REPO" }}'
      releaseName: rel
      version: 0.1.0
      ignoreCredentials: true
```

`test/golden/fixtures/skip-patterns/certs/raw.pfx` musí obsahovat text, který by jako šablona spadl:

```
{{ this would not parse as a template }}
```

Že soubor přežil nedotčený, golden neukáže a z vnějšku to ověřit nejde — pracovní adresář
se po běhu maže. Scénář tu tedy dokazuje jen jednu věc: **že běh vůbec neselže**, což by se
bez skip patternu stalo, protože ten obsah není platná šablona. Že se přeskočený soubor
zkopíruje bajt po bajtu, ověřuje `TestCopySrcDir_EvaluatesTemplatesExceptSkipped` v úloze 6.

- [ ] **Step 4: Přidat scénář se šablonovacími funkcemi**

`test/golden/fixtures/template-functions/krmgen.yaml`:

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
helm:
  charts:
    - name: demo
      repo: '{{ argocdEnv "CHART_REPO" }}'
      releaseName: '{{ kubeEnv "RELEASE" "fallback-release" }}'
      version: 0.1.0
      ignoreCredentials: true
      valuesInline:
        message: '{{ argocdEnv "MESSAGE" "default-message" }}'
```

Azure funkce se sem netahají — ty jsou pokryté unit testy s podvrženým transportem.

- [ ] **Step 5: Přidat testy**

```go
func TestGolden_KustomizeOnly(t *testing.T) {
	res := runScenario(t, "kustomize-only")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "kustomize-only", res.stdout)
}

func TestGolden_HelmWithKustomize(t *testing.T) {
	res := runScenario(t, "helm-with-kustomize")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "helm-with-kustomize", res.stdout)
	if strings.Contains(res.stdout, "Pulled:") || strings.Contains(res.stdout, "Digest:") {
		t.Error("helm banner leaked into the output")
	}
}

func TestGolden_SkipPatterns(t *testing.T) {
	res := runScenario(t, "skip-patterns")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "skip-patterns", res.stdout)
}

func TestGolden_TemplateFunctions(t *testing.T) {
	t.Setenv("ARGOCD_ENV_MESSAGE", "from-env")
	res := runScenario(t, "template-functions")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "template-functions", res.stdout)
	if !strings.Contains(res.stdout, "from-env") {
		t.Error("argocdEnv value did not reach the rendered output")
	}
	if !strings.Contains(res.stdout, "fallback-release") {
		t.Error("kubeEnv default did not reach the release name")
	}
}

func TestGolden_StdoutCarriesOnlyYaml(t *testing.T) {
	res := runScenario(t, "helm-with-kustomize")
	for i, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		if strings.HasPrefix(line, "level=") || strings.HasPrefix(line, "time=") {
			t.Errorf("line %d on stdout is a log line, not YAML: %q", i+1, line)
		}
	}
}
```

Pozn.: `runScenario` předává `ARGOCD_ENV_CHART_REPO` přes `cmd.Env`, a `t.Setenv` mění prostředí testovacího procesu, ze kterého `os.Environ()` čte — proto se `ARGOCD_ENV_MESSAGE` do podprocesu dostane.

- [ ] **Step 6: Vygenerovat goldeny a přečíst každý z nich**

```bash
go test ./test/golden/ -update
for f in test/golden/fixtures/*/golden.yaml; do echo "=== $f"; head -20 "$f"; done
```

U každého ověř, že obsahuje jen to, co scénář slibuje. `helm-with-kustomize` musí mít namespace `overridden` a label `managed-by: krmgen` na obou manifestech.

- [ ] **Step 7: Pustit celou sadu bez `-update`**

```bash
go test ./test/golden/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Očekávané: vše PASS.

- [ ] **Step 8: Commit**

```bash
git add test/golden
git commit -m "test: cover the remaining golden scenarios"
```

---

### Task 10: Chybové cesty a OCI za build tagem

**Files:**
- Create: `test/golden/errors_test.go`
- Create: `test/golden/oci_test.go`
- Create: `test/golden/fixtures/two-kustomizations/{krmgen.yaml,kustomization.yaml,nested/kustomization.yaml}`
- Create: `test/golden/fixtures/bad-repo-scheme/krmgen.yaml`

**Interfaces:**
- Consumes: `runScenario` z úlohy 8

- [ ] **Step 1: Vytvořit chybové fixtures**

`test/golden/fixtures/two-kustomizations/krmgen.yaml` obsahuje jen `apiVersion` a `kind: KrmGen`; oba soubory `kustomization.yaml` obsahují prázdnou kustomizaci:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
```

`test/golden/fixtures/bad-repo-scheme/krmgen.yaml`:

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
helm:
  charts:
    - name: demo
      repo: ftp://not.supported/charts
      releaseName: rel
```

- [ ] **Step 2: Napsat testy chybových cest**

Chybové hlášky se neporovnávají celé — obsahují dočasné cesty. Tvrdíme na exit kód a na stabilní část textu.

```go
package golden

import (
	"strings"
	"testing"
)

func TestError_TwoKustomizations(t *testing.T) {
	res := runScenario(t, "two-kustomizations")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "multiple kustomization files") {
		t.Errorf("stderr = %q, want it to name the duplicate kustomization files", res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("stdout = %q, want nothing on a failure", res.stdout)
	}
}

func TestError_UnsupportedRepoScheme(t *testing.T) {
	res := runScenario(t, "bad-repo-scheme")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "not supported by any generator") {
		t.Errorf("stderr = %q, want it to name the unsupported repository", res.stderr)
	}
}

func TestError_MissingPathArgument(t *testing.T) {
	res := runBinary(t, "generate")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
}
```

Do `harness_test.go` doplň pomocníka, kterého poslední test potřebuje:

```go
// runBinary runs krmgen with arbitrary arguments and no fixture.
func runBinary(t *testing.T, args ...string) result {
	t.Helper()
	cmd := exec.Command(binaryPath(t), args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return result{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCodeOf(t, err)}
}
```

- [ ] **Step 3: Ověřit skutečné znění hlášek**

```bash
go build -o /tmp/krmgen-check .
/tmp/krmgen-check generate test/golden/fixtures/two-kustomizations 2>&1 | head -2
/tmp/krmgen-check generate test/golden/fixtures/bad-repo-scheme 2>&1 | head -2
```

Podřetězce v testech **uprav podle skutečného výstupu**, ne naopak.

- [ ] **Step 4: Napsat OCI test za build tagem**

`test/golden/oci_test.go`:

```go
//go:build oci

// These tests reach a public OCI registry. They are excluded from the default
// build because the rest of the suite is hermetic. Run them with:
//
//	go test -tags oci ./test/golden/
//
// A hermetic OCI registry is not possible today: helm requires --plain-http
// for an HTTP registry and --insecure-skip-tls-verify for a self-signed one,
// and krmgen passes neither.
package golden

import (
	"strings"
	"testing"
)

func TestOci_BannerIsStripped(t *testing.T) {
	res := runScenario(t, "oci-public")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	if strings.HasPrefix(res.stdout, "Pulled:") || strings.HasPrefix(res.stdout, "Digest:") {
		t.Error("helm's OCI banner reached stdout")
	}
	if !strings.Contains(res.stdout, "kind:") {
		t.Errorf("stdout does not look like rendered YAML: %q", res.stdout[:min(200, len(res.stdout))])
	}
}
```

`test/golden/fixtures/oci-public/krmgen.yaml`:

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
helm:
  charts:
    - name: reloader
      repo: oci://ghcr.io/stakater/charts/reloader
      releaseName: rel
      namespace: default
      ignoreCredentials: true
```

Tenhle scénář **nemá golden** — verze chartu se v registru mění, takže se tvrdí jen na vlastnosti výstupu.

- [ ] **Step 5: Pustit obojí**

```bash
go test ./test/golden/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
go test -tags oci ./test/golden/ -run TestOci -v 2>&1 | tail -3
```

Očekávané: hermetická sada PASS; OCI test PASS, pokud je síť dostupná.

- [ ] **Step 6: Commit**

```bash
git add test/golden
git commit -m "test: cover error paths and the networked OCI path

Error paths assert on exit code and a stable message fragment, not on whole
stderr, which carries temporary paths. The OCI scenario sits behind a build
tag because a hermetic OCI registry is not reachable: helm needs
--plain-http or --insecure-skip-tls-verify and krmgen passes neither."
```

---

## Dokončení fáze

- [ ] **Plná kontrola**

```bash
make build
go test -race -coverprofile=coverage.out ./... 2>&1 | grep -Ev 'no test files'
go tool cover -func=coverage.out | tail -1
gofmt -l . | grep -v '^$' || echo "formatovani OK"
go vet ./...
```

- [ ] **Vyhodnotit pokrytí proti cíli**

Cíl je ~80 %. Pokud se tam sada nedostane, **nedopisuj testy jen kvůli číslu** — vypiš, co zůstalo nepokryté a proč. `NewGenerateCommand` je cobra lepidlo, které smysluplně otestovat nejde a jehož chování pokrývají golden scénáře skrz binárku.

- [ ] **Ověřit, že CI je zelené**

```bash
git push && sleep 60 && gh run list --limit 2 --json conclusion,workflowName
```

Tohle je první běh CI po opravě z úlohy 1. Dokud nesvítí zeleně, fáze není hotová.

- [ ] **Zapsat výsledek do designového dokumentu**

V `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md` doplň k fázi 2 dosažené pokrytí a počet scénářů; brána fáze zní „goldeny zelené, pokrytí ~80 %".

- [ ] **Rozhodnutí k zaznamenání před fází 3**

1. Zůstane `test/golden` jako umístění, nebo se scénáře přesunou pod `test/resources` k existujícím fixtures?
2. Má se `helm package` volat za běhu testu, nebo se zabalený chart commitne? Dnešní volba je balit za běhu, což vyžaduje helm i pro čistě jednotkový běh sady.
