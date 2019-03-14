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
	"strings"

	"golang.org/x/net/context/ctxhttp"
)

type (

	// AzureCertFetcher loads the CA certificates from remote endpoint or file.
	AzureCertFetcher struct {
		client *http.Client
		// certLocation can be a remote endpoint or a file path
		certLocation string
		useHTTP      bool
	}
)

// NewAzureCertFetcher constructs a new *AzureCertFetcher.
// See https://docs.microsoft.com/en-us/azure/mysql/howto-configure-ssl.
func NewAzureCertFetcher(caBundleLocation string) (*AzureCertFetcher, error) {
	if caBundleLocation == "" {
		return nil, fmt.Errorf("invalid argument caBundleLocation")
	}
	useHTTP := strings.HasPrefix(caBundleLocation, "http")
	return &AzureCertFetcher{
		certLocation: caBundleLocation,
		useHTTP:      useHTTP,
	}, nil
}

// AzureCertPool fetches the Azure CA certificate from a remote URL.
func (acf *AzureCertFetcher) AzureCertPool(ctx context.Context) (*x509.CertPool, error) {
	var certs []*x509.Certificate
	var err error

	// Fetch or Load the CA certificate.
	if acf.useHTTP {
		certs, err = acf.fetch(ctx)
	} else {
		certs, err = acf.load(ctx)
	}
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	for _, c := range certs {
		certPool.AddCert(c)
	}
	return certPool, nil
}

func (acf *AzureCertFetcher) load(ctx context.Context) ([]*x509.Certificate, error) {
	pemData, err := ioutil.ReadFile(acf.certLocation)
	if err != nil {
		return nil, fmt.Errorf("load Azure certificates: %v", err)
	}
	return acf.loadPem(pemData)
}

func (acf *AzureCertFetcher) fetch(ctx context.Context) ([]*x509.Certificate, error) {
	client := acf.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := ctxhttp.Get(ctx, client, acf.certLocation)
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
	return acf.loadPem(pemData)
}

func (acf *AzureCertFetcher) loadPem(pemData []byte) ([]*x509.Certificate, error) {
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
