# Fáze 3: Vytažení Azure funkcí do knihovny cloud-go-templates — implementační plán

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Přesunout Azure šablonovací funkce z krmgenu do samostatné knihovny s API, které nestojí na globálním stavu, aniž by se komukoli změnil jediný `krmgen.yaml`.

**Architecture:** Knihovna vystaví **jeden balíček `azure` s jedním typem `Provider`**. Konstruktor bere `context.Context` a volitelně credential; klienti i cache jsou pole instance, ne globální mapy. `FuncMap()` je až výstup. Jména šablonovacích funkcí se nemění, s jedinou výjimkou přejmenování rozhodnutého v fázi 1. krmgen knihovnu během vývoje konzumuje přes `replace`.

**Tech Stack:** Go 1.26, Azure SDK (azsecrets, azcertificates, azkeys, armstorage, armmsi, armsubscriptions), `golang.org/x/crypto/pkcs12`

**Spec:** `docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md` (rozhodnutí R4 a sekce „Fáze 2 — výsledek"), produktová specifikace `docs/specification.md` sekce 4

## Global Constraints

- Go 1.26.0 v obou repozitářích; `.tool-versions` krmgenu uvádí `golang 1.26.3`
- Kód, komentáře i dokumentace **anglicky**
- **Jména šablonovacích funkcí se nemění.** Existující `krmgen.yaml` musí fungovat beze změny. Jediná výjimka: `azUaIdClientId` → `azUserIdentityClientId`, přičemž **krmgen si staré jméno ponechá jako deprecated alias** (rozhodnuto 2026-08-21, viz specifikace sekce 4, „Naming note")
- **Žádný test nesmí na síť ani potřebovat Azure credentials.** Vzor s podvrženým transportem existuje v krmgenu pětkrát; v knihovně bude jednou
- Golden sada krmgenu musí po celou fázi zůstat zelená
- Commituje se výčtem cest, nikdy `git add -A`
- **Nepushovat.** Push je rozhodnutí uživatele
- Dva repozitáře: knihovna `/Users/librucha/Projects/Personal/cloud-go-templates`, krmgen `/Users/librucha/Projects/Personal/krmgen`. Každý má vlastní větev a vlastní commity

---

## Výchozí stav, ověřený 2026-08-25

| Fakt | Hodnota |
|---|---|
| Azure kód ke stěhování | 612 řádků |
| Azure testy ke stěhování | 850 řádků |
| Co v krmgenu zůstává | `template.go` (70), `argocd` (40), `files` (35), `kube` (32), `krmgen` (16) |
| Globální mapy k odstranění | **10** — pět na klienty, pět na výsledky |
| `log.Fatal` v template balíčcích | **žádný**, funkce už chyby vracejí |
| Stav knihovny | jediný commit `7c5b356`, obsahuje jen `go.mod` a `.gitignore` |
| Vazba template balíčků na krmgen | jen `version` (používá ji krmgenový provider, který zůstává) |

### Rozhodnutí, se kterými plán pracuje

- **Stěhuje se jen Azure.** `argocdEnv`, `kubeEnv`, `readF` a `krmgenVer` nejsou cloudové funkce a v knihovně jménem cloud-go-templates nemají co dělat.
- **Entra se nestěhuje vůbec** (rozhodnuto 2026-08-25). Odpadá tím i závislost na `msgraph-sdk-go`. Rozpracovaný Entra kód zůstává mimo repozitář v `~/Projects/Personal/cloud-go-templates-entra-wip/`.
- **`log.Fatal` → návratové chyby není součástí téhle fáze.** Designový dokument to k fázi 3 přiřadil, ale v template balíčcích žádný `log.Fatal` není — všech 26 výskytů je v krmgenu samotném (`cmd`, `kustomize`, `config`, `helm`). Je to jiná práce s jiným rizikem a dostane vlastní plán.

---

## Konvence tohohle plánu

Většina kódu se **přenáší, nepíše**. Tam, kde úloha říká „přenes z originálu", je uvedená
přesná cesta ke zdroji a přesné podpisy, které mají vzniknout — ale samotné tělo funkce se
tu neopisuje. Je to záměr: 600 řádků přepsaných v plánu je 600 příležitostí k překlepu,
který by se pak tvářil jako zamýšlená změna chování. Originál je autorita; plán říká, co se
kolem něj mění.

Nové kódy, které v originálu nemají obdobu (`Provider`, `cache`, volby, testovací harness),
jsou v plánu vypsané celé.

## File Structure

### Knihovna `cloud-go-templates`

| Soubor | Zodpovědnost |
|---|---|
| `LICENSE` | Vytvořit. MIT. |
| `README.md` | Vytvořit. K čemu knihovna je, jak se používá, referenční seznam funkcí. |
| `azure/provider.go` | Vytvořit. `Provider`, `New`, `FuncMap`, líné vytváření credentials. |
| `azure/options.go` | Vytvořit. `Option`, `WithCredential`, `WithSubscriptionID`. |
| `azure/cache.go` | Vytvořit. Typované cache jako pole instance, chráněné mutexem. |
| `azure/secrets.go` | Vytvořit. `azSec`, `toPem`. |
| `azure/pkcs12.go` | Vytvořit. `azPfxKey`, `azPfxCrt`. |
| `azure/certificates.go` | Vytvořit. `azCert`. |
| `azure/keys.go` | Vytvořit. `azKey`. |
| `azure/storage.go` | Vytvořit. `azStoreKey`. |
| `azure/identity.go` | Vytvořit. `azUserIdentityClientId`. |
| `azure/subscription.go` | Vytvořit. Dohledání subscription ID. |
| `azure/testing_test.go` | Vytvořit. **Jeden** sdílený mock transport pro všechny testy balíčku. |
| `azure/*_test.go` | Vytvořit. Testy vedle svých funkcí. |

**Proč jeden balíček a ne šest:** dnešních šest balíčků existuje jen proto, aby každý mohl mít vlastní globální mapy. S instancí, která drží klienty i cache, je to zbytečné dělení — a hlavně by šest konstruktorů znamenalo šest `FuncMap()`, které si konzument musí slučovat sám.

### krmgen

| Soubor | Zodpovědnost |
|---|---|
| `go.mod`, `go.sum` | Upravit. Přidat knihovnu, během vývoje přes `replace`. |
| `internal/template/template.go` | Upravit. Registrovat funkce z knihovny místo z `internal/template/azure`. |
| `internal/template/template_test.go` | Upravit. Přibude test na množinu registrovaných jmen. |
| `internal/template/azure/**` | **Smazat.** 612 řádků kódu a 850 řádků testů odchází s knihovnou. |
| `CLAUDE.md`, `docs/specification.md` | Upravit. Strom architektury a referenční tabulka funkcí. |

---

### Task 1: Jádro knihovny a první funkce

Nejdůležitější úloha celé fáze: ustaví tvar, který zbytek jen následuje. Proto s sebou nese jednu skutečnou funkci — bez ní by se nedalo poznat, jestli ten tvar funguje.

**Files:**
- Create: `LICENSE`, `README.md`, `azure/provider.go`, `azure/options.go`, `azure/cache.go`, `azure/secrets.go`, `azure/testing_test.go`, `azure/secrets_test.go`
- Modify: `go.mod`

**Interfaces:**
- Produces: `azure.New(ctx context.Context, opts ...Option) (*Provider, error)`, `(*Provider).FuncMap() template.FuncMap`, `azure.WithCredential(azcore.TokenCredential) Option`, a testovací pomocníci `newTestProvider(t *testing.T, sender *mockSender, extra ...Option) *Provider` a `mockResponse(status int, body string) *http.Response`. Úlohy 2 až 5 na nich staví.

Pracuj v `/Users/librucha/Projects/Personal/cloud-go-templates` na větvi `feat/azure-provider`.

- [ ] **Step 1: Založit větev a licenci**

```bash
cd /Users/librucha/Projects/Personal/cloud-go-templates
git checkout -b feat/azure-provider
```

`LICENSE` — MIT, rok 2026, držitel `Libor Ondrušek`. Použij standardní text MIT beze změn. (Pokud uživatel licenci ještě nepotvrdil, je to jediný soubor, který se mění.)

- [ ] **Step 2: Napsat padající test pro konstruktor a FuncMap**

`azure/testing_test.go` — sdílený mock transport. V krmgenu je tenhle harness pětkrát, jednou v každém Azure balíčku; tady stačí jednou:

```go
package azure

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// fakeCredential stands in for a real Azure credential so no test needs
// credentials or a network.
type fakeCredential struct{}

func (fakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake_token", ExpiresOn: time.Now().Add(time.Hour).UTC()}, nil
}

// mockSender records how many requests were made and answers each one with
// whatever doFunc returns.
type mockSender struct {
	calls  int
	doFunc func(r *http.Request) (*http.Response, error)
}

func (m *mockSender) Do(r *http.Request) (*http.Response, error) {
	m.calls++
	return m.doFunc(r)
}

func mockResponse(status int, body string) *http.Response {
	h := http.Header{}
	// Key Vault clients perform an authentication challenge before the real
	// request; without this header the SDK rejects the response.
	h.Set("WWW-Authenticate", `Bearer authorization="https://login.windows.net/d5069782-a6df-436e-bac4-67b0c78175c8", resource="not_empty"`)
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// newTestProvider builds a Provider whose every Azure client talks to sender
// instead of the network.
func newTestProvider(t *testing.T, sender *mockSender, extra ...Option) *Provider {
	t.Helper()
	opts := append([]Option{
		WithCredential(fakeCredential{}),
		withTransport(sender),
	}, extra...)
	p, err := New(context.Background(), opts...)
	if err != nil {
		t.Fatalf("building the test provider: %v", err)
	}
	return p
}
```

`azure/provider_test.go`:

```go
package azure

import (
	"context"
	"testing"
)

func TestNew_RequiresContext(t *testing.T) {
	if _, err := New(nil); err == nil { //nolint:staticcheck // passing nil is the thing under test
		t.Error("New(nil) error = nil, want an error - the context is not optional")
	}
}

func TestFuncMap_ExposesEveryFunctionUnderItsTemplateName(t *testing.T) {
	p, err := New(context.Background(), WithCredential(fakeCredential{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	funcs := p.FuncMap()
	// Only azSec and toPem exist at this point; later tasks extend this list
	// and the test with it.
	for _, name := range []string{"azSec", "toPem"} {
		if _, ok := funcs[name]; !ok {
			t.Errorf("FuncMap() is missing %q", name)
		}
	}
}

func TestProvider_CachesRepeatedLookups(t *testing.T) {
	sender := &mockSender{doFunc: func(r *http.Request) (*http.Response, error) {
		return mockResponse(200, `{"id":"https://fake.vault.io/secrets/sec/ver1","value":"cached"}`), nil
	}}
	p := newTestProvider(t, sender)

	for i := 0; i < 2; i++ {
		got, err := p.Secret("vault", "sec", "ver1")
		if err != nil {
			t.Fatalf("call %d failed: %v", i+1, err)
		}
		if got != "cached" {
			t.Errorf("call %d = %q, want %q", i+1, got, "cached")
		}
	}
	if sender.calls != 1 {
		t.Errorf("the vault was called %d times, want once", sender.calls)
	}
}
```

Do importů `provider_test.go` doplň `net/http`.

- [ ] **Step 3: Pustit a ověřit RED**

```bash
go test ./azure/ 2>&1 | tail -5
```

Očekávané: FAIL, `undefined: New`. Kdyby to hlásilo něco jiného, oprav to dřív, než začneš psát implementaci.

- [ ] **Step 4: Napsat `azure/options.go`**

```go
package azure

import "github.com/Azure/azure-sdk-for-go/sdk/azcore"

// Option configures a Provider.
type Option func(*config)

type config struct {
	credential     azcore.TokenCredential
	subscriptionID string
	transport      azcore.Transporter
}

// WithCredential supplies the credential every Azure client will use. Without
// it the provider falls back to azidentity.NewDefaultAzureCredential, resolved
// lazily on first use so that constructing a Provider never touches the
// network or the filesystem.
func WithCredential(cred azcore.TokenCredential) Option {
	return func(c *config) { c.credential = cred }
}

// WithSubscriptionID pins the subscription used by the functions that need
// one. Without it the provider reads AZURE_SUBSCRIPTION_ID, and failing that
// asks Azure for the first subscription the credential can see.
func WithSubscriptionID(id string) Option {
	return func(c *config) { c.subscriptionID = id }
}

// withTransport is unexported: it exists so the package's own tests can point
// every client at a stub. Consumers configure transports through their
// credential or environment instead.
func withTransport(t azcore.Transporter) Option {
	return func(c *config) { c.transport = t }
}
```

- [ ] **Step 5: Napsat `azure/cache.go`**

```go
package azure

import "sync"

// cache holds every resolved lookup for the lifetime of a Provider. Entries
// are never invalidated: a template render is short-lived, and re-reading a
// secret mid-render would be surprising rather than helpful.
//
// Keys are always built from the function's own arguments, never from the
// resource ID Azure returns. Those two disagree - Azure's IDs carry a
// collection segment such as /secrets/ - and keying on the response made
// every entry unreachable in an earlier version of this code.
type cache struct {
	mu      sync.Mutex
	entries map[string]any
}

func newCache() *cache {
	return &cache{entries: make(map[string]any, 32)}
}

func (c *cache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[key]
	return v, ok
}

func (c *cache) put(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = value
}
```

- [ ] **Step 6: Napsat `azure/provider.go`**

```go
// Package azure provides Go template functions that read secrets,
// certificates, keys and identifiers from Azure.
//
// A Provider owns its clients and its cache, so two providers never share
// state. Construct one, take its FuncMap, and hand that to text/template.
package azure

import (
	"context"
	"errors"
	"sync"
	"text/template"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// Provider resolves Azure resources for template functions.
type Provider struct {
	// ctx is stored deliberately. Go template functions receive only their
	// arguments, so there is no call site to thread a context through; the
	// lifetime of a render is the lifetime of the provider.
	ctx context.Context
	cfg config

	credOnce sync.Once
	cred     azcore.TokenCredential
	credErr  error

	clientsMu sync.Mutex
	clients   map[string]any

	cache *cache
}

// New builds a Provider. It performs no I/O: credentials and clients are
// created on first use.
func New(ctx context.Context, opts ...Option) (*Provider, error) {
	if ctx == nil {
		return nil, errors.New("azure: New requires a non-nil context")
	}
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Provider{
		ctx:     ctx,
		cfg:     cfg,
		clients: make(map[string]any, 8),
		cache:   newCache(),
	}, nil
}

// FuncMap returns the template functions this provider exposes, keyed by the
// name templates call them under.
func (p *Provider) FuncMap() template.FuncMap {
	return template.FuncMap{
		"azSec": p.SecretFunc,
		"toPem": ToPem,
	}
}

// credential resolves the credential once, lazily.
func (p *Provider) credential() (azcore.TokenCredential, error) {
	p.credOnce.Do(func() {
		if p.cfg.credential != nil {
			p.cred = p.cfg.credential
			return
		}
		p.cred, p.credErr = azidentity.NewDefaultAzureCredential(nil)
	})
	return p.cred, p.credErr
}

// client returns the client stored under key, or builds one with build and
// stores it. Clients are reused for the provider's lifetime.
func (p *Provider) client(key string, build func(azcore.TokenCredential) (any, error)) (any, error) {
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()
	if c, ok := p.clients[key]; ok {
		return c, nil
	}
	cred, err := p.credential()
	if err != nil {
		return nil, err
	}
	c, err := build(cred)
	if err != nil {
		return nil, err
	}
	p.clients[key] = c
	return c, nil
}
```

- [ ] **Step 7: Napsat `azure/secrets.go`**

Přenes logiku z krmgenu `internal/template/azure/sec/azure-sec-provider.go`, včetně dohledání nejnovější aktivní verze. Změna oproti originálu je jen v tom, odkud se berou klienti a cache:

```go
package azure

import (
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

// SecretFunc is the azSec template function: azSec <vault> <name> [version].
func (p *Provider) SecretFunc(vaultName string, args ...string) (any, error) {
	switch len(args) {
	case 1:
		return p.Secret(vaultName, args[0], "")
	case 2:
		return p.Secret(vaultName, args[0], args[1])
	default:
		return nil, fmt.Errorf("wrong arguments count for function %q: expected 1 or 2 arguments after the vault name, got %d", "azSec", len(args))
	}
}

// Secret reads one secret. An empty version selects the most recently created
// enabled version whose notBefore is not in the future.
func (p *Provider) Secret(vaultName, name, version string) (string, error) {
	key := cacheKey("secret", vaultName, name, version)
	if v, ok := p.cache.get(key); ok {
		return v.(string), nil
	}

	client, err := p.secretsClient(vaultName)
	if err != nil {
		return "", err
	}

	if version == "" {
		resolved, err := p.latestActiveSecretVersion(client, vaultName, name)
		if err != nil {
			return "", err
		}
		version = resolved
	}

	secret, err := client.GetSecret(p.ctx, name, version, nil)
	if err != nil {
		return "", err
	}
	if secret.Value == nil {
		return "", fmt.Errorf("secret %q in vault %q has no value", name, vaultName)
	}

	// Cache under the version that was asked for and under the one that was
	// resolved, so a later call naming that version hits too.
	p.cache.put(key, *secret.Value)
	p.cache.put(cacheKey("secret", vaultName, name, version), *secret.Value)
	return *secret.Value, nil
}

func (p *Provider) secretsClient(vaultName string) (*azsecrets.Client, error) {
	c, err := p.client("secrets:"+vaultName, func(cred azcore.TokenCredential) (any, error) {
		return azsecrets.NewClient(vaultURL(vaultName), cred, &azsecrets.ClientOptions{
			ClientOptions: azcore.ClientOptions{Transport: p.cfg.transport},
		})
	})
	if err != nil {
		return nil, err
	}
	return c.(*azsecrets.Client), nil
}

// latestActiveSecretVersion returns the newest enabled version whose
// notBefore has passed.
func (p *Provider) latestActiveSecretVersion(client *azsecrets.Client, vaultName, name string) (string, error) {
	now := time.Now().UTC()
	var best *azsecrets.SecretProperties

	pager := client.NewListSecretPropertiesVersionsPager(name, nil)
	for pager.More() {
		page, err := pager.NextPage(p.ctx)
		if err != nil {
			return "", fmt.Errorf("listing versions of secret %q in vault %q: %w", name, vaultName, err)
		}
		for _, item := range page.Value {
			if item == nil || item.Attributes == nil || item.ID == nil {
				continue
			}
			if item.Attributes.Enabled != nil && !*item.Attributes.Enabled {
				continue
			}
			if item.Attributes.NotBefore != nil && item.Attributes.NotBefore.UTC().After(now) {
				continue
			}
			if best == nil || newerSecret(item, best) {
				best = item
			}
		}
	}
	if best == nil {
		return "", fmt.Errorf("no active version found for secret %q in vault %q", name, vaultName)
	}
	return best.ID.Version(), nil
}

func newerSecret(a, b *azsecrets.SecretProperties) bool {
	if a.Attributes.Created == nil {
		return false
	}
	if b.Attributes.Created == nil {
		return true
	}
	return a.Attributes.Created.After(*b.Attributes.Created)
}

// ToPem is the toPem template function: it wraps bytes in a PEM block of the
// given type. It needs no Azure access, so it is a plain function.
func ToPem(blockType, text string) (string, error) {
	return string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: []byte(text)})), nil
}

func vaultURL(vaultName string) string {
	return fmt.Sprintf("https://%v.vault.azure.net", vaultName)
}

// cacheKey builds a key from the caller's own arguments. Never key on an ID
// returned by Azure - see cache.go.
func cacheKey(kind string, parts ...string) string {
	return kind + ":" + strings.Join(parts, "/")
}
```

- [ ] **Step 8: Doplnit závislosti a pustit testy**

```bash
go mod tidy
go test ./azure/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Očekávané: všechny PASS. Pokud `TestProvider_CachesRepeatedLookups` hlásí dvě volání, klíč pro čtení a zápis se rozchází — sjednoť je, neupravuj test.

- [ ] **Step 9: Napsat README**

`README.md` s tímhle obsahem, doplněným o seznam funkcí, které dnes existují (zatím `azSec` a `toPem`; úlohy 2 až 4 seznam rozšíří):

````markdown
# cloud-go-templates

Go template functions that read configuration out of public clouds, in the
spirit of [sprig](https://github.com/Masterminds/sprig): you build a `FuncMap`
and hand it to `text/template`.

Today only Azure is implemented.

## Usage

```go
provider, err := azure.New(ctx)
if err != nil {
	return err
}

tmpl := template.New("example").Funcs(provider.FuncMap())
```

A `Provider` owns its Azure clients and its cache. Two providers share
nothing, so a test or a second render can have its own.

Credentials come from `azidentity.NewDefaultAzureCredential` unless you pass
`azure.WithCredential`. Nothing touches the network until the first function
call.

## Functions

| Function | Arity | Returns |
|---|---|---|
| `azSec <vault> <name> [version]` | 2–3 | Key Vault secret value |
| `toPem <type> <data>` | 2 | The data wrapped in a PEM block |

Omitting the version selects the most recently created enabled version whose
`notBefore` has passed.

## Caching

Every lookup is cached for the lifetime of the provider and keyed by the
function's own arguments. Nothing is invalidated mid-render.
````

- [ ] **Step 10: Commit**

```bash
git add LICENSE README.md go.mod go.sum azure/
git commit -m "feat: add the Azure provider with secret lookup

A Provider owns its clients and cache instead of the package-level maps this
code used in krmgen, so nothing is shared between instances and the cache can
be reasoned about. Credentials resolve lazily, so building one does no I/O."
```

---

### Task 2: PKCS12 funkce

**Files:**
- Create: `azure/pkcs12.go`, `azure/pkcs12_test.go`
- Modify: `azure/provider.go` (registrace do `FuncMap`), `README.md`

**Interfaces:**
- Consumes: `(*Provider).Secret`, `newTestProvider`, `mockResponse` z úlohy 1
- Produces: `(*Provider).PfxKeyFunc`, `(*Provider).PfxCertFunc`

- [ ] **Step 1: Přečíst originál**

```bash
sed -n '1,93p' /Users/librucha/Projects/Personal/krmgen/internal/template/azure/sec/azure-pkcs12-provider.go
sed -n '1,148p' /Users/librucha/Projects/Personal/krmgen/internal/template/azure/sec/azure-pkcs12-provider_test.go
```

Přenášíš chování jedna ku jedné. Jediná změna je, že se secret bere přes `p.Secret(...)` místo přes globální funkci.

- [ ] **Step 2: Napsat padající testy**

Testovací data z originálu (base64 PKCS12) přenes beze změny — vygenerovat nová by znamenalo netestovat totéž. Přidej k nim testy, které originál neměl:

```go
func TestPfxKeyFunc_WrongArgumentCount(t *testing.T) {
	p := newTestProvider(t, &mockSender{doFunc: func(*http.Request) (*http.Response, error) {
		return mockResponse(200, `{"value":"ignored"}`), nil
	}})
	if _, err := p.PfxKeyFunc("vault"); err == nil {
		t.Error("PfxKeyFunc with no secret name error = nil, want an arity error")
	}
	if _, err := p.PfxKeyFunc("vault", "a", "b", "c"); err == nil {
		t.Error("PfxKeyFunc with four arguments error = nil, want an arity error")
	}
}

func TestPfxKeyFunc_NotAPkcs12Payload(t *testing.T) {
	p := newTestProvider(t, &mockSender{doFunc: func(*http.Request) (*http.Response, error) {
		return mockResponse(200, `{"id":"https://fake.vault.io/secrets/pfx/v1","value":"bm90LWEtcGZ4"}`), nil
	}})
	if _, err := p.PfxKeyFunc("vault", "pfx", "v1"); err == nil {
		t.Error("PfxKeyFunc on a non-PKCS12 secret error = nil, want a decode error")
	}
}
```

- [ ] **Step 3: Pustit a ověřit RED**

```bash
go test ./azure/ -run TestPfx 2>&1 | tail -5
```

Očekávané: FAIL, `undefined: PfxKeyFunc`.

- [ ] **Step 4: Napsat `azure/pkcs12.go`**

Přenes obě funkce z originálu. Podpis metod:

```go
// PfxKeyFunc is the azPfxKey template function: azPfxKey <vault> <secret> [version].
func (p *Provider) PfxKeyFunc(vaultName string, args ...string) (any, error)

// PfxCertFunc is the azPfxCrt template function: azPfxCrt <vault> <secret> [version].
func (p *Provider) PfxCertFunc(vaultName string, args ...string) (any, error)
```

Chybové hlášky nech znít jako v originálu, jen oprav překlepy, pokud tam jsou — ty se do veřejného API nepřenášejí.

- [ ] **Step 5: Zaregistrovat do FuncMap**

V `azure/provider.go` doplň do `FuncMap()`:

```go
		"azPfxKey": p.PfxKeyFunc,
		"azPfxCrt": p.PfxCertFunc,
```

a rozšiř seznam v `TestFuncMap_ExposesEveryFunctionUnderItsTemplateName` o obě jména.

- [ ] **Step 6: Pustit testy**

```bash
go test ./azure/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Očekávané: vše PASS.

- [ ] **Step 7: Doplnit README a commitnout**

Do tabulky funkcí přidej `azPfxKey <vault> <secret> [version]` a `azPfxCrt <vault> <secret> [version]`.

```bash
git add azure/pkcs12.go azure/pkcs12_test.go azure/provider.go azure/provider_test.go README.md
git commit -m "feat: add PKCS12 key and certificate extraction"
```

---

### Task 3: Certifikáty a klíče

**Files:**
- Create: `azure/certificates.go`, `azure/certificates_test.go`, `azure/keys.go`, `azure/keys_test.go`
- Modify: `azure/provider.go`, `README.md`

**Interfaces:**
- Consumes: `(*Provider).client`, `p.cache`, `newTestProvider`, `mockResponse`
- Produces: `(*Provider).CertificateFunc`, `(*Provider).KeyFunc`

- [ ] **Step 1: Přečíst originály**

```bash
cat /Users/librucha/Projects/Personal/krmgen/internal/template/azure/cert/azure-cert-provider.go
cat /Users/librucha/Projects/Personal/krmgen/internal/template/azure/key/azure-key-provider.go
```

Všimni si, co `wrapKey` doopravdy dělá: PEM-obalí `key.Key.N`, tedy **RSA modulus**, pod hlavičkou `"<KTY> PRIVATE KEY"`. Není to privátní klíč. Chování se v téhle fázi nemění, ale musí být popsané v komentáři i v README, aby to příští čtenář nepovažoval za slib.

- [ ] **Step 2: Napsat padající testy**

```go
func TestCertificateFunc_CachesRepeatedLookups(t *testing.T) {
	sender := &mockSender{doFunc: func(*http.Request) (*http.Response, error) {
		return mockResponse(200, `{"id":"https://fake.vault.io/certificates/crt/v1","cer":"YmFzZTY0Y2VydA=="}`), nil
	}}
	p := newTestProvider(t, sender)

	first, err := p.CertificateFunc("vault", "crt", "v1")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	second, err := p.CertificateFunc("vault", "crt", "v1")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if first != second {
		t.Errorf("got %v then %v, want the same certificate", first, second)
	}
	if !strings.Contains(first.(string), "BEGIN CERTIFICATE") {
		t.Errorf("got %q, want a PEM certificate", first)
	}
	if sender.calls != 1 {
		t.Errorf("the vault was called %d times, want once", sender.calls)
	}
}

func TestKeyFunc_ReturnsModulusUnderAPrivateKeyHeader(t *testing.T) {
	sender := &mockSender{doFunc: func(*http.Request) (*http.Response, error) {
		return mockResponse(200, `{"key":{"kid":"https://fake.vault.io/keys/k/v1","kty":"RSA","n":"AQAB","e":"AQAB"}}`), nil
	}}
	p := newTestProvider(t, sender)

	got, err := p.KeyFunc("vault", "k", "v1")
	if err != nil {
		t.Fatalf("KeyFunc() error = %v", err)
	}
	// Documented oddity, preserved deliberately: the payload is the RSA
	// modulus, not a private key, despite the header.
	if !strings.Contains(got.(string), "BEGIN RSA PRIVATE KEY") {
		t.Errorf("got %q, want the RSA PRIVATE KEY header today's code emits", got)
	}
	if sender.calls != 1 {
		t.Errorf("the vault was called %d times, want once", sender.calls)
	}
}
```

- [ ] **Step 3: Pustit a ověřit RED**

```bash
go test ./azure/ -run 'TestCertificateFunc|TestKeyFunc' 2>&1 | tail -5
```

Očekávané: FAIL, `undefined: CertificateFunc`.

- [ ] **Step 4: Napsat obě implementace a zaregistrovat je**

Podpisy:

```go
// CertificateFunc is the azCert template function: azCert <vault> <name> [version].
func (p *Provider) CertificateFunc(vaultName string, args ...string) (any, error)

// KeyFunc is the azKey template function: azKey <vault> <name> [version].
func (p *Provider) KeyFunc(vaultName string, args ...string) (any, error)
```

Do `FuncMap()` přidej `"azCert": p.CertificateFunc` a `"azKey": p.KeyFunc`, do testu množiny jmen obě jména.

- [ ] **Step 5: Pustit testy a commitnout**

```bash
go test ./azure/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
git add azure/certificates.go azure/certificates_test.go azure/keys.go azure/keys_test.go azure/provider.go azure/provider_test.go README.md
git commit -m "feat: add Key Vault certificate and key lookup

azKey emits the RSA modulus under an RSA PRIVATE KEY header. That is what
krmgen has always done; it is preserved here and documented rather than
quietly corrected, because changing it would change rendered output."
```

---

### Task 4: Storage a managed identity

Tahle úloha nese **jedinou vědomou změnu chování celé fáze**.

**Files:**
- Create: `azure/storage.go`, `azure/storage_test.go`, `azure/identity.go`, `azure/identity_test.go`, `azure/subscription.go`
- Modify: `azure/provider.go`, `README.md`

**Interfaces:**
- Consumes: `(*Provider).client`, `p.cache`, `newTestProvider`, `mockResponse`
- Produces: `(*Provider).StorageKeyFunc`, `(*Provider).UserIdentityClientIDFunc`, `(*Provider).subscriptionID()`

- [ ] **Step 1: Přečíst originály**

```bash
cat /Users/librucha/Projects/Personal/krmgen/internal/template/azure/storage/azure-storage-provider.go
cat /Users/librucha/Projects/Personal/krmgen/internal/template/azure/identity/azure-identity-provider.go
cat /Users/librucha/Projects/Personal/krmgen/internal/template/azure/commons/subscription.go
```

- [ ] **Step 2: Napsat padající testy, včetně testu na opravovanou chybu**

Originál dělá `keys.Keys[0]` bez kontroly délky a `*keys.Keys[0].Value` bez kontroly na nil. Prázdná odpověď z Azure tedy shodí celý proces panikou. **To se v téhle fázi opravuje** — funkce nově vrátí chybu:

```go
func TestStorageKeyFunc_ReturnsFirstKey(t *testing.T) {
	sender := &mockSender{doFunc: func(*http.Request) (*http.Response, error) {
		return mockResponse(200, `{"keys":[{"keyName":"key1","value":"first"},{"keyName":"key2","value":"second"}]}`), nil
	}}
	p := newTestProvider(t, sender)

	got, err := p.StorageKeyFunc("sub", "rg", "account")
	if err != nil {
		t.Fatalf("StorageKeyFunc() error = %v", err)
	}
	if got != "first" {
		t.Errorf("got %q, want the first key", got)
	}
}

// The version of this code that shipped in krmgen indexed Keys[0] without
// checking the length, so an account with no keys panicked and took the whole
// process down. This is the one deliberate behaviour change in this phase.
func TestStorageKeyFunc_NoKeysIsAnErrorNotAPanic(t *testing.T) {
	sender := &mockSender{doFunc: func(*http.Request) (*http.Response, error) {
		return mockResponse(200, `{"keys":[]}`), nil
	}}
	p := newTestProvider(t, sender)

	_, err := p.StorageKeyFunc("sub", "rg", "account")
	if err == nil {
		t.Fatal("StorageKeyFunc() error = nil, want an error for an account with no keys")
	}
	if !strings.Contains(err.Error(), "account") {
		t.Errorf("error = %v, want it to name the storage account", err)
	}
}

func TestUserIdentityClientIDFunc_ReturnsClientID(t *testing.T) {
	sender := &mockSender{doFunc: func(*http.Request) (*http.Response, error) {
		return mockResponse(200, `{"id":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id","properties":{"clientId":"11111111-1111-1111-1111-111111111111"}}`), nil
	}}
	p := newTestProvider(t, sender, WithSubscriptionID("sub"))

	got, err := p.UserIdentityClientIDFunc("rg", "id")
	if err != nil {
		t.Fatalf("UserIdentityClientIDFunc() error = %v", err)
	}
	if got != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("got %v, want the client id from the response", got)
	}
	if sender.calls != 1 {
		t.Errorf("the identity was queried %d times, want once", sender.calls)
	}
}
```

Pozn.: `UserIdentityClientIDFunc` vrací `string`, ne `*string` jako originál v krmgenu.
**Vyrenderovaný výstup se tím nemění** — `text/template` ukazatele při tisku dereferencuje,
takže šablona vidí obě varianty stejně. Změna je jen v tom, že se nil ošetří na jednom místě
uvnitř funkce místo aby se propsal do šablony.

- [ ] **Step 3: Pustit a ověřit RED**

```bash
go test ./azure/ -run 'TestStorageKeyFunc|TestUserIdentity' 2>&1 | tail -5
```

Očekávané: FAIL, `undefined: StorageKeyFunc`.

- [ ] **Step 4: Napsat implementace**

Podpisy:

```go
// StorageKeyFunc is the azStoreKey template function:
// azStoreKey <subscription> <resourceGroup> <account>.
func (p *Provider) StorageKeyFunc(subscriptionID, resourceGroup, account string) (string, error)

// UserIdentityClientIDFunc is the azUserIdentityClientId template function:
// azUserIdentityClientId <resourceGroup> <name>. It resolves user-assigned
// identities only; system-assigned ones are addressed by scope and would need
// a different function.
func (p *Provider) UserIdentityClientIDFunc(resourceGroup, name string) (string, error)

// subscriptionID returns the configured subscription, else
// AZURE_SUBSCRIPTION_ID, else the first subscription the credential can see.
func (p *Provider) subscriptionID() (string, error)
```

Prázdný seznam klíčů musí dát chybu, která pojmenuje účet — ne panic.

- [ ] **Step 5: Zaregistrovat a pustit testy**

Do `FuncMap()`: `"azStoreKey": p.StorageKeyFunc` a `"azUserIdentityClientId": p.UserIdentityClientIDFunc`. Doplň obě jména do testu množiny.

```bash
go test ./azure/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'
```

- [ ] **Step 6: Commit**

```bash
git add azure/storage.go azure/storage_test.go azure/identity.go azure/identity_test.go azure/subscription.go azure/provider.go azure/provider_test.go README.md
git commit -m "feat: add storage account key and managed identity lookup

An account with no keys now returns an error. The version this was lifted
from indexed Keys[0] unchecked and panicked, taking the process with it."
```

---

### Task 5: Souběžnost, kontrakt FuncMap a dokumentace

**Files:**
- Create: `azure/concurrency_test.go`
- Modify: `azure/provider_test.go`, `README.md`

**Interfaces:**
- Consumes: vše z úloh 1 až 4

- [ ] **Step 1: Napsat test souběžnosti**

Knihovna musí říct, jestli je bezpečná pro souběžné použití. Tenhle test to buď dokáže, nebo pod `-race` spadne:

```go
package azure

import (
	"net/http"
	"sync"
	"testing"
)

func TestProvider_IsSafeForConcurrentUse(t *testing.T) {
	sender := &mockSender{doFunc: func(*http.Request) (*http.Response, error) {
		return mockResponse(200, `{"id":"https://fake.vault.io/secrets/sec/v1","value":"v"}`), nil
	}}
	p := newTestProvider(t, sender)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Secret("vault", "sec", "v1"); err != nil {
				t.Errorf("concurrent Secret() failed: %v", err)
			}
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Pustit pod race detektorem**

```bash
go test -race ./azure/ -run TestProvider_IsSafeForConcurrentUse -count=1 -v 2>&1 | tail -8
```

Očekávané: PASS bez hlášení závodu. Pokud race detektor něco najde, chybí zámek — přidej ho do `client()` nebo `cache`, ne do testu.

Pozn.: `mockSender.calls` v tomhle testu **není** chráněný a záměrně se na něj netvrdí; kdybys to potřeboval, přidej mu vlastní mutex.

- [ ] **Step 3: Dotáhnout test množiny funkcí**

`TestFuncMap_ExposesEveryFunctionUnderItsTemplateName` musí teď obsahovat úplný seznam a navíc hlídat, že tam nic navíc není — je to veřejný kontrakt knihovny:

```go
func TestFuncMap_ExposesEveryFunctionUnderItsTemplateName(t *testing.T) {
	p, err := New(context.Background(), WithCredential(fakeCredential{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	want := []string{
		"azSec", "toPem", "azPfxKey", "azPfxCrt",
		"azCert", "azKey", "azStoreKey", "azUserIdentityClientId",
	}
	funcs := p.FuncMap()
	for _, name := range want {
		if _, ok := funcs[name]; !ok {
			t.Errorf("FuncMap() is missing %q", name)
		}
	}
	if len(funcs) != len(want) {
		got := make([]string, 0, len(funcs))
		for name := range funcs {
			got = append(got, name)
		}
		sort.Strings(got)
		t.Errorf("FuncMap() exposes %d functions %v, want exactly %d %v", len(funcs), got, len(want), want)
	}
}
```

Do importů doplň `sort`.

- [ ] **Step 4: Dotáhnout README**

Tabulka funkcí musí obsahovat všech osm, k tomu sekce o souběžnosti („a Provider is safe for concurrent use") a poznámka u `azKey`, že vrací modulus pod hlavičkou privátního klíče.

- [ ] **Step 5: Plná kontrola a commit**

```bash
go build ./... && go test -race -count=1 ./... 2>&1 | tail -3
gofmt -l . | grep -v '^$' || echo "gofmt cisto"
go vet ./...
go mod tidy -diff >/dev/null 2>&1 && echo "moduly kanonicke"
git add azure/concurrency_test.go azure/provider_test.go README.md
git commit -m "test: prove the provider is safe for concurrent use

Also pins the FuncMap's exact contents: it is the library's public surface
and a function appearing or vanishing should fail a test, not surprise a
consumer."
```

---

### Task 6: Napojení krmgenu

Od téhle úlohy se pracuje v `/Users/librucha/Projects/Personal/krmgen` na větvi `refactor/faze-3-knihovna`.

**Files:**
- Modify: `go.mod`, `go.sum`, `internal/template/template.go`
- Delete: `internal/template/azure/**` (celý strom)

**Interfaces:**
- Consumes: `azure.New`, `(*Provider).FuncMap` z úloh 1 až 5
- Produces: krmgen bez vlastního Azure kódu

- [ ] **Step 1: Založit větev a napojit knihovnu přes replace**

```bash
cd /Users/librucha/Projects/Personal/krmgen
git checkout -b refactor/faze-3-knihovna
go mod edit -require=github.com/librucha/cloud-go-templates@v0.0.0
go mod edit -replace=github.com/librucha/cloud-go-templates=../cloud-go-templates
```

`replace` je dočasný: knihovna zatím nemá vydanou verzi. Poslední úloha to řeší.

- [ ] **Step 2: Přepsat registraci funkcí**

V `internal/template/template.go` nahraď pět Azure importů jedním a osm registrací blokem z knihovny. Alias na staré jméno zůstává, jinak by se rozbily existující konfigurace:

**Provider se staví jednou za proces, ne jednou za soubor.** `EvalGoTemplates` volá
`cmd/generate.go:118` pro každý kopírovaný soubor zvlášť. Kdyby provider vznikal uvnitř
`initFuncs`, dostal by každý soubor vlastní cache — a stejný secret zmíněný ve třech
souborech by se z Azure stáhl třikrát. Specifikace přitom slibuje cache **na běh procesu**.

Přidej proto do `internal/template/template.go`:

```go
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
```

a v `initFuncs`:

```go
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
```

Do importů doplň `context`, `sync` a `"github.com/librucha/cloud-go-templates/azure"`.

`initFuncs` tím začne vracet chybu, takže se mění dva podpisy:

```go
// pred
func initFuncs(t *template.Template)

// po
func initFuncs(t *template.Template) error
```

a v `EvalGoTemplates` se volání změní z

```go
	initFuncs(t)
```

na

```go
	if err := initFuncs(t); err != nil {
		return "", err
	}
```

Chování při chybě se tím nemění: `EvalGoTemplates` chyby vrací už dnes a volající je hlásí
přes `log.Fatalf`.

- [ ] **Step 3: Smazat vystěhovaný kód**

```bash
git rm -r internal/template/azure
```

- [ ] **Step 4: Přeložit a pustit testy**

```bash
go mod tidy
go build ./... && go test -count=1 ./... 2>&1 | grep -E 'FAIL|^ok'
```

Očekávané: vše `ok`. Pokud něco nejde přeložit, zůstal někde import na smazaný balíček.

- [ ] **Step 5: Ověřit, že se výstup nezměnil**

```bash
go test ./test/golden/ -count=1 -v 2>&1 | grep -E '^--- '
```

Očekávané: všech 14 testů PASS **bez regenerace goldenů**. Goldeny Azure funkce nepokrývají, takže zelené být musí; kdyby spadly, rozbil se někde jiný kus registrace funkcí, a to je vážné.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/template/template.go
git commit -m "refactor: take Azure template functions from cloud-go-templates

Deletes 612 lines of provider code and 850 lines of its tests, which now
live in the library. azUaIdClientId stays registered as a deprecated alias
of azUserIdentityClientId so existing configurations keep working."
```

---

### Task 7: Důkaz, že se nic nezměnilo, a dokumentace

**Files:**
- Modify: `internal/template/template_test.go`, `CLAUDE.md`, `docs/specification.md`

**Interfaces:**
- Consumes: krmgen napojený na knihovnu z úlohy 6

- [ ] **Step 1: Napsat test na množinu registrovaných jmen**

Tohle je skutečná regresní pojistka téhle fáze. Goldeny Azure nepokrývají, takže bez tohohle testu by špatně přepsaná registrace prošla nepovšimnutá:

```go
func TestEvalGoTemplates_RegistersEveryDocumentedFunction(t *testing.T) {
	// Every name docs/specification.md section 4 documents, plus the
	// deprecated alias krmgen keeps for backward compatibility.
	names := []string{
		"krmgenVer", "krmgenGenerated",
		"argocdEnv", "kubeEnv", "readF",
		"azSec", "toPem", "azPfxKey", "azPfxCrt",
		"azCert", "azKey", "azStoreKey", "azUserIdentityClientId",
		"azUaIdClientId",
		// a sprig function, to prove sprig is still merged in
		"upper",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			// A template that only references the function without calling it
			// parses if and only if the function is registered.
			if _, err := EvalGoTemplates("{{ if false }}{{ " + name + " }}{{ end }}"); err != nil {
				if strings.Contains(err.Error(), "function \"" + name + "\" not defined") {
					t.Errorf("template function %q is not registered", name)
				}
			}
		})
	}
}

func TestEvalGoTemplates_DoesNotRegisterEnvFunctions(t *testing.T) {
	// sprig's env and expandenv are removed deliberately: templates must not
	// read arbitrary process environment.
	for _, name := range []string{"env", "expandenv"} {
		_, err := EvalGoTemplates("{{ if false }}{{ " + name + " \"X\" }}{{ end }}")
		if err == nil || !strings.Contains(err.Error(), "not defined") {
			t.Errorf("%q must not be registered, got err = %v", name, err)
		}
	}
}
```

- [ ] **Step 2: Pustit**

```bash
go test ./internal/template/ -run TestEvalGoTemplates -v 2>&1 | grep -E '^(=== RUN|--- |ok|FAIL)' | head -25
```

Očekávané: vše PASS. Kdyby některé jméno chybělo, registrace v úloze 6 něco vynechala.

- [ ] **Step 3: Narovnat dokumentaci**

V `CLAUDE.md`:
- ve stromu architektury nahraď celý podstrom `azure/` jedním řádkem, který říká, že Azure funkce přicházejí z `github.com/librucha/cloud-go-templates`
- v tabulce šablonovacích funkcí přejmenuj `azUaIdClientId` na `azUserIdentityClientId` a doplň řádek, že staré jméno zůstává jako deprecated alias

V `docs/specification.md` sekce 4:
- totéž přejmenování a poznámka o aliasu
- v „Naming note" nahraď formulaci o budoucím přejmenování konstatováním, že k němu došlo, a uveď od kdy
- v sekci Caching nahraď popis šesti cest popisem toho, co dělá knihovna: klíč se staví z argumentů volání, cache žije po dobu života provideru, provider je bezpečný pro souběžné použití

- [ ] **Step 4: Plná kontrola**

```bash
make build
go test -race -count=1 ./... 2>&1 | grep -E 'FAIL|^ok'
gofmt -l . | grep -v '^$' || echo "gofmt cisto"
go vet ./...
go tool cover -func=<(go test -coverprofile=/dev/stdout ./... 2>/dev/null) 2>/dev/null | tail -1 || go test -coverprofile=/tmp/c.out ./... >/dev/null 2>&1 && go tool cover -func=/tmp/c.out | tail -1
```

Pokrytí krmgenu **klesne** — 850 řádků Azure testů odešlo do knihovny. To je správně, ne regrese; zapiš skutečné číslo do reportu.

- [ ] **Step 5: Commit**

```bash
git add internal/template/template_test.go CLAUDE.md docs/specification.md
git commit -m "test: pin the registered template function names

The goldens do not cover Azure functions, so a botched rewiring would
otherwise pass unnoticed. Also renames azUaIdClientId to
azUserIdentityClientId in the docs and records the alias."
```

---

## Dokončení fáze

- [ ] **Obě sady zelené**

```bash
cd /Users/librucha/Projects/Personal/cloud-go-templates && go test -race -count=1 ./...
go test -race -count=1 ./...
```

- [ ] **Goldeny beze změny**

```bash
cd /Users/librucha/Projects/Personal/krmgen && git status --porcelain test/golden/fixtures/
```

Očekávané: prázdný výstup. Jediný golden se v téhle fázi měnit nesměl.

- [ ] **Rozhodnutí, které zbývá uživateli**

`replace` v `go.mod` krmgenu ukazuje na lokální adresář. Zrušit ho jde teprve tehdy, až bude knihovna **pushnutá a otagovaná** — a push je rozhodnutí uživatele. Do té doby krmgen nejde postavit nikde jinde než na tomhle stroji. Napiš to do závěrečného hlášení jako první věc, ne jako poznámku pod čarou.

- [ ] **Zapsat výsledek do designového dokumentu**

K fázi 3 doplň dosažený stav: kolik řádků odešlo, jaké je pokrytí obou repozitářů, a že jediná vědomá změna chování byla oprava paniky v `azStoreKey`.
