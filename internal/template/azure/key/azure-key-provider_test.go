package azkey

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

type FakeCredential struct{}

func (f *FakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake_token", ExpiresOn: time.Now().Add(time.Hour).UTC()}, nil
}

type mockSender struct {
	doFunc func(r *http.Request) (*http.Response, error)
}

func (m mockSender) Do(r *http.Request) (*http.Response, error) {
	return m.doFunc(r)
}

func newTestClient(sender *mockSender) *azkeys.Client {
	options := azkeys.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: sender,
		},
		DisableChallengeResourceVerification: true,
	}
	client, _ := azkeys.NewClient("https://fake.vault.io", &FakeCredential{}, &options)
	return client
}

func testHeaders() http.Header {
	h := http.Header{}
	h.Set("WWW-Authenticate", `Bearer authorization="https://login.windows.net/d5069782-a6df-436e-bac4-67b0c78175c8", resource="not_empty"`)
	return h
}

func mockResponse(status int, body string, headers http.Header) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestResolveKey_CachesRepeatedLookups(t *testing.T) {
	const vaultName = "vault_name"

	sender := &mockSender{}
	client := newTestClient(sender)
	headers := testHeaders()

	azureClients[vaultName] = client
	cachedKeys = make(map[azkeys.ID]*azkeys.KeyBundle, 5)

	getCalls := 0
	sender.doFunc = func(r *http.Request) (*http.Response, error) {
		getCalls++
		body := `{"key":{"kid":"https://fake.vault.io/keys/key_name/ver1","kty":"RSA","n":"AQAB","e":"AQAB"}}`
		return mockResponse(http.StatusOK, body, headers), nil
	}

	first, err := ResolveKey(vaultName, "key_name", "ver1")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	second, err := ResolveKey(vaultName, "key_name", "ver1")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if first != second {
		t.Errorf("expected both calls to return the same key, got %q and %q", first, second)
	}
	if !strings.Contains(first.(string), "BEGIN RSA PRIVATE KEY") {
		t.Errorf("expected a PEM block, got %q", first)
	}
	if getCalls != 1 {
		t.Errorf("expected key fetched once, but the vault was called %d times", getCalls)
	}
}
