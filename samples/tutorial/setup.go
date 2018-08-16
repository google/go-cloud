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

	azureblob "../../blob/azureblob"

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
	case "azure":
		return setupAzureWithGeneratedSASToken(ctx, bucket)
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

func setupAzureWithServicePrincipal(ctx context.Context, bucket string) (*blob.Bucket, error) {
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

	settings := azureblob.AzureBlobSettings{
		Authorizer:          auth,         // caller must specify *autorest.BearerAuthorizer using their preferred authorization options
		EnvironmentName:     environment.Name,   // caller must specify the target Azure Environment (https://github.com/Azure/go-autorest/blob/master/autorest/azure/environments.go)
		SubscriptionId:      subscriptionID,     // caller must specify their Azure SubscriptionID
		ResourceGroupName:   resourceGroupName,  // caller must specify an already provisioned Azure Resource Group
		StorageAccountName:  storageAccountName, // caller must specify an already provisioned Azure Storage Account
		StorageKey:          "",                 // to be fetched from storage account if empty and no connectionString is set
		ContainerAccessType: "blob",             // See https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx and "x-ms-blob-public-access" header.
		ConnectionString:    "",                 // use connectionString/SASToken over Authorizer w/ StorageKey
	}

	return azureblob.OpenBucket(ctx, &settings, bucket)
}

func setupAzureWithGeneratedSASToken(ctx context.Context, bucket string) (*blob.Bucket, error) {

	// Azure Authorization w/ Service Principal
	clientID := os.Getenv("AZURE_CLIENT_ID")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")

	// Azure Storage Account, Resource Group and SubscriptionId
	subscriptionID := os.Getenv("SUBSCRIPTION_ID")	
	resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME")	
	storageAccountName := os.Getenv("STORAGE_ACCOUNT_NAME")

	auth, _ := getAuthorizationToken(clientID, clientSecret, tenantID, &environment)

	settings := azureblob.AzureBlobSettings{
		Authorizer:          auth,               // caller must specify *autorest.BearerAuthorizer using their preferred authorization options
		EnvironmentName:     environment.Name,   // caller must specify the target Azure Environment (https://github.com/Azure/go-autorest/blob/master/autorest/azure/environments.go)
		SubscriptionId:      subscriptionID,     // caller must specify their Azure SubscriptionID
		ResourceGroupName:   resourceGroupName,  // caller must specify an already provisioned Azure Resource Group
		StorageAccountName:  storageAccountName, // caller must specify an already provisioned Azure Storage Account
		StorageKey:          "",                 // to be fetched from storage account if empty and no connectionString is set
		ContainerAccessType: "",                 // See https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx and "x-ms-blob-public-access" header.
		ConnectionString:    "",                 // use connectionString/SASToken over Authorizer w/ StorageKey
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

	tokenVals, err := azureblob.GenerateSasToken(&settings, &accountSASOptions)
	if err == nil {
		return azureblob.OpenBucket(ctx, &azureblob.AzureBlobSettings{SASTokenValues: tokenVals, StorageAccountName: storageAccountName, EnvironmentName: environment.Name}, bucket)
	} else {
		return nil, err
	}
}

func setupAzureWithConnectionString(ctx context.Context, bucket string) (*blob.Bucket, error) {

	connectionString := "ENTER YOUR AZURE STORAGE CONNECTION STRING"
	settings := azureblob.AzureBlobSettings{
		Authorizer:          nil,
		EnvironmentName:     "",
		SubscriptionId:      "",
		ResourceGroupName:   "",
		StorageAccountName:  "",
		StorageKey:          "",
		ContainerAccessType: "blob",
		ConnectionString:    connectionString, // caller must supply the storage connection string or a SASToken
	}

	return azureblob.OpenBucket(ctx, &settings, bucket)
}

func getAuthorizationToken(clientId string, clientSecret string, tenantId string, environment *azure.Environment) (*autorest.BearerAuthorizer, error) {

	oauthConfig, err := adal.NewOAuthConfig(environment.ActiveDirectoryEndpoint, tenantId)
	if err != nil {
		return nil, err
	}

	if oauthConfig == nil {
		return nil, fmt.Errorf("Unable to configure OAuthConfig for tenant %s", tenantId)
	}

	spt, err := adal.NewServicePrincipalToken(*oauthConfig, clientId, clientSecret, environment.ResourceManagerEndpoint)

	if err != nil {
		return nil, err
	}

	auth := autorest.NewBearerAuthorizer(spt)
	return auth, nil
}

func getAuthorizationTokenFromDeviceFlow(clientId string, tenantId string, environment *azure.Environment) (autorest.Authorizer, error) {
	deviceFlowConfig := auth.NewDeviceFlowConfig(clientId, tenantId)
	deviceFlowConfig.Resource = environment.ResourceManagerEndpoint
	return deviceFlowConfig.Authorizer()
}
