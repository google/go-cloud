// Copyright 2018 The Go Cloud Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package azureblob

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
)

// GetServiceURL creates the ServiceURL client
func GetServiceURL(accountName string, accountKey string, pipelineOptions azblob.PipelineOptions) azblob.ServiceURL {
	u, _ := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net/", accountName))
	credential, _ := azblob.NewSharedKeyCredential(accountName, accountKey)
	pipeline := azblob.NewPipeline(credential, pipelineOptions)
	return azblob.NewServiceURL(*u, pipeline)
}

// GenerateSampleSASTokenForAccount Generates SASToken for a Storage Account
func GenerateSampleSASTokenForAccount(accountName string, accountKey string, startTime time.Time, expiresOn time.Time) string {
	credentials, _ := azblob.NewSharedKeyCredential(accountName, accountKey)

	sasQueryParams, _ := azblob.AccountSASSignatureValues{
		Protocol:   azblob.SASProtocolHTTPS,
		StartTime:  startTime,
		ExpiryTime: expiresOn,
		Permissions: azblob.AccountSASPermissions{
			Read:    true,
			Write:   true,
			Delete:  true,
			List:    true,
			Add:     true,
			Create:  true,
			Update:  true,
			Process: true,
		}.String(),
		Services:      azblob.AccountSASServices{Blob: true}.String(),
		ResourceTypes: azblob.AccountSASResourceTypes{Container: true, Object: true}.String(),
	}.NewSASQueryParameters(credentials)

	return sasQueryParams.Encode()
}

// GenerateSampleSASTokenForContainerBlob Generates SASToken for Container or Blob
func GenerateSampleSASTokenForContainerBlob(accountName string, accountKey string, containerName string, blobName string, startTime time.Time, expiresOn time.Time) string {
	credentials, _ := azblob.NewSharedKeyCredential(accountName, accountKey)

	// for Container SASToken, set blob to ""; for blob SASToken, set both ContainerName and BlobName
	queryParams, _ := azblob.BlobSASSignatureValues{
		Protocol:      azblob.SASProtocolHTTPS,
		StartTime:     startTime,
		ExpiryTime:    expiresOn,
		Permissions:   azblob.BlobSASPermissions{Read: true, Add: true, Create: true, Write: true, Delete: true}.String(),
		ContainerName: containerName,
		BlobName:      blobName,
	}.NewSASQueryParameters(credentials)

	return queryParams.Encode()
}

// These helper functions convert a binary block ID to a base-64 string and vice versa
// NOTE: The blockID must be <= 64 bytes and ALL blockIDs for the block must be the same length
func blockIDBinaryToBase64(blockID []byte) string { return base64.StdEncoding.EncodeToString(blockID) }
func blockIDBase64ToBinary(blockID string) []byte {
	binary, _ := base64.StdEncoding.DecodeString(blockID)
	return binary
}

// BlockIDIntToBase64 convert an int block ID to a base-64 string
func BlockIDIntToBase64(blockID int) string {
	binaryBlockID := (&[4]byte{})[:] // All block IDs are 4 bytes long
	binary.LittleEndian.PutUint32(binaryBlockID, uint32(blockID))
	return blockIDBinaryToBase64(binaryBlockID)
}

// BlockIDBase64ToInt convert a base-64 string to int block ID
func BlockIDBase64ToInt(blockID string) int {
	blockIDBase64ToBinary(blockID)
	return int(binary.LittleEndian.Uint32(blockIDBase64ToBinary(blockID)))
}
