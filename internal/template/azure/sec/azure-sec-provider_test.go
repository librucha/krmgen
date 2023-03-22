package azsec

import (
	"bytes"
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
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

func TestGetSecret(t *testing.T) {
	type args struct {
		vaultName string
		keyArgs   []string
	}
	tests := []struct {
		name      string
		args      args
		resBody   string
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
			resBody: `{"id":"https://vault_name.vault.azure.net/key_id/","value":"secretValue"}`,
			want:    "secretValue",
			wantErr: false,
		},
		{
			name: "secret with version",
			args: args{
				vaultName: "vault_name",
				keyArgs:   []string{"key_id", "key_version"},
			},
			resBody: `{"id":"https://vault_name.vault.azure.net/key_id/key_version/","value":"secretValueV1"}`,
			want:    "secretValueV1",
			wantErr: false,
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

	// Setup client and mock Sender
	sender := &mockSender{}

	options := azsecrets.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: sender,
		},
		DisableChallengeResourceVerification: true,
	}
	client, _ := azsecrets.NewClient("https://fake.vault.io", &FakeCredential{}, &options)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			azureClients[tt.args.vaultName] = client

			if tt.resStatus == 0 {
				tt.resStatus = http.StatusOK
			}

			headers := http.Header{}
			headers.Set("WWW-Authenticate", `Bearer authorization="https://login.windows.net/d5069782-a6df-436e-bac4-67b0c78175c8", resource="not_empty"`)

			sender.doFunc = func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.resStatus,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader(tt.resBody)),
				}, nil
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
