package azureblob

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/go-autorest/autorest"
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

	if accountKeys.Response.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("Keys not found")
	}

	if err != nil {
		// We assume this is a transient error rather than a 404 (which is caught above),  so assume the
		// storeAccount still exists.
		return "", fmt.Errorf("Error retrieving keys for storage storeAccount %q: %s", storageAccountName, err)
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
