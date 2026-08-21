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
