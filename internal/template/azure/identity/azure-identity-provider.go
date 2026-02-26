package azid

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	azcommons "github.com/librucha/krmgen/internal/template/azure/commons"
)

const ClientIdFunc = "azUaIdClientId"

var azureClients = make(map[string]*armmsi.UserAssignedIdentitiesClient, 10)

var cachedIdentities = make(map[ID]*armmsi.UserAssignedIdentitiesClientGetResponse, 50)

func GetClientId(rgName string, idName string) (any, error) {
	identity, err := getIdentity(rgName, idName)
	if err != nil {
		return nil, err
	}
	return identity.Properties.ClientID, err

}

func getIdentity(rgName string, idName string) (*armmsi.UserAssignedIdentitiesClientGetResponse, error) {
	id := newId(rgName, idName)
	cached := getFromCache(id)
	if cached != nil {
		return cached, nil
	}
	client, err := getClient(rgName)
	if err != nil {
		return nil, err
	}
	identity, err := client.Get(context.Background(), rgName, idName, nil)
	if err != nil {
		return nil, err
	}
	saveToCache(id, &identity)
	return &identity, nil
}

func getClient(vaultName string) (*armmsi.UserAssignedIdentitiesClient, error) {
	client := azureClients[vaultName]
	if client != nil {
		return client, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	subscriptionId, err := azcommons.GetSubscriptionId(cred)
	if err != nil {
		return nil, err
	}

	client, err = armmsi.NewUserAssignedIdentitiesClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, err
	}
	azureClients[vaultName] = client
	return client, nil
}

func newId(rgName string, idName string) ID {
	return ID{
		rgName: rgName,
		idName: idName,
	}
}

func getFromCache(id ID) *armmsi.UserAssignedIdentitiesClientGetResponse {
	cached := cachedIdentities[id]
	if cached == nil {
		return nil
	}
	return cached
}

func saveToCache(id ID, identity *armmsi.UserAssignedIdentitiesClientGetResponse) {
	cachedIdentities[id] = identity
}

type ID struct {
	rgName string
	idName string
}
