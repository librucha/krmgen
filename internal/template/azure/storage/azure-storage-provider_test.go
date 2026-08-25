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
