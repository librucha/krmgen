package azcommons

import (
	"context"
	"errors"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

func GetSubscriptionId(cred azcore.TokenCredential) (string, error) {
	subscriptionId := os.Getenv("AZURE_SUBSCRIPTION_ID")
	if subscriptionId != "" {
		return subscriptionId, nil
	}

	client, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return "", err
	}

	pager := client.NewListPager(nil)
	if pager.More() {
		page, err := pager.NextPage(context.Background())
		if err != nil {
			return "", err
		}
		if len(page.Value) > 0 && page.Value[0].SubscriptionID != nil {
			return *page.Value[0].SubscriptionID, nil
		}
	}

	return "", errors.New("no subscriptions found. AZURE_SUBSCRIPTION_ID environment variable can be set to specify the subscription")
}
