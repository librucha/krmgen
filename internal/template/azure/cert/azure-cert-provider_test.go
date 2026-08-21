package azcert

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates"
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

func newTestClient(sender *mockSender) *azcertificates.Client {
	options := azcertificates.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: sender,
		},
		DisableChallengeResourceVerification: true,
	}
	client, _ := azcertificates.NewClient("https://fake.vault.io", &FakeCredential{}, &options)
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

func TestResolveCert_CachesRepeatedLookups(t *testing.T) {
	const vaultName = "vault_name"

	sender := &mockSender{}
	client := newTestClient(sender)
	headers := testHeaders()

	azureClients[vaultName] = client
	cachedCerts = make(map[azcertificates.ID]*azcertificates.Certificate, 5)

	getCalls := 0
	sender.doFunc = func(r *http.Request) (*http.Response, error) {
		getCalls++
		body := `{"id":"https://fake.vault.io/certificates/cert_name/ver1","cer":"YmFzZTY0Y2VydA=="}`
		return mockResponse(http.StatusOK, body, headers), nil
	}

	first, err := ResolveCert(vaultName, "cert_name", "ver1")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	second, err := ResolveCert(vaultName, "cert_name", "ver1")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if first != second {
		t.Errorf("expected both calls to return the same certificate, got %q and %q", first, second)
	}
	if !strings.Contains(first.(string), "BEGIN CERTIFICATE") {
		t.Errorf("expected a PEM certificate, got %q", first)
	}
	if getCalls != 1 {
		t.Errorf("expected certificate fetched once, but the vault was called %d times", getCalls)
	}
}
