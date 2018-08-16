package azureblob

import (
	"net/url"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	mainStorage "github.com/Azure/azure-sdk-for-go/storage"
	
)

// helpers ported from https://github.com/terraform-providers/terraform-provider-azurerm/blob/master/azurerm/config.go
func WithRequestLogging() autorest.SendDecorator {
	return func(s autorest.Sender) autorest.Sender {
		return autorest.SenderFunc(func(r *http.Request) (*http.Response, error) {
			// dump request to wire format
			if dump, err := httputil.DumpRequestOut(r, true); err == nil {
				log.Printf("[DEBUG] AzureRM Request: \n%s\n", dump)
			} else {
				// fallback to basic message
				log.Printf("[DEBUG] AzureRM Request: %s to %s\n", r.Method, r.URL)
			}

			resp, err := s.Do(r)
			if resp != nil {
				// dump response to wire format
				if dump, err := httputil.DumpResponse(resp, true); err == nil {
					log.Printf("[DEBUG] AzureRM Response for %s: \n%s\n", r.URL, dump)
				} else {
					// fallback to basic message
					log.Printf("[DEBUG] AzureRM Response: %s for %s\n", resp.Status, r.URL)
				}
			} else {
				log.Printf("[DEBUG] Request to %s completed with no response", r.URL)
			}
			return resp, err
		})
	}
}

func GetStorageAccountKey(accountClient *storage.AccountsClient, resourceGroupName string, storageAccountName string) (string, error) {

	// fetch the storage account keys
	accountKeys, err := accountClient.ListKeys(context.Background(), resourceGroupName, storageAccountName)

	if err != nil {
		return "", fmt.Errorf("Error retrieving keys for storage storeAccount %q: %s", storageAccountName, err)
	}

	if accountKeys.Response.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("Keys not found")
	}

	if accountKeys.Keys == nil {
		return "", fmt.Errorf("Nil key returned for storage storeAccount %q", storageAccountName)
	}

	keys := *accountKeys.Keys
	if len(keys) <= 0 {
		return "", fmt.Errorf("No keys returned for storage storeAccount %q", storageAccountName)
	}

	key := keys[0].Value
	if key == nil {
		return "", fmt.Errorf("The first key returned is nil for storage storeAccount %q", storageAccountName)
	}

	return *key, nil

}

func GenerateSasToken (settings *AzureBlobSettings, sasOptions *mainStorage.AccountSASTokenOptions) (url.Values, error) {

	accountClient := storage.NewAccountsClient(settings.SubscriptionId)
	accountClient.Authorizer =  settings.Authorizer	
	environment, err := azure.EnvironmentFromName(settings.EnvironmentName)
	
	if err == nil {
		key := settings.StorageKey
		if key == "" {
			key, err = GetStorageAccountKey(&accountClient, settings.ResourceGroupName, settings.StorageAccountName)			
		}
		
		if err != nil {
			return nil, err
		}

		storageClient, _ := mainStorage.NewClient(settings.StorageAccountName, key, environment.StorageEndpointSuffix,
			mainStorage.DefaultAPIVersion, true)	
			
		return storageClient.GetAccountSASToken(*sasOptions)
	} else {
		return nil, err
	}
}