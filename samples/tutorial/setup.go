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

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/gcsblob"
	"github.com/google/go-cloud/blob/s3blob"
	"github.com/google/go-cloud/gcp"

	azureblob "../../blob/azureblob"   //
	azureblob2 "../../blob/azureblob2" // to be deleted, old implementation

	mainStorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

var (
	environment = azure.PublicCloud
)

// setupBucket creates a connection to a particular cloud provider's blob storage.
func setupBucket(ctx context.Context, cloud, bucket string) (*blob.Bucket, error) {
	switch cloud {
	case "aws":
		return setupAWS(ctx, bucket)
	case "gcp":
		return setupGCP(ctx, bucket)
	case "azure": // uses github.com/Azure/azure-storage-blob-go/2018-03-28/azblob
		return setupAzureWithSASToken(ctx, bucket)


	case "old_azure_impl": // old impelmentation, to be removed: uses github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2017-10-01/storage
		return setupAzure2WithSASToken(ctx, bucket)
	default:
		return nil, fmt.Errorf("invalid cloud provider: %s", cloud)
	}
}

// setupGCP creates a connection to Google Cloud Storage (GCS).
func setupGCP(ctx context.Context, bucket string) (*blob.Bucket, error) {
	// DefaultCredentials assumes a user has logged in with gcloud.
	// See here for more information:
	// https://cloud.google.com/docs/authentication/getting-started
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, err
	}
	c, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		return nil, err
	}
	return gcsblob.OpenBucket(ctx, bucket, c)
}

// setupAWS creates a connection to Simple Cloud Storage Service (S3).
func setupAWS(ctx context.Context, bucket string) (*blob.Bucket, error) {
	c := &aws.Config{
		// Either hard-code the region or use AWS_REGION.
		Region: aws.String("us-east-2"),
		// credentials.NewEnvCredentials assumes two environment variables are
		// present:
		// 1. AWS_ACCESS_KEY_ID, and
		// 2. AWS_SECRET_ACCESS_KEY.
		Credentials: credentials.NewEnvCredentials(),
	}
	s := session.Must(session.NewSession(c))
	return s3blob.OpenBucket(ctx, s, bucket)
}

func setupAzure(ctx context.Context, bucket string) (*blob.Bucket, error) {
	settings := azureblob.Settings{
		AccountName:      os.Getenv("ACCOUNT_NAME"),
		AccountKey:       os.Getenv("ACCOUNT_KEY"),
		PublicAccessType: azureblob.PublicAccessBlob,
		SASToken:         "", // Not used when bootstrapping with AccountName & AccountKey
	}
	return azureblob.OpenBucket(ctx, &settings, bucket)
}

func setupAzureWithSASToken(ctx context.Context, bucket string) (*blob.Bucket, error) {

	// Use SASToken scoped at the Storage Account (full permission)
	// with this sasToken, ContainerExists can be either true or false
	sasToken := azureblob.GenerateSampleSASTokenForAccount(os.Getenv("ACCOUNT_NAME"), os.Getenv("ACCOUNT_KEY"), time.Now().UTC(), time.Now().UTC().Add(48*time.Hour))
	
	// Use SASToken scoped to a container (limited permissions, cannot create new container)
	// with this sasToken, ContainerExists should be true to avoid AuthenticationFailure exceptions
	//sasToken := azureblob.GenerateSampleSASTokenForContainerBlob(os.Getenv("ACCOUNT_NAME"), os.Getenv("ACCOUNT_KEY"), bucket, "", time.Now().UTC(), time.Now().UTC().Add(48*time.Hour))

	settings := azureblob.Settings{
		AccountName:      os.Getenv("ACCOUNT_NAME"),
		AccountKey:       "", // Not used when bootstrapping with SASToken
		PublicAccessType: azureblob.PublicAccessContainer,
		SASToken:         sasToken,		
	}
	return azureblob.OpenBucket(ctx, &settings, bucket)
}

func setupAzure2WithServicePrincipal(ctx context.Context, bucket string) (*blob.Bucket, error) {
	// Azure Authorization w/ Service Principal
	clientID := os.Getenv("AZURE_CLIENT_ID")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")

	// Azure Storage Account, Resource Group and SubscriptionId
	subscriptionID := os.Getenv("SUBSCRIPTION_ID")
	resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME")
	storageAccountName := os.Getenv("STORAGE_ACCOUNT_NAME")

	auth, err := getAuthorizationToken(clientID, clientSecret, tenantID, &environment)
	if err != nil {
		return nil, err
	}

	settings := azureblob2.Settings{
		Authorizer:          auth,               // caller must specify *autorest.BearerAuthorizer using their preferred authorization options
		EnvironmentName:     environment.Name,   // caller must specify the target Azure Environment (https://github.com/Azure/go-autorest/blob/master/autorest/azure/environments.go)
		SubscriptionID:      subscriptionID,     // caller must specify their Azure SubscriptionID
		ResourceGroupName:   resourceGroupName,  // caller must specify an already provisioned Azure Resource Group
		StorageAccountName:  storageAccountName, // caller must specify an already provisioned Azure Storage Account
		StorageKey:          "",                 // to be fetched from storage account if empty and no connectionString is set
		ContainerAccessType: "blob",             // See https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx and "x-ms-blob-public-access" header.
		ConnectionString:    "",                 // use connectionString/SASToken over Authorizer w/ StorageKey
	}

	return azureblob2.OpenBucket(ctx, &settings, bucket)
}

func setupAzure2WithSASToken(ctx context.Context, bucket string) (*blob.Bucket, error) {

	// Azure Authorization w/ Service Principal
	clientID := os.Getenv("AZURE_CLIENT_ID")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")

	// Azure Storage Account, Resource Group and SubscriptionId
	subscriptionID := os.Getenv("SUBSCRIPTION_ID")
	resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME")
	storageAccountName := os.Getenv("STORAGE_ACCOUNT_NAME")

	auth, _ := getAuthorizationToken(clientID, clientSecret, tenantID, &environment)

	settings := azureblob2.Settings{
		Authorizer:         auth,               // caller must specify *autorest.BearerAuthorizer using their preferred authorization options
		EnvironmentName:    environment.Name,   // caller must specify the target Azure Environment (https://github.com/Azure/go-autorest/blob/master/autorest/azure/environments.go)
		SubscriptionID:     subscriptionID,     // caller must specify their Azure SubscriptionID
		ResourceGroupName:  resourceGroupName,  // caller must specify an already provisioned Azure Resource Group
		StorageAccountName: storageAccountName, // caller must specify an already provisioned Azure Storage Account
	}

	accountSASOptions := mainStorage.AccountSASTokenOptions{
		Services: mainStorage.Services{
			Blob: true,
		},
		ResourceTypes: mainStorage.ResourceTypes{
			Service:   true,
			Container: true,
			Object:    true,
		},
		Permissions: mainStorage.Permissions{
			Read:    true,
			Write:   true,
			Delete:  true,
			List:    true,
			Add:     true,
			Create:  true,
			Update:  true,
			Process: true,
		},
		Expiry:   time.Date(2018, time.December, 31, 8, 0, 0, 0, time.FixedZone("GMT", -6)),
		UseHTTPS: true,
	}

	tokenVals, err := azureblob2.GenerateSasToken(&settings, &accountSASOptions)
	if err == nil {
		return azureblob2.OpenBucket(ctx, &azureblob2.Settings{SASTokenValues: tokenVals, StorageAccountName: storageAccountName, EnvironmentName: environment.Name}, bucket)
	}

	return nil, err
}

func setupAzure2WithConnectionString(ctx context.Context, bucket string) (*blob.Bucket, error) {

	connectionString := "ENTER YOUR AZURE STORAGE CONNECTION STRING"
	settings := azureblob2.Settings{
		Authorizer:          nil,
		ContainerAccessType: "blob",
		ConnectionString:    connectionString, // caller must supply the storage connection string or a SASToken
	}

	return azureblob2.OpenBucket(ctx, &settings, bucket)
}

func getAuthorizationToken(clientID string, clientSecret string, tenantID string, environment *azure.Environment) (*autorest.BearerAuthorizer, error) {

	oauthConfig, err := adal.NewOAuthConfig(environment.ActiveDirectoryEndpoint, tenantID)
	if err != nil {
		return nil, err
	}

	if oauthConfig == nil {
		return nil, fmt.Errorf("Unable to configure OAuthConfig for tenant %s", tenantID)
	}

	spt, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, environment.ResourceManagerEndpoint)

	if err != nil {
		return nil, err
	}

	auth := autorest.NewBearerAuthorizer(spt)
	return auth, nil
}

func getAuthorizationTokenFromDeviceFlow(clientID string, tenantID string, environment *azure.Environment) (autorest.Authorizer, error) {
	deviceFlowConfig := auth.NewDeviceFlowConfig(clientID, tenantID)
	deviceFlowConfig.Resource = environment.ResourceManagerEndpoint
	return deviceFlowConfig.Authorizer()
}
