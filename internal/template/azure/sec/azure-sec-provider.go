package azsec

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"strings"
)

const SecFunc = "azSec"

var azureClients = make(map[string]*azsecrets.Client, 10)

var cachedSecrets = make(map[azsecrets.ID]*azsecrets.SecretBundle, 50)

func ResolveSecret(vaultName string, keyArgs ...string) (any, error) {
	switch len(keyArgs) {
	case 1:
		return getSecretFromAzure(vaultName, keyArgs[0], "")
	case 2:
		return getSecretFromAzure(vaultName, keyArgs[0], keyArgs[1])
	default:
		return nil, fmt.Errorf("wrong arguments count for function %q expected 1 or 2 aruments but got %d", SecFunc, len(keyArgs))
	}
}

func getSecretFromAzure(vaultName string, keyId string, keyVer string) (string, error) {
	secretId := newId(vaultName, keyId, keyVer)
	cached := getFromCache(secretId)
	if cached != nil {
		return *cached.Value, nil
	}
	client, err := getClient(vaultName)
	if err != nil {
		return "", err
	}
	secret, err := client.GetSecret(context.Background(), keyId, keyVer, nil)
	if err != nil {
		return "", err
	}
	saveToCache(*secret.ID, &secret.SecretBundle)
	return *secret.Value, nil
}

func getClient(vaultName string) (*azsecrets.Client, error) {
	client := azureClients[vaultName]
	if client != nil {
		return client, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	client, err = azsecrets.NewClient(getVaultUrl(vaultName), cred, nil)
	if err != nil {
		return nil, err
	}
	azureClients[vaultName] = client
	return client, nil
}

func newId(vaultName string, keyId string, keyVer string) azsecrets.ID {
	return azsecrets.ID(strings.Join([]string{getVaultUrl(vaultName), keyId, keyVer}, "/"))
}

func getVaultUrl(vaultName string) string {
	return fmt.Sprintf("https://%v.vault.azure.net", vaultName)
}

func getFromCache(id azsecrets.ID) *azsecrets.SecretBundle {
	cached := cachedSecrets[id]
	if cached == nil {
		return nil
	}
	return cached
}

func saveToCache(id azsecrets.ID, secret *azsecrets.SecretBundle) {
	cachedSecrets[id] = secret
}
