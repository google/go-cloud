package azureblob

import (
	"fmt"
	"net/url"	
	"time"

	"github.com/Azure/azure-storage-blob-go/2018-03-28/azblob"
)

// GetServiceURL creates the ServiceURL client
func GetServiceURL(accountName string, accountKey string, pipelineOptions azblob.PipelineOptions) azblob.ServiceURL {
	u, _ := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net/", accountName))
	credential := azblob.NewSharedKeyCredential(accountName, accountKey)
	pipeline := azblob.NewPipeline(credential, pipelineOptions)
	return azblob.NewServiceURL(*u, pipeline)
}
// GenerateSampleSASTokenForAccount Generates SASToken for a Storage Account
func GenerateSampleSASTokenForAccount(accountName string, accountKey string, startTime time.Time, expiresOn time.Time) string {
	credentials := azblob.NewSharedKeyCredential(accountName, accountKey)

	sasQueryParams := azblob.AccountSASSignatureValues{
		Protocol:      azblob.SASProtocolHTTPS,
		StartTime:     startTime,
		ExpiryTime:    expiresOn,
		Permissions:   azblob.AccountSASPermissions{
			Read: true, 
			Write: true, 
			Delete: true, 
			List: true, 
			Add: true, 
			Create: true, 
			Update: true, 
			Process: true,
		}.String(),
		Services:      azblob.AccountSASServices{Blob: true}.String(),
		ResourceTypes: azblob.AccountSASResourceTypes{Container: true, Object: true}.String(),
	}.NewSASQueryParameters(credentials)

	return sasQueryParams.Encode()
}

// GenerateSampleSASTokenForContainerBlob Generates SASToken for Container or Blob
func GenerateSampleSASTokenForContainerBlob(accountName string, accountKey string, containerName string, blobName string, startTime time.Time, expiresOn time.Time) string {
	credentials := azblob.NewSharedKeyCredential(accountName, accountKey)

	// for Container SASToken, set blob to ""; for blob SASToken, set both ContainerName and BlobName
	queryParams := azblob.BlobSASSignatureValues{
		Protocol: azblob.SASProtocolHTTPS,
		StartTime: startTime,
		ExpiryTime: expiresOn,
		Permissions: azblob.BlobSASPermissions{Read : true, Add : true, Create : true, Write : true, Delete : true}.String(),
		ContainerName: containerName,
		BlobName: blobName,
	}.NewSASQueryParameters(credentials)

	return queryParams.Encode()
}

