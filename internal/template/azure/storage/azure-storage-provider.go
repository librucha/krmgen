package azstorage

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"strings"
)

const StoreKeyFunc = "azStoreKey"

type storageId string

var azureClients = make(map[string]*armstorage.AccountsClient, 10)

var cachedKeys = make(map[storageId]*armstorage.AccountKey, 50)

func GetStoreKey(subscriptionID string, resourceGroupName string, storageAccountName string) (string, error) {
	id := newId(subscriptionID, resourceGroupName, storageAccountName)
	cached := getFromCache(id)
	if cached != nil {
		return *cached.Value, nil
	}
	client, err := getClient(subscriptionID)
	if err != nil {
		return "", err
	}
	keys, err := client.ListKeys(context.Background(), resourceGroupName, storageAccountName, nil)
	if err != nil {
		return "", err
	}
	saveToCache(id, keys.Keys[0])
	return *keys.Keys[0].Value, nil
}

func getClient(subscriptionID string) (*armstorage.AccountsClient, error) {
	client := azureClients[subscriptionID]
	if client != nil {
		return client, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	client, err = armstorage.NewAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	azureClients[subscriptionID] = client
	return client, nil
}

func newId(subscriptionID string, resourceGroupName string, storageAccountName string) storageId {
	return storageId(strings.Join([]string{subscriptionID, resourceGroupName, storageAccountName}, ":"))
}

func getFromCache(id storageId) *armstorage.AccountKey {
	cached := cachedKeys[id]
	if cached == nil {
		return nil
	}
	return cached
}

func saveToCache(id storageId, key *armstorage.AccountKey) {
	cachedKeys[id] = key
}
