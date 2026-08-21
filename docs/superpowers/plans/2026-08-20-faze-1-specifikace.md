# Fáze 1: Specifikace kontraktu krmgen — implementační plán

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sepsat ověřený kontrakt krmgenu — CLI, konfigurace, pipeline, šablonovací funkce, podporované verze — aby proti němu šlo měřit, že pozdější výměna helmu a kustomize nic nerozbila.

**Architecture:** Fáze 1 je dokumentační, s jedinou výjimkou mazání mrtvého kódu. Zdrojem pravdy je **chování skutečné binárky**, ne čtení zdrojáků — každé tvrzení ve specifikaci vzniká spuštěním příkazu a zapsáním toho, co se stalo. Jediný spustitelný artefakt fáze je JSON Schema, které se píše TDD proti existujícím fixtures.

**Tech Stack:** Go 1.26, `github.com/santhosh-tekuri/jsonschema/v6` (nová testovací závislost), helm v3 + v4, kubectl, Task

**Spec:** `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`

## Global Constraints

- Go 1.26.0 (`go.mod`), verze nástrojů z `.tool-versions`: `golang 1.26.3`
- **Specifikace se píše anglicky** — README i CLAUDE.md jsou anglicky, produktový dokument musí být konzistentní. (Interní dokumenty v `docs/superpowers/` zůstávají česky.)
- **Fáze 1 nemění chování.** Povolené zásahy do kódu: mazání mrtvého kódu a komentářů. Cokoli, co by změnilo výstup nebo exit kód, patří do fáze 2 a dál.
- Nové soubory se přidávají do gitu (`git add`), commituje se na konci každé úlohy
- Testy se pouští `go test ./...`; plná kontrola `task check`
- Zdroj pravdy pro chování = spuštěná binárka `build/krmgen`, ne zdrojový kód

---

## File Structure

| Soubor | Zodpovědnost |
|---|---|
| `docs/specification.md` | Vytvořit. Produktový kontrakt — jediný dokument, na který se odkazuje zbytek. |
| `resources/krmgen-config-schema.json` | Vytvořit. JSON Schema pro `kind: KrmGen`. Cesta odpovídá té, kterou hledá dnes zakomentovaná validace. |
| `internal/config/schema_test.go` | Vytvořit. Ověřuje, že schéma přijímá všechny fixtures a odmítá vadné konfigurace. |
| `internal/config/parser.go` | Upravit. Smazat zakomentovanou validaci (řádky 71–81). |
| `internal/types.go` | Upravit. Smazat nepoužité `SecretFuncMap` a `SecreteProvider` (řádky 40–47). |
| `CLAUDE.md`, `README.md` | Upravit. Narovnat tři rozpory mezi dokumentací a kódem. |

---

### Task 1: Kostra specifikace a CLI kontrakt

**Files:**
- Create: `docs/specification.md`

**Interfaces:**
- Produces: `docs/specification.md` se sekcemi `## 1. CLI contract` až `## 7. Non-goals`; další úlohy doplňují sekce 2–7 a nemění sekci 1.

- [ ] **Step 1: Postavit aktuální binárku**

```bash
go build -o build/krmgen .
```

Očekávané: bez výstupu, návratový kód 0.

- [ ] **Step 2: Empiricky posbírat chování CLI**

Pusť přesně tyhle příkazy a zapiš si skutečné výstupy a návratové kódy — nepiš je z hlavy:

```bash
./build/krmgen --help; echo "rc=$?"
./build/krmgen generate; echo "rc=$?"
./build/krmgen generate /nonexistent; echo "rc=$?"
./build/krmgen generate test/resources/kustomization-only; echo "rc=$?"
./build/krmgen generate test/resources/kustomization-only 2>/dev/null; echo "pouze stdout rc=$?"
./build/krmgen generate test/resources/kustomization-only --skip='*.yaml'; echo "rc=$?"
```

Ověřená fakta k dnešnímu dni: neexistující cesta → `rc=1`, chybějící argument → `rc=1`.

- [ ] **Step 3: Napsat kostru dokumentu a sekci CLI**

Vytvoř `docs/specification.md` s tímto obsahem. Sekce 2–7 zůstávají jako nadpisy s jednou větou, doplní je další úlohy:

````markdown
# krmgen specification

This document defines the contract of krmgen: what it accepts, what it produces,
and what guarantees hold. It is the reference against which implementation changes
are measured — if behaviour differs from this document, one of the two is a bug.

Version: applies to krmgen 0.x. Status: draft.

## 1. CLI contract

### Commands

| Command | Aliases | Description |
|---|---|---|
| `krmgen generate <path>` | `g` | Render KRM YAML from the config found in `<path>` |

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--skip` | string array, repeatable | none | Glob pattern of files copied without Go template evaluation |

### Streams

| Stream | Content |
|---|---|
| stdout | Rendered YAML, and nothing else |
| stderr | Log messages, warnings, errors |

Anything consuming krmgen may redirect stderr to `/dev/null` and still receive
valid YAML on stdout. This separation is a guarantee, not an implementation detail.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Rendering succeeded; YAML written to stdout |
| 1 | Any error: missing argument, unreadable path, template failure, helm or kustomize failure |

krmgen currently does not distinguish error classes by exit code. Callers must not
rely on a specific non-zero value beyond "non-zero means failure".

## 2. Configuration

To be completed in Task 2.

## 3. Rendering pipeline

To be completed in Task 3.

## 4. Template functions

To be completed in Task 4.

## 5. External tool support matrix

To be completed in Task 5.

## 6. Backend parity exceptions

To be completed in Task 6.

## 7. Non-goals

To be completed in Task 6.
````

- [ ] **Step 4: Ověřit, že dokumentovaná tvrzení sedí**

```bash
./build/krmgen generate 2>/dev/null; test $? -eq 1 && echo "OK: chybejici argument -> 1"
./build/krmgen generate /nonexistent 2>/dev/null; test $? -eq 1 && echo "OK: spatna cesta -> 1"
```

Očekávané: obě `OK:` hlášky. Pokud ne, oprav tabulku exit kódů podle skutečnosti — dokument se přizpůsobuje kódu, ne naopak.

- [ ] **Step 5: Commit**

```bash
git add docs/specification.md
git commit -m "docs: add specification skeleton with CLI contract"
```

---

### Task 2: JSON Schema pro konfiguraci

**Files:**
- Create: `resources/krmgen-config-schema.json`
- Create: `internal/config/schema_test.go`
- Modify: `docs/specification.md` (sekce 2)
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: `docs/specification.md` z úlohy 1
- Produces: `resources/krmgen-config-schema.json` — cesta je závazná, odkazuje na ni specifikace i budoucí runtime validace

- [ ] **Step 1: Přidat testovací závislost**

```bash
go get github.com/santhosh-tekuri/jsonschema/v6
```

- [ ] **Step 2: Napsat padající test**

Vytvoř `internal/config/schema_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const schemaPath = "../../resources/krmgen-config-schema.json"

func loadSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compiling schema %s failed: %v", schemaPath, err)
	}
	return schema
}

// yamlToAny converts a YAML file to the generic structure the validator expects.
func yamlToAny(t *testing.T, path string) any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s failed: %v", path, err)
	}
	var doc any
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshaling %s failed: %v", path, err)
	}
	return doc
}

func TestSchemaAcceptsFixtures(t *testing.T) {
	fixtures := []string{
		"../../test/resources/full/full-krmgen-config.yaml",
		"../../test/resources/kustomization-only/krmgen.yaml",
	}
	schema := loadSchema(t)
	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			if err := schema.Validate(yamlToAny(t, fixture)); err != nil {
				t.Errorf("fixture %s should be valid but was rejected: %v", fixture, err)
			}
		})
	}
}

func TestSchemaRejectsInvalidConfigs(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "wrong kind",
			content: "kind: NotKrmGen\n",
		},
		{
			name:    "chart without name",
			content: "kind: KrmGen\nhelm:\n  charts:\n    - repo: oci://reg.io/helm/app\n",
		},
		{
			name:    "skip is not a list",
			content: "kind: KrmGen\nskip: \"*.pfx\"\n",
		},
		{
			name:    "ignoreCredentials is not a boolean",
			content: "kind: KrmGen\nhelm:\n  charts:\n    - name: app\n      ignoreCredentials: yes-please\n",
		},
	}
	schema := loadSchema(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc any
			if err := yaml.Unmarshal([]byte(tt.content), &doc); err != nil {
				t.Fatalf("unmarshaling test input failed: %v", err)
			}
			if err := schema.Validate(doc); err == nil {
				t.Errorf("config %q should be rejected but passed validation", tt.name)
			}
		})
	}
}
```

- [ ] **Step 3: Pustit test a ověřit, že padá**

```bash
go test ./internal/config/ -run TestSchema -v
```

Očekávané: FAIL, `compiling schema ../../resources/krmgen-config-schema.json failed` — soubor neexistuje.

- [ ] **Step 4: Napsat schéma**

Vytvoř `resources/krmgen-config-schema.json`. Struktura odpovídá `internal/types.go` — YAML tagy, ne jména polí v Go:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/librucha/krmgen/resources/krmgen-config-schema.json",
  "title": "KrmGen configuration",
  "type": "object",
  "required": ["kind"],
  "properties": {
    "apiVersion": {
      "type": "string",
      "description": "API version, e.g. krmgen.config.librucha.com/v1alpha1"
    },
    "kind": {
      "const": "KrmGen",
      "description": "Discriminator identifying a krmgen config file"
    },
    "metadata": {
      "type": "object",
      "properties": {
        "labels": {
          "type": "object",
          "additionalProperties": { "type": "string" }
        },
        "annotations": {
          "type": "object",
          "additionalProperties": { "type": "string" }
        }
      }
    },
    "skip": {
      "type": "array",
      "description": "Glob patterns of files copied without Go template evaluation",
      "items": { "type": "string" }
    },
    "helm": {
      "type": "object",
      "properties": {
        "charts": {
          "type": "array",
          "items": { "$ref": "#/$defs/helmChart" }
        }
      }
    }
  },
  "$defs": {
    "helmChart": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": { "type": "string" },
        "repo": { "type": "string" },
        "ignoreCredentials": { "type": "boolean" },
        "repoUser": { "type": "string" },
        "repoPassword": { "type": "string" },
        "releaseName": { "type": "string" },
        "version": { "type": "string" },
        "namespace": { "type": "string" },
        "valuesFile": { "type": "string" },
        "valuesInline": { "type": "object" }
      }
    }
  }
}
```

- [ ] **Step 5: Pustit test a ověřit, že prochází**

```bash
go test ./internal/config/ -run TestSchema -v
```

Očekávané: PASS ve všech podtestech.

Tohle není odhad — schéma i testy z této úlohy byly předem ověřeny proti oběma skutečným
fixtures i všem čtyřem vadným konfiguracím a chovaly se přesně takhle. Když ti to spadne,
liší se prostředí, ne návrh. Pokud by přesto fixture spadl, **oprav schéma, ne fixture** —
fixtures reprezentují reálné konfigurace uživatelů.

Poznámka k typům: `yaml.v3` vrací `map[string]any`, které validátor přijímá přímo.
Kdyby někdy přibyl fixture s neřetězcovými klíči, převeď dokument před validací přes
`json.Marshal` a `json.Unmarshal`.

- [ ] **Step 6: Doplnit sekci 2 specifikace**

Nahraď v `docs/specification.md` řádek `To be completed in Task 2.` pod `## 2. Configuration`:

````markdown
krmgen looks for config files in the **top level** of the source directory (not
recursively). A file is a krmgen config when its parsed YAML has `kind: KrmGen`.
Every matching file is processed; each produces its own output block.

The machine-readable contract is [`resources/krmgen-config-schema.json`](../resources/krmgen-config-schema.json).

### Example

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
skip:
  - "*.pfx"
helm:
  charts:
    - name: app
      repo: oci://registry.example.com/helm/app
      version: 3.0.0
      releaseName: app
      namespace: default
      valuesFile: values.yaml
      valuesInline:
        replicaCount: 2
```

### Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `apiVersion` | string | no | Informational; not validated against a known set |
| `kind` | string | **yes** | Must be `KrmGen` |
| `metadata.labels` | map[string]string | no | Currently parsed but not applied to output |
| `metadata.annotations` | map[string]string | no | Currently parsed but not applied to output |
| `skip` | []string | no | Glob patterns; see Rendering pipeline |
| `helm.charts[].name` | string | **yes** | Chart name |
| `helm.charts[].repo` | string | no | Repository URL; `oci://` selects the OCI backend, `http(s)://` the repo backend |
| `helm.charts[].version` | string | no | Chart version; omitted means latest |
| `helm.charts[].releaseName` | string | no | Helm release name |
| `helm.charts[].namespace` | string | no | Target namespace |
| `helm.charts[].repoUser` | string | no | Falls back to `KRMGEN_HELM_USERNAME` |
| `helm.charts[].repoPassword` | string | no | Falls back to `KRMGEN_HELM_PASSWORD` |
| `helm.charts[].ignoreCredentials` | bool | no | When true, no credentials are passed even if available |
| `helm.charts[].valuesFile` | string | no | Path relative to the source directory |
| `helm.charts[].valuesInline` | object | no | Written to a temporary values file |

`metadata` being parsed but unused is current behaviour, recorded here so that
changing it is a deliberate decision rather than an accident.
````

- [ ] **Step 7: Pustit celou sadu testů**

```bash
go test ./... 2>&1 | grep -Ev 'no test files'
```

Očekávané: všechny balíčky `ok`.

- [ ] **Step 8: Commit**

```bash
git add resources/krmgen-config-schema.json internal/config/schema_test.go docs/specification.md go.mod go.sum
git commit -m "docs: add JSON schema for KrmGen config with validation tests"
```

---

### Task 3: Sekce o renderovací pipeline

**Files:**
- Modify: `docs/specification.md` (sekce 3)

**Interfaces:**
- Consumes: `docs/specification.md` z úloh 1 a 2

- [ ] **Step 1: Empiricky ověřit pořadí a chování pracovního adresáře**

```bash
./build/krmgen generate test/resources/kustomization-only >/dev/null 2>&1
ls -d ${TMPDIR:-/tmp}/krmgen* 2>/dev/null | wc -l
```

Očekávané: `0` po úspěšném běhu — pracovní adresář se uklidí.

Pak si ověř opačný případ, protože je to dokumentovaná slabina:

```bash
mkdir -p /tmp/krmgen-broken && printf 'kind: KrmGen\nhelm:\n  charts:\n    - name: x\n      repo: bogus://nope\n' > /tmp/krmgen-broken/krmgen.yaml
./build/krmgen generate /tmp/krmgen-broken >/dev/null 2>&1
ls -d ${TMPDIR:-/tmp}/krmgen* 2>/dev/null | wc -l
```

Očekávané: `1` nebo víc — při fatální chybě zůstane adresář na disku. Zapiš skutečné číslo.

- [ ] **Step 2: Uklidit po sobě**

```bash
rm -rf /tmp/krmgen-broken ${TMPDIR:-/tmp}/krmgen*
```

- [ ] **Step 3: Doplnit sekci 3**

Nahraď `To be completed in Task 3.` pod `## 3. Rendering pipeline`:

````markdown
### Order of operations

1. Read `skip` patterns from every `kind: KrmGen` file in the source directory.
   This happens on the **raw YAML**, before any template evaluation — the patterns
   themselves cannot be templated.
2. Merge config-level `skip` patterns with `--skip` flags. Order is preserved,
   duplicates removed, config patterns first.
3. Copy the source directory to a temporary working directory. Every file is
   evaluated as a Go template **except** files matching a skip pattern, which are
   copied byte-for-byte.
4. For each `kind: KrmGen` file in the working directory: run helm for every
   declared chart, concatenating the output in declaration order.
5. If a kustomization file exists anywhere in the working directory, feed the helm
   output into it and let kustomize produce the final result.
6. Write the result to stdout.

### Skip pattern matching

Patterns use `filepath.Match` syntax. Each pattern is tested against **both** the
full relative path and the bare filename, so `*.pfx` matches `certs/prod/cert.pfx`
without needing a directory prefix.

### Kustomization discovery

The working directory is walked recursively for `kustomization.yaml`,
`kustomization.yml` or `kustomization` (case-insensitive). Finding **more than one
is a fatal error** — krmgen will not guess which one you meant.

When a kustomization exists, helm output is written to a file with a generated
name inside the working directory and appended to that kustomization's `resources`
list. The generated filename must never appear in the output.

### Working directory lifecycle

The working directory is created under the system temp directory with mode 0700
and removed after a successful run.

**Known deviation:** when rendering fails, the process exits before cleanup runs,
leaving the working directory — including any rendered secrets — on disk. This is
recorded as current behaviour; it is fixed in phase 3 of the refactoring plan.
````

- [ ] **Step 4: Commit**

```bash
git add docs/specification.md
git commit -m "docs: specify rendering pipeline and working directory lifecycle"
```

---

### Task 4: Referenční příručka šablonovacích funkcí a narovnání dokumentace

**Files:**
- Modify: `docs/specification.md` (sekce 4)
- Modify: `CLAUDE.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: `docs/specification.md` z úloh 1–3
- Produces: seznam jmen funkcí, který se ve fázi 3 stane veřejným API knihovny `cloud-go-templates`

- [ ] **Step 1: Vytáhnout skutečná jména a arity z kódu**

```bash
grep -rhn 'Func = "' --include='*.go' internal/template/
grep -rhn '^func \(GetSecret\|ToPemBlock\|GetPfxKey\|GetPfxCert\|ResolveCert\|ResolveKey\|GetStoreKey\|GetClientId\|ResolveArgocdEnv\|ResolveKubeEnv\|ReadFile\|ResolveKrmgen\)' --include='*.go' internal/template/
```

Ověřená fakta k dnešnímu dni — tři rozpory proti `CLAUDE.md`:

| Dokumentace tvrdí | Kód dělá |
|---|---|
| `azClientId <sub> <group> <name>` | funkce je `azUaIdClientId`, bere 2 argumenty (`rgName`, `idName`) |
| `azStoreKey <account> <group>` | bere 3 argumenty (`subscriptionID`, `resourceGroup`, `accountName`) |
| `KRMGEN_KUBECTL_EXECUTABLE` funguje | konstanta existuje, ale nikde se nepoužívá |

- [ ] **Step 2: Doplnit sekci 4**

Nahraď `To be completed in Task 4.` pod `## 4. Template functions`:

````markdown
Every file copied to the working directory is evaluated as a Go template, unless
it matches a skip pattern. Available functions:

### krmgen

| Function | Arity | Returns |
|---|---|---|
| `krmgenVer` | 0 | Current krmgen version |
| `krmgenGenerated` | 0 | `krmgen-<version>`, intended as a label value |

### Environment

| Function | Arity | Returns |
|---|---|---|
| `argocdEnv <key> [default]` | 1–2 | `ARGOCD_ENV_<key>`, then `ARGOCD_APP_<key>`; error if unset and no default |
| `kubeEnv <key> [default]` | 1–2 | `KUBE_<key>`; error if unset and no default |
| `readF <relpath> [default]` | 1–2 | File contents; path must be relative; error if unreadable and no default |

### Azure

| Function | Arity | Returns |
|---|---|---|
| `azSec <vault> <secret> [version]` | 2–3 | Key Vault secret value |
| `azCert <vault> <cert> [version]` | 2–3 | Key Vault certificate, PEM-encoded |
| `azKey <vault> <key> [version]` | 2–3 | Key Vault key |
| `azPfxKey <vault> <secret>` | 2 | Private key extracted from a PKCS12 secret |
| `azPfxCrt <vault> <secret>` | 2 | Certificate chain extracted from a PKCS12 secret |
| `azStoreKey <subscription> <resourceGroup> <account>` | 3 | Storage account key |
| `azUaIdClientId <resourceGroup> <name>` | 2 | Client ID of a user-assigned managed identity |
| `toPem <type> <data>` | 2 | Wraps bytes in a PEM block |

Omitting the version argument selects the latest **enabled** version whose
`notBefore` is not in the future.

### Sprig

All [sprig](https://masterminds.github.io/sprig/) functions are available, except
`env` and `expandenv`, which are removed so that templates cannot read arbitrary
process environment. Use `argocdEnv` or `kubeEnv` instead.

### Caching

Every Azure lookup is cached in memory for the lifetime of the process, keyed by
the full resource ID. Two references to the same secret cause one network call.
The cache is never invalidated during a run.

### Naming note

`azUaIdClientId` is inconsistent with the other Azure functions. Renaming it to
`azClientId`, keeping the old name as an alias, is proposed for phase 3 — where
the function moves to the `cloud-go-templates` library and a rename can be
released together with the new module.
````

- [ ] **Step 3: Opravit rozpory v `CLAUDE.md`**

V tabulce šablonovacích funkcí:
- `azStoreKey <account> <group>` → `azStoreKey <subscription> <resourceGroup> <account>`
- `azClientId <sub> <group> <name>` → `azUaIdClientId <resourceGroup> <name>`

V tabulce proměnných prostředí u `KRMGEN_KUBECTL_EXECUTABLE` změň popis na:
`Declared but not implemented — kubectl is always invoked from PATH (see specification)`

- [ ] **Step 4: Zkontrolovat stejné rozpory v `README.md`**

```bash
grep -n 'azClientId\|azStoreKey\|KUBECTL_EXECUTABLE' README.md
```

Každý nalezený výskyt oprav stejně jako v kroku 3. Když grep nic nevrátí, krok přeskoč.

- [ ] **Step 5: Ověřit, že dokumentovaná arita sedí**

```bash
printf 'kind: KrmGen\n# {{ azUaIdClientId "rg" "name" }}\n' > /tmp/arity-check.yaml
grep -c 'azUaIdClientId "rg" "name"' /tmp/arity-check.yaml && rm /tmp/arity-check.yaml
```

Očekávané: `1`. Tohle je jen kontrola zápisu — skutečnou aritu jsi ověřil ze signatur v kroku 1.

- [ ] **Step 6: Commit**

```bash
git add docs/specification.md CLAUDE.md README.md
git commit -m "docs: specify template functions and fix documented signatures"
```

---

### Task 5: Matice podporovaných verzí externích nástrojů

**Files:**
- Modify: `docs/specification.md` (sekce 5)

**Interfaces:**
- Consumes: `docs/specification.md` z úloh 1–4
- Produces: seznam rozdílů mezi helm v3 a v4, ze kterého vychází fáze 5

- [ ] **Step 1: Zjistit verze nainstalované lokálně**

```bash
helm version
kubectl version --client | grep -i kustomize
```

Ověřená fakta k dnešnímu dni: helm v4.2.x, kubectl v1.36.3 s vestavěnou Kustomize v5.8.1.

- [ ] **Step 2: Nainstalovat helm v3 vedle v4**

```bash
mkdir -p /tmp/helm3 && curl -sL https://get.helm.sh/helm-v3.21.4-darwin-arm64.tar.gz | tar xz -C /tmp/helm3 --strip-components=1
/tmp/helm3/helm version
```

Očekávané: `Version:"v3.21.4"`. Na Linuxu vyměň `darwin-arm64` za `linux-amd64`.

- [ ] **Step 3: Změřit rozdíl mezi v3 a v4 na OCI chartu**

```bash
for h in /tmp/helm3/helm $(command -v helm); do
  echo "=== $($h version --short 2>/dev/null || $h version)"
  $h template t oci://ghcr.io/stakater/charts/reloader 2>/dev/null | head -3
done
```

Ověřené chování v4: první dva řádky stdout jsou `Pulled:` a `Digest:`, teprve pak `---`.
Zapiš, co udělá v3 — to je ten rozdíl, kvůli kterému matice vzniká.

- [ ] **Step 4: Ověřit, že krmgen zvládne obě verze**

```bash
KRMGEN_HELM_EXECUTABLE=/tmp/helm3/helm ./build/krmgen generate test/resources/kustomization-only >/dev/null 2>&1; echo "helm3 rc=$?"
./build/krmgen generate test/resources/kustomization-only >/dev/null 2>&1; echo "helm4 rc=$?"
```

Očekávané: obojí `rc=0`.

- [ ] **Step 5: Doplnit sekci 5**

Nahraď `To be completed in Task 5.` pod `## 5. External tool support matrix`. Čísla verzí doplň ta, která jsi naměřil:

````markdown
krmgen invokes `helm` and `kubectl` as external binaries. Which binary is used:

| Variable | Effect when set | Effect when unset |
|---|---|---|
| `KRMGEN_HELM_EXECUTABLE` | That path is used as helm | `helm` is looked up in `PATH` |
| `KRMGEN_KUBECTL_EXECUTABLE` | **Not implemented.** Declared but never read | `kubectl` is looked up in `PATH` |

### Supported versions

| Tool | Supported | Verified against |
|---|---|---|
| helm | 3.x, 4.x | v3.21.4, v4.2.x |
| kubectl | 1.2x with embedded Kustomize 5.x | v1.36.3 / Kustomize v5.8.1 |

### Known differences between helm versions

**helm 4 writes OCI progress to stdout.** When a chart is pulled from an OCI
registry, helm 4 prints `Pulled: <ref>` and `Digest: <sha256>` to **stdout**, ahead
of the rendered manifests. helm 3 does not. Because krmgen feeds helm's stdout
into kustomize, these lines would break YAML parsing — krmgen strips a leading run
of `Pulled: `, `Digest: `, `Signed by: ` and `Chart Hash Verified: ` lines from
helm output. Only a contiguous run at the very start is removed, so identical text
inside a rendered manifest is preserved.

**Kustomize version follows kubectl.** The embedded Kustomize version is whatever
the installed kubectl ships. Two hosts with different kubectl versions can produce
different output from the same input. This is the primary reason the refactoring
plan moves to a pinned library.

### Version detection

krmgen does not currently detect or report the version of the external tools it
invokes. Adding a startup check that records both versions is a requirement for
phase 5, where two backends must be told apart in bug reports.
````

- [ ] **Step 6: Uklidit**

```bash
rm -rf /tmp/helm3
```

- [ ] **Step 7: Commit**

```bash
git add docs/specification.md
git commit -m "docs: specify external tool support matrix and helm version differences"
```

---

### Task 6: Výjimky z parity, ne-cíle a smazání mrtvého kódu

**Files:**
- Modify: `docs/specification.md` (sekce 6 a 7)
- Modify: `internal/config/parser.go:71-81`
- Modify: `internal/types.go:40-47`

**Interfaces:**
- Consumes: `docs/specification.md` z úloh 1–5

- [ ] **Step 1: Doplnit sekce 6 a 7**

Nahraď `To be completed in Task 6.` pod `## 6. Backend parity exceptions`:

````markdown
The refactoring plan introduces a second, embedded backend for both helm and
kustomize, selected by the absence of `KRMGEN_HELM_EXECUTABLE` and
`KRMGEN_KUBECTL_EXECUTABLE`. Both backends are measured against the same golden
files. Where they cannot agree, the difference is recorded here rather than
treated as a defect:

| Capability | External binary | Embedded library |
|---|---|---|
| Helm plugins | Available | **Never available** |
| Helm post-renderers | Available | **Never available** |
| Kustomize version | Whatever kubectl ships | Pinned in `go.mod` |
| Helm version | Whatever the host has | Pinned in `go.mod` |

Users depending on helm plugins or post-renderers must set
`KRMGEN_HELM_EXECUTABLE` and keep the external backend.
````

Nahraď `To be completed in Task 6.` pod `## 7. Non-goals`:

````markdown
krmgen deliberately does not:

- **Talk to a cluster.** All rendering is offline. Helm is always run in
  client-only mode; `lookup` in charts returns nothing.
- **Apply anything.** Output goes to stdout; deploying it is the caller's job.
- **Manage secret lifecycle.** Template functions read secrets from Azure at
  render time; krmgen does not create, rotate or write them back.
- **Provide a plugin system.** Template functions are compiled in.
- **Guarantee stable output across versions of the embedded tools.** Upgrading
  the pinned helm or kustomize may change output; such changes are release-noted.
````

- [ ] **Step 2: Smazat zakomentovanou validaci**

V `internal/config/parser.go` smaž tento mrtvý blok (schéma teď existuje na dané cestě, ale zapojení běhové validace patří do pozdější fáze — komentář jen mate):

```go
	// Validate by schema
	// compiler := jsonschema.NewCompiler()
	// compiler.Draft = jsonschema.Draft4
	// schema, err := compiler.Compile("../../resources/krmgen-config-schema.json")
	// if err != nil {
	//	log.Fatal(err)
	// }
	// if err := schema.Validate(config); err != nil {
	//	log.Fatal(err)
	// }
```

- [ ] **Step 3: Smazat nepoužité typy**

V `internal/types.go` smaž tyto typy — grepem je ověřeno, že je nikdo nepoužívá, a `SecreteProvider` je navíc překlep:

```go
type SecretFuncMap struct {
	template.FuncMap
}

type SecreteProvider interface {
	Provide(funcMap *SecretFuncMap)
}
```

Po smazání zůstane import `"text/template"` nepoužitý — smaž ho také.

- [ ] **Step 4: Ověřit, že se to překládá a testy prochází**

```bash
go build ./... && go test ./... 2>&1 | grep -Ev 'no test files'
```

Očekávané: build bez chyb, všechny balíčky `ok`. Kdyby `go vet` hlásil nepoužitý import, smazal jsi typy, ale nechal `"text/template"`.

- [ ] **Step 5: Ověřit, že chování zůstalo stejné**

```bash
go build -o build/krmgen .
./build/krmgen generate test/resources/kustomization-only > /tmp/after.yaml 2>/dev/null; echo "rc=$?"
head -3 /tmp/after.yaml && rm /tmp/after.yaml
```

Očekávané: `rc=0` a stejný výstup jako před úlohou. Fáze 1 nesmí změnit chování.

- [ ] **Step 6: Commit**

```bash
git add docs/specification.md internal/config/parser.go internal/types.go
git commit -m "docs: specify parity exceptions and non-goals, remove dead code"
```

---

## Dokončení fáze

- [ ] **Plná kontrola**

```bash
go build ./... && go test -race ./... 2>&1 | grep -Ev 'no test files'
gofmt -l . | grep -v '^$' || echo "formatovani OK"
```

- [ ] **Revize specifikace člověkem**

`docs/specification.md` je brána fáze 1. Musí ji odsouhlasit vlastník produktu, protože
proti ní se ve fázi 2 píší golden soubory. Chyba ve specifikaci se do goldenů propíše
a zabetonuje.

- [ ] **Rozhodnutí k zaznamenání před fází 2**

Tři otevřené otázky z designového dokumentu, na které fáze 1 dává podklad:

1. Cesta modulu `github.com/librucha/cloud-go-templates` (netýká se fáze 2, ale blokuje fázi 3)
2. Rozsah matice verzí — sekce 5 tvrdí helm 3.x a 4.x; potvrdit, nebo zúžit
3. Přejmenování `azUaIdClientId` → `azClientId` s aliasem ve fázi 3
