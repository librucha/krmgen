package helm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	types "github.com/librucha/krmgen/v2/internal"
	cons "github.com/librucha/krmgen/v2/internal/utils"
)

type fakeTokenCredential struct {
	token  string
	err    error
	scopes []string
	calls  int
}

func (f *fakeTokenCredential) GetToken(_ context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	f.calls++
	f.scopes = opts.Scopes
	if f.err != nil {
		return azcore.AccessToken{}, f.err
	}
	return azcore.AccessToken{Token: f.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// fakeACR wires the Azure credential and the exchange endpoint to fakes and
// returns the number of exchange requests the fake registry received.
func fakeACR(t *testing.T, cred *fakeTokenCredential, handler http.HandlerFunc) *int {
	t.Helper()
	exchanges := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*exchanges++
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	origCred, origURL, origCache := newAzureCredential, acrExchangeURL, acrTokens.byHost
	t.Cleanup(func() {
		newAzureCredential, acrExchangeURL, acrTokens.byHost = origCred, origURL, origCache
	})
	newAzureCredential = func() (azcore.TokenCredential, error) { return cred, nil }
	acrExchangeURL = func(string) string { return srv.URL + "/oauth2/exchange" }
	acrTokens.byHost = map[string]acrResult{}

	t.Setenv(cons.EnvHelmUsername, "")
	t.Setenv(cons.EnvHelmPassword, "")
	return exchanges
}

func exchangeOK(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"refresh_token":"` + token + `"}`))
	}
}

func Test_acrHost(t *testing.T) {
	tests := []struct {
		repoURL  string
		wantHost string
		wantOK   bool
	}{
		{"oci://myacr.azurecr.io/helm", "myacr.azurecr.io", true},
		{"OCI://MyACR.AzureCR.io/helm/chart", "myacr.azurecr.io", true},
		{"oci://myacr.azurecr.io", "myacr.azurecr.io", true},
		{"oci://registry.example.com/helm", "", false},
		{"oci://azurecr.io/helm", "", false},
		{"oci://myacr.azurecr.io.evil.com/helm", "", false},
		{"https://myacr.azurecr.io/helm/v1/repo", "", false},
		{"not a url", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.repoURL, func(t *testing.T) {
			host, ok := acrHost(tt.repoURL)
			if host != tt.wantHost || ok != tt.wantOK {
				t.Errorf("acrHost(%q) = (%q, %v), want (%q, %v)", tt.repoURL, host, ok, tt.wantHost, tt.wantOK)
			}
		})
	}
}

func Test_credentials_ACRWithoutCredentialsUsesAzureIdentity(t *testing.T) {
	cred := &fakeTokenCredential{token: "entra-token"}
	var gotForm map[string]string
	fakeACR(t, cred, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth2/exchange" {
			t.Errorf("exchange request = %s %s, want POST /oauth2/exchange", r.Method, r.URL.Path)
		}
		_ = r.ParseForm()
		gotForm = map[string]string{
			"grant_type":   r.PostForm.Get("grant_type"),
			"service":      r.PostForm.Get("service"),
			"access_token": r.PostForm.Get("access_token"),
		}
		exchangeOK("acr-refresh-token")(w, r)
	})

	u, p := credentials(&types.HelmChart{RepoUrl: "oci://myacr.azurecr.io/helm"})

	if u != acrUsername || p != "acr-refresh-token" {
		t.Errorf("credentials() = (%q, %q), want (%q, %q)", u, p, acrUsername, "acr-refresh-token")
	}
	want := map[string]string{"grant_type": "access_token", "service": "myacr.azurecr.io", "access_token": "entra-token"}
	for k, v := range want {
		if gotForm[k] != v {
			t.Errorf("exchange form %s = %q, want %q", k, gotForm[k], v)
		}
	}
	if len(cred.scopes) != 1 || cred.scopes[0] != acrTokenScope {
		t.Errorf("token scopes = %v, want [%s]", cred.scopes, acrTokenScope)
	}
}

// credentials() runs several times per chart; the exchange must not.
func Test_credentials_ACRTokenIsExchangedOncePerHost(t *testing.T) {
	cred := &fakeTokenCredential{token: "entra-token"}
	exchanges := fakeACR(t, cred, exchangeOK("rt"))
	cfg := &types.HelmChart{RepoUrl: "oci://myacr.azurecr.io/helm"}

	credentials(cfg)
	credentials(cfg)
	credentialsArgs(cfg)
	credentialsProvided(cfg)

	if *exchanges != 1 || cred.calls != 1 {
		t.Errorf("exchanges = %d, token requests = %d, want 1 each", *exchanges, cred.calls)
	}
}

func Test_credentials_ExplicitCredentialsBeatAzureIdentity(t *testing.T) {
	tests := []struct {
		name   string
		cfg    types.HelmChart
		envU   string
		envP   string
		wantU  string
		wantP  string
		ignore bool
	}{
		{name: "config", cfg: types.HelmChart{Username: "admin", Password: "admin-key"}, wantU: "admin", wantP: "admin-key"},
		{name: "env", envU: "env-user", envP: "env-pass", wantU: "env-user", wantP: "env-pass"},
		{name: "partial config is still explicit", cfg: types.HelmChart{Username: "admin"}, wantU: "admin"},
		{name: "ignoreCredentials", cfg: types.HelmChart{IgnoreCredentials: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cred := &fakeTokenCredential{token: "entra-token"}
			exchanges := fakeACR(t, cred, exchangeOK("rt"))
			t.Setenv(cons.EnvHelmUsername, tt.envU)
			t.Setenv(cons.EnvHelmPassword, tt.envP)
			cfg := tt.cfg
			cfg.RepoUrl = "oci://myacr.azurecr.io/helm"

			u, p := credentials(&cfg)

			if u != tt.wantU || p != tt.wantP {
				t.Errorf("credentials() = (%q, %q), want (%q, %q)", u, p, tt.wantU, tt.wantP)
			}
			if *exchanges != 0 || cred.calls != 0 {
				t.Errorf("Azure identity was used (%d exchanges, %d token requests), want none", *exchanges, cred.calls)
			}
		})
	}
}

func Test_credentials_NonACRRegistryNeverTouchesAzure(t *testing.T) {
	cred := &fakeTokenCredential{token: "entra-token"}
	exchanges := fakeACR(t, cred, exchangeOK("rt"))

	for _, repo := range []string{"oci://registry.example.com/helm", "https://charts.example.com"} {
		if u, p := credentials(&types.HelmChart{RepoUrl: repo}); u != "" || p != "" {
			t.Errorf("credentials(%q) = (%q, %q), want none", repo, u, p)
		}
	}
	if *exchanges != 0 || cred.calls != 0 {
		t.Errorf("Azure identity was used (%d exchanges, %d token requests), want none", *exchanges, cred.calls)
	}
}

// Without a usable identity krmgen must behave as before: no credentials,
// so helm falls back to its registry config or an anonymous pull.
func Test_credentials_ACRFallsBackWhenAzureIdentityFails(t *testing.T) {
	tests := []struct {
		name    string
		cred    *fakeTokenCredential
		handler http.HandlerFunc
	}{
		{name: "no token", cred: &fakeTokenCredential{err: errors.New("no identity")}, handler: exchangeOK("rt")},
		{
			name: "exchange rejected",
			cred: &fakeTokenCredential{token: "entra-token"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"errors":[{"code":"UNAUTHORIZED"}]}`, http.StatusUnauthorized)
			},
		},
		{
			name: "empty refresh token",
			cred: &fakeTokenCredential{token: "entra-token"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{}`))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeACR(t, tt.cred, tt.handler)
			cfg := &types.HelmChart{RepoUrl: "oci://myacr.azurecr.io/helm"}

			if u, p := credentials(cfg); u != "" || p != "" {
				t.Errorf("credentials() = (%q, %q), want none", u, p)
			}
			if credentialsProvided(cfg) {
				t.Error("credentialsProvided() = true, want false so the binary backend skips registry login")
			}
		})
	}
}

func Test_exchangeACRToken_ErrorDoesNotEchoTheAccessToken(t *testing.T) {
	fakeACR(t, &fakeTokenCredential{}, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	})

	_, err := exchangeACRToken(context.Background(), "myacr.azurecr.io", "secret-entra-token")

	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want a 403 failure", err)
	}
	if strings.Contains(err.Error(), "secret-entra-token") {
		t.Errorf("error leaked the access token: %v", err)
	}
}
