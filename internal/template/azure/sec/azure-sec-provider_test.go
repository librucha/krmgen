package azsec

import (
	"bytes"
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type mockSender struct {
	doFunc func(r *http.Request) (*http.Response, error)
}

func (m mockSender) Do(r *http.Request) (*http.Response, error) {
	return m.doFunc(r)
}

func newTestClient(sender *mockSender) *azsecrets.Client {
	options := azsecrets.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: sender,
		},
		DisableChallengeResourceVerification: true,
	}
	client, _ := azsecrets.NewClient("https://fake.vault.io", &FakeCredential{}, &options)
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

func TestGetSecret(t *testing.T) {
	type args struct {
		vaultName string
		keyArgs   []string
	}
	tests := []struct {
		name      string
		args      args
		listBody  string // response body for /versions request (used only for no-version calls)
		resBody   string // response body for GetSecret request
		resStatus int
		want      string
		wantErr   bool
	}{
		{
			name: "valid secret",
			args: args{
				vaultName: "vault_name",
				keyArgs:   []string{"key_id"},
			},
			listBody: `{"value":[{"id":"https://fake.vault.io/secrets/key_id/ver1","attributes":{"enabled":true,"created":1000000000}}]}`,
			resBody:  `{"id":"https://fake.vault.io/secrets/key_id/ver1","value":"secretValue"}`,
			want:     "secretValue",
		},
		{
			name: "secret with version",
			args: args{
				vaultName: "vault_name",
				keyArgs:   []string{"key_id", "key_version"},
			},
			resBody: `{"id":"https://vault_name.vault.azure.net/key_id/key_version/","value":"secretValueV1"}`,
			want:    "secretValueV1",
		},
		{
			name: "unknown secret",
			args: args{
				vaultName: "vault_name",
				keyArgs:   []string{"unknown_key_id"},
			},
			resStatus: http.StatusNotFound,
			resBody:   `{"error":{"code":"SecretNotFound","message":"Secret unknown_key_id not found"}}`,
			want:      "",
			wantErr:   true,
		},
		{
			name: "unknown secret version",
			args: args{
				vaultName: "vault_name",
				keyArgs:   []string{"key_id", "unknown_key_version"},
			},
			resStatus: http.StatusNotFound,
			resBody:   `{"error":{"code":"SecretNotFound","message":"Secret key_id/unknown_key_version not found"}}`,
			want:      "",
			wantErr:   true,
		},
	}

	sender := &mockSender{}
	client := newTestClient(sender)
	headers := testHeaders()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			azureClients[tt.args.vaultName] = client
			cachedSecrets = make(map[azsecrets.ID]*azsecrets.Secret, 10)

			status := tt.resStatus
			if status == 0 {
				status = http.StatusOK
			}

			sender.doFunc = func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "/versions") {
					return mockResponse(http.StatusOK, tt.listBody, headers), nil
				}
				return mockResponse(status, tt.resBody, headers), nil
			}

			got, err := GetSecret(tt.args.vaultName, tt.args.keyArgs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetSecret() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLatestActiveSecret(t *testing.T) {
	const vaultName = "vault_name"
	const secretName = "sec"

	tests := []struct {
		name     string
		listBody string
		getBody  string
		want     string
		wantErr  bool
	}{
		{
			name: "returns most recently created active version",
			listBody: `{"value":[
				{"id":"https://fake.vault.io/secrets/sec/ver1","attributes":{"enabled":true,"created":1748200000}},
				{"id":"https://fake.vault.io/secrets/sec/ver2","attributes":{"enabled":true,"created":1748300000}}
			]}`,
			getBody: `{"id":"https://fake.vault.io/secrets/sec/ver2","value":"secret_ver2"}`,
			want:    "secret_ver2",
		},
		{
			name: "skips version with future NotBefore",
			listBody: `{"value":[
				{"id":"https://fake.vault.io/secrets/sec/ver1","attributes":{"enabled":true,"created":1748200000}},
				{"id":"https://fake.vault.io/secrets/sec/ver2","attributes":{"enabled":true,"created":1748300000,"nbf":9999999999}}
			]}`,
			getBody: `{"id":"https://fake.vault.io/secrets/sec/ver1","value":"secret_ver1"}`,
			want:    "secret_ver1",
		},
		{
			name: "returns error when all versions have future NotBefore",
			listBody: `{"value":[
				{"id":"https://fake.vault.io/secrets/sec/ver1","attributes":{"enabled":true,"nbf":9999999999}},
				{"id":"https://fake.vault.io/secrets/sec/ver2","attributes":{"enabled":true,"nbf":9999999999}}
			]}`,
			getBody: ``,
			wantErr: true,
		},
		{
			name: "skips disabled version",
			listBody: `{"value":[
				{"id":"https://fake.vault.io/secrets/sec/ver1","attributes":{"enabled":false,"created":1748200000}},
				{"id":"https://fake.vault.io/secrets/sec/ver2","attributes":{"enabled":true,"created":1748300000}}
			]}`,
			getBody: `{"id":"https://fake.vault.io/secrets/sec/ver2","value":"secret_ver2"}`,
			want:    "secret_ver2",
		},
	}

	sender := &mockSender{}
	client := newTestClient(sender)
	headers := testHeaders()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			azureClients[vaultName] = client
			cachedSecrets = make(map[azsecrets.ID]*azsecrets.Secret, 10)

			sender.doFunc = func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "/versions") {
					return mockResponse(http.StatusOK, tt.listBody, headers), nil
				}
				return mockResponse(http.StatusOK, tt.getBody, headers), nil
			}

			got, err := GetSecret(vaultName, secretName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("GetSecret() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLatestActiveSecret_CachesNoVersionKey(t *testing.T) {
	const vaultName = "vault_name"

	sender := &mockSender{}
	client := newTestClient(sender)
	headers := testHeaders()

	azureClients[vaultName] = client
	cachedSecrets = make(map[azsecrets.ID]*azsecrets.Secret, 10)

	listCalls := 0
	sender.doFunc = func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/versions") {
			listCalls++
			body := `{"value":[{"id":"https://fake.vault.io/secrets/sec/ver1","attributes":{"enabled":true,"created":1748300000}}]}`
			return mockResponse(http.StatusOK, body, headers), nil
		}
		return mockResponse(http.StatusOK, `{"id":"https://fake.vault.io/secrets/sec/ver1","value":"cachedValue"}`, headers), nil
	}

	first, err := GetSecret(vaultName, "sec")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	second, err := GetSecret(vaultName, "sec")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if first != "cachedValue" || second != "cachedValue" {
		t.Errorf("expected both calls to return cachedValue, got %q and %q", first, second)
	}
	if listCalls != 1 {
		t.Errorf("expected list endpoint called once, but was called %d times", listCalls)
	}
}

func TestToPemBlock(t *testing.T) {
	type args struct {
		text      string
		blockType string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "valid",
			args: args{
				text:      string(bytes.Repeat([]byte{13}, 60)),
				blockType: "RSA PRIVATE KEY",
			},
			want:    "-----BEGIN RSA PRIVATE KEY-----\nDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0NDQ0N\nDQ0NDQ0NDQ0NDQ0N\n-----END RSA PRIVATE KEY-----\n",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToPemBlock(tt.args.blockType, tt.args.text)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToPemBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToPemBlock() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type FakeCredential struct{}

func (f *FakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "faketoken", ExpiresOn: time.Now().Add(time.Hour).UTC()}, nil
}
