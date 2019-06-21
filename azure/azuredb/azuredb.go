// Copyright 2019 The Go Cloud Development Kit Authors
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

// Package azuredb contains Wire providers that are common across Azure Database.
package azuredb

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"golang.org/x/net/context/ctxhttp"
)

const caBundleURL = "https://www.digicert.com/CACerts/BaltimoreCyberTrustRoot.crt.pem"

// A CertPoolProvider returns a certificate pool that contains the Azure CA certificate.
type CertPoolProvider interface {
	AzureCertPool(context.Context) (*x509.CertPool, error)
}

// CertFetcher pulls the Azure CA certificates from Digicert's servers. The zero
// value will fetch certificates using the default HTTP client.
type CertFetcher struct {
	// Client is the HTTP client used to make requests. If nil, then
	// http.DefaultClient is used.
	Client *http.Client
}

// AzureCertPool fetches the Azure CA certificates and places them into a pool.
// It is safe to call from multiple goroutines.
func (cf *CertFetcher) AzureCertPool(ctx context.Context) (*x509.CertPool, error) {
	certs, err := cf.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	for _, c := range certs {
		certPool.AddCert(c)
	}
	return certPool, nil
}

// Fetch fetches the Azure CA certificates. It is safe to call from multiple goroutines.
func (cf *CertFetcher) Fetch(ctx context.Context) ([]*x509.Certificate, error) {
	client := cf.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := ctxhttp.Get(ctx, client, caBundleURL)
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
