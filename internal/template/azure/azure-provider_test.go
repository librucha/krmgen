package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"net/http"
	"testing"
)

type mockSender struct {
	DoFunc func(r *http.Request) (*http.Response, error)
}

func (m mockSender) Do(r *http.Request) (*http.Response, error) {
	return m.DoFunc(r)
}

func Test_getSecretFromCache(t *testing.T) {
	type args struct {
		vaultName string
		keyId     string
		keyVer    string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "cached secret",
			args: args{"vault_name", "key_id", ""},
			want: "secretValue",
		},
	}

	// Setup client and mock Sender
	sender := &mockSender{}

	options := azsecrets.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: mockSender{},
		},
	}
	client, _ := azsecrets.NewClient("https://fake.vault.io", nil, &options)
	azureClients = map[string]*azsecrets.Client{
		"vault_name": client,
	}

	sender.DoFunc = func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
		}, nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//value := "secretValue"
			//id := newId(tt.args.vaultName, tt.args.keyId, tt.args.keyVer)
			//var secret = azsecrets.SecretBundle{
			//	Attributes:  nil,
			//	ContentType: nil,
			//	ID:          &id,
			//	Tags:        nil,
			//	Value:       &value,
			//	Kid:         nil,
			//	Managed:     nil,
			//}

			//cachedSecrets = map[azsecrets.ID]*azsecrets.SecretBundle{id: &secret}
			got, err := getSecretFromAzure(tt.args.vaultName, tt.args.keyId, tt.args.keyVer)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSecretFromAzure() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getSecretFromAzure() got = %v, want %v", got, tt.want)
			}
		})
	}
}
