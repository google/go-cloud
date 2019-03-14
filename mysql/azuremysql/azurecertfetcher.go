// Copyright 2018 The Go Cloud Development Kit Authors
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

// Package azuremysql provides connections to Azure Database for MySql.
package azuremysql // import "gocloud.dev/mysql/azuremysql"

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"gocloud.dev/aws/rds"
	"golang.org/x/net/context/ctxhttp"
)

const (
	defaultBundleURL = "https://www.digicert.com/CACerts/BaltimoreCyberTrustRoot.crt.pem"
)

type (

	// AzureCertFetcher pulls the CA certificates from remote endpoint or file.
	AzureCertFetcher struct {
		rds.CertFetcher
		caBundleURL string
	}
)

// NewAzureCertFetcher returns a AzureCertFetcher with the specified cert bundle URL or default URL
// See https://docs.microsoft.com/en-us/azure/mysql/howto-configure-ssl.
func NewAzureCertFetcher(caBundleURL string) *AzureCertFetcher {
	url := defaultBundleURL
	if caBundleURL != "" {
		url = caBundleURL
	}
	return &AzureCertFetcher{
		caBundleURL: url,
	}
}

// RDSCertPoolFromFile fetches the Azure CA certificates from specified path.
func (ac *AzureCertFetcher) RDSCertPoolFromFile(ctx context.Context, path string) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	pem, _ := ioutil.ReadFile(path)
	if ok := certPool.AppendCertsFromPEM(pem); !ok {
		return nil, fmt.Errorf("Failed to append PEM from %v", path)
	}
	return certPool, nil
}

// Fetch fetches the Azure CA certificates. It is safe to call from multiple goroutines.
func (ac *AzureCertFetcher) Fetch(ctx context.Context) ([]*x509.Certificate, error) {
	client := ac.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := ctxhttp.Get(ctx, client, ac.caBundleURL)
	if err != nil {
		return nil, fmt.Errorf("fetch Azure certificates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch Azure certificates: HTTP %s", resp.Status)
	}
	pemData, err := ioutil.ReadAll(&io.LimitedReader{R: resp.Body, N: 1 << 20}) // limit to 1MiB
	if err != nil {
		return nil, fmt.Errorf("fetch Azure certificates: %v", err)
	}
	var certs []*x509.Certificate
	for len(pemData) > 0 {
		var block *pem.Block
		block, pemData = pem.Decode(pemData)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("fetch Azure certificates: %v", err)
		}
		certs = append(certs, c)
	}
	return certs, nil
}
