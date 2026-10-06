package helm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// Azure Container Registry does not accept a Microsoft Entra token as a
// registry credential directly. The token is first exchanged at the
// registry's /oauth2/exchange endpoint for an ACR refresh token, which is then
// presented as the password of the fixed "null GUID" user - the same pair
// `az acr login` hands to docker. Both helm backends accept that pair like
// any other username/password, so the exchange plugs into credentials()
// without either renderer knowing about Azure.
const (
	acrUsername = "00000000-0000-0000-0000-000000000000"
	// ACR-audience scope: accepted by every registry, unlike an ARM-audience
	// token, which a registry can reject (azureADAuthenticationAsArmPolicy).
	acrTokenScope = "https://containerregistry.azure.net/.default"
	acrHostSuffix = ".azurecr.io"
	acrTimeout    = time.Minute
)

// Seams: tests replace these to avoid real Azure and real registries.
var (
	// DefaultAzureCredential covers Workload Identity (AKS, the Argo CD
	// case), managed identity, the AZURE_* service principal variables and
	// the Azure CLI - the same chain the az* template functions use.
	newAzureCredential = func() (azcore.TokenCredential, error) {
		return azidentity.NewDefaultAzureCredential(nil)
	}
	acrExchangeURL = func(host string) string {
		return "https://" + host + "/oauth2/exchange"
	}
	acrHTTPClient = &http.Client{Timeout: acrTimeout}
)

type acrResult struct {
	refreshToken string
	err          error
}

// acrTokens caches one exchange per registry host for the process lifetime.
// credentials() runs several times per chart (login, flags, the SDK
// client), and a refresh token stays valid for hours - far longer than a
// krmgen run. Failures are cached too, so an unavailable identity costs one
// attempt and one warning, not one per call.
var acrTokens = struct {
	sync.Mutex
	byHost map[string]acrResult
}{byHost: map[string]acrResult{}}

// acrHost returns the registry host of an oci:// chart reference served by
// Azure Container Registry (public cloud).
func acrHost(repoURL string) (string, bool) {
	u, err := url.Parse(repoURL)
	if err != nil || !strings.EqualFold(u.Scheme, "oci") {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, acrHostSuffix) {
		return "", false
	}
	return host, true
}

// acrWorkloadCredentials returns registry credentials obtained through the
// ambient Azure identity when repoURL points at an ACR registry, or empty
// strings otherwise. An identity that cannot produce a token is not an
// error: the caller then behaves exactly as before this existed (helm's
// registry config, or an anonymous pull), and a warning on stderr says why.
func acrWorkloadCredentials(repoURL string) (username, password string) {
	host, ok := acrHost(repoURL)
	if !ok {
		return "", ""
	}

	acrTokens.Lock()
	defer acrTokens.Unlock()
	res, cached := acrTokens.byHost[host]
	if !cached {
		ctx, cancel := context.WithTimeout(context.Background(), acrTimeout)
		res.refreshToken, res.err = acrRefreshToken(ctx, host)
		cancel()
		acrTokens.byHost[host] = res
		if res.err != nil {
			log.Printf("warning: Azure identity login to %s skipped, falling back to helm registry config: %v", host, res.err)
		}
	}
	if res.err != nil {
		return "", ""
	}
	return acrUsername, res.refreshToken
}

func acrRefreshToken(ctx context.Context, host string) (string, error) {
	cred, err := newAzureCredential()
	if err != nil {
		return "", fmt.Errorf("no Azure credential: %w", err)
	}
	tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{acrTokenScope}})
	if err != nil {
		return "", fmt.Errorf("getting Microsoft Entra token failed: %w", err)
	}
	return exchangeACRToken(ctx, host, tok.Token)
}

// exchangeACRToken trades a Microsoft Entra access token for an ACR refresh
// token (POST /oauth2/exchange, grant_type=access_token).
func exchangeACRToken(ctx context.Context, host, accessToken string) (string, error) {
	form := url.Values{
		"grant_type":   {"access_token"},
		"service":      {host},
		"access_token": {accessToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, acrExchangeURL(host), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := acrHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ACR token exchange failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// The error body names the reason (e.g. an unknown tenant) and never
		// echoes the submitted token; cap it all the same.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("ACR token exchange failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decoding ACR token exchange response failed: %w", err)
	}
	if payload.RefreshToken == "" {
		return "", errors.New("ACR token exchange returned no refresh token")
	}
	return payload.RefreshToken, nil
}
