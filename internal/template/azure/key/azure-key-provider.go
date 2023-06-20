package azkey

import (
	"context"
	"encoding/pem"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"strings"
)

const KeyFunc = "azKey"

var azureClients = make(map[string]*azkeys.Client, 10)

var cachedCerts = make(map[azkeys.ID]*azkeys.KeyBundle, 5)

func ResolveKey(vaultName string, keyArgs ...string) (any, error) {
	switch len(keyArgs) {
	case 1:
		return getKeyFromAzure(vaultName, keyArgs[0], "")
	case 2:
		return getKeyFromAzure(vaultName, keyArgs[0], keyArgs[1])
	default:
		return nil, fmt.Errorf("wrong arguments count for function %q expected 1 or 2 aruments but got %d", KeyFunc, len(keyArgs))
	}
}

func getKeyFromAzure(vaultName string, keyName string, keyVer string) (string, error) {
	secretId := newId(vaultName, keyName, keyVer)
	cached := getFromCache(secretId)
	if cached != nil {
		return wrapKey(cached), nil
	}
	client, err := getClient(vaultName)
	if err != nil {
		return "", err
	}
	key, err := client.GetKey(context.Background(), keyName, keyVer, nil)
	if err != nil {
		return "", err
	}
	saveToCache(*key.Key.KID, &azkeys.KeyBundle{})
	return wrapKey(&key.KeyBundle), nil
}

func getClient(vaultName string) (*azkeys.Client, error) {
	client := azureClients[vaultName]
	if client != nil {
		return client, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	client, err = azkeys.NewClient(getVaultUrl(vaultName), cred, nil)
	if err != nil {
		return nil, err
	}
	azureClients[vaultName] = client
	return client, nil
}

func newId(vaultName string, keyName string, keyVer string) azkeys.ID {
	return azkeys.ID(strings.Join([]string{getVaultUrl(vaultName), keyName, keyVer}, "/"))
}

func getVaultUrl(vaultName string) string {
	return fmt.Sprintf("https://%v.vault.azure.net", vaultName)
}

func getFromCache(id azkeys.ID) *azkeys.KeyBundle {
	cached := cachedCerts[id]
	if cached == nil {
		return nil
	}
	return cached
}

func saveToCache(id azkeys.ID, secret *azkeys.KeyBundle) {
	cachedCerts[id] = secret
}

func wrapKey(key *azkeys.KeyBundle) string {
	block := &pem.Block{
		Type:  fmt.Sprintf("%s PRIVATE KEY", *key.Key.Kty),
		Bytes: key.Key.N,
	}
	return string(pem.EncodeToMemory(block))
}
