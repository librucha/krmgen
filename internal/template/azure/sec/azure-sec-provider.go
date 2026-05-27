package azsec

import (
	"context"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

const SecFunc = "azSec"
const ToPemFunc = "toPem"

var azureClients = make(map[string]*azsecrets.Client, 10)

var cachedSecrets = make(map[azsecrets.ID]*azsecrets.Secret, 50)

func GetSecret(vaultName string, keyArgs ...string) (any, error) {
	switch len(keyArgs) {
	case 1:
		return getSecretFromAzure(vaultName, keyArgs[0], "")
	case 2:
		return getSecretFromAzure(vaultName, keyArgs[0], keyArgs[1])
	default:
		return nil, fmt.Errorf("wrong arguments count for function %q expected 1 or 2 aruments but got %d", SecFunc, len(keyArgs))
	}
}

func ToPemBlock(blockType string, text string) (string, error) {
	block := &pem.Block{
		Type:  blockType,
		Bytes: []byte(text),
	}
	return string(pem.EncodeToMemory(block)), nil
}

func getSecretFromAzure(vaultName string, keyId string, keyVer string) (string, error) {
	secretId := newId(vaultName, keyId, keyVer)
	if cached := getFromCache(secretId); cached != nil {
		return *cached.Value, nil
	}
	client, err := getClient(vaultName)
	if err != nil {
		return "", err
	}
	if keyVer == "" {
		return getLatestActiveSecret(client, vaultName, keyId, secretId)
	}
	secret, err := client.GetSecret(context.Background(), keyId, keyVer, nil)
	if err != nil {
		return "", err
	}
	saveToCache(*secret.ID, &secret.Secret)
	return *secret.Value, nil
}

// getLatestActiveSecret finds the most recently created version whose NotBefore is not in the future.
func getLatestActiveSecret(client *azsecrets.Client, vaultName string, keyId string, noVerCacheId azsecrets.ID) (string, error) {
	now := time.Now().UTC()
	var best *azsecrets.SecretProperties

	pager := client.NewListSecretPropertiesVersionsPager(keyId, nil)
	for pager.More() {
		page, err := pager.NextPage(context.Background())
		if err != nil {
			return "", fmt.Errorf("listing versions of secret %q in vault %q: %w", keyId, vaultName, err)
		}
		for _, item := range page.Value {
			if item == nil || item.Attributes == nil || item.ID == nil {
				continue
			}
			if item.Attributes.Enabled != nil && !*item.Attributes.Enabled {
				continue
			}
			if item.Attributes.NotBefore != nil && item.Attributes.NotBefore.UTC().After(now) {
				continue
			}
			if best == nil || isNewerSecretVersion(item, best) {
				best = item
			}
		}
	}

	if best == nil {
		return "", fmt.Errorf("no active version found for secret %q in vault %q", keyId, vaultName)
	}

	resolvedVer := best.ID.Version()
	secret, err := client.GetSecret(context.Background(), keyId, resolvedVer, nil)
	if err != nil {
		return "", err
	}
	saveToCache(*secret.ID, &secret.Secret)
	// also cache under the no-version key so repeated calls skip listing
	saveToCache(noVerCacheId, &secret.Secret)
	return *secret.Value, nil
}

func isNewerSecretVersion(a, b *azsecrets.SecretProperties) bool {
	if a.Attributes.Created == nil {
		return false
	}
	if b.Attributes.Created == nil {
		return true
	}
	return a.Attributes.Created.After(*b.Attributes.Created)
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

func getFromCache(id azsecrets.ID) *azsecrets.Secret {
	cached := cachedSecrets[id]
	if cached == nil {
		return nil
	}
	return cached
}

func saveToCache(id azsecrets.ID, secret *azsecrets.Secret) {
	cachedSecrets[id] = secret
}
