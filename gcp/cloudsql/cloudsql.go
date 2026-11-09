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

// Package cloudsql contains Wire providers that are common across Google Cloud
// SQL.
package cloudsql // import "gocloud.dev/gcp/cloudsql"

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/certs"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/google/wire"
	"gocloud.dev/gcp"
	"golang.org/x/oauth2"
)

// CertSourceSet is a Wire provider set that binds a Cloud SQL proxy
// certificate source from an GCP-authenticated HTTP client.
//
// Deprecated: Use DialerSet instead, which supports PSC and explicit IP type
// selection via URLOpener.IPType or the "ip_type" URL query parameter.
var CertSourceSet = wire.NewSet(
	NewCertSource,
	wire.Bind(new(proxy.CertSource), new(*certs.RemoteCertSource)))

// NewCertSource creates a local certificate source that uses the given
// HTTP client. The client is assumed to make authenticated requests.
//
// Deprecated: Use NewDialer instead.
func NewCertSource(c *gcp.HTTPClient) *certs.RemoteCertSource {
	return certs.NewCertSourceOpts(&c.Client, certs.RemoteOpts{})
}

// NewCertSourceWithIAM creates a local certificate source, including Token
// source for token information used in cert creation, that uses the given HTTP
// client. The client is assumed to make authenticated requests.
//
// Deprecated: Use NewDialerWithIAM instead.
func NewCertSourceWithIAM(c *gcp.HTTPClient, t oauth2.TokenSource) *certs.RemoteCertSource {
	return certs.NewCertSourceOpts(&c.Client, certs.RemoteOpts{EnableIAMLogin: true, TokenSource: t})
}

// DialerSet is a Wire provider set that binds a Cloud SQL Dialer from a GCP
// token source. Use this instead of CertSourceSet when PSC or explicit IP type
// selection is required.
var DialerSet = wire.NewSet(NewDialer)

// IPType specifies the type of IP address to use when connecting to a Cloud SQL instance.
type IPType int

const (
	// IPTypeAuto uses a public IP if one is assigned to the instance; otherwise
	// falls back to private IP. This is the default and matches the behavior of
	// the legacy cloudsql-proxy v1 (equivalent to --auto-ip).
	IPTypeAuto IPType = iota
	// IPTypePublic uses the instance's public IP address.
	IPTypePublic
	// IPTypePrivate uses the instance's private IP address (requires VPC peering).
	IPTypePrivate
	// IPTypePSC uses a Private Service Connect endpoint.
	IPTypePSC
)

// DialOptions returns the cloudsqlconn.DialOption(s) for this IPType.
func (t IPType) DialOptions() []cloudsqlconn.DialOption {
	switch t {
	case IPTypePublic:
		return []cloudsqlconn.DialOption{cloudsqlconn.WithPublicIP()}
	case IPTypePrivate:
		return []cloudsqlconn.DialOption{cloudsqlconn.WithPrivateIP()}
	case IPTypePSC:
		return []cloudsqlconn.DialOption{cloudsqlconn.WithPSC()}
	default: // IPTypeAuto
		return []cloudsqlconn.DialOption{cloudsqlconn.WithAutoIP()}
	}
}

// ParseIPType parses the IPType from a string (likely a URL parameter).
func ParseIPType(s string) (IPType, error) {
	switch strings.ToLower(s) {
	case "auto", "":
		return IPTypeAuto, nil
	case "public":
		return IPTypePublic, nil
	case "private":
		return IPTypePrivate, nil
	case "psc":
		return IPTypePSC, nil
	default:
		return 0, fmt.Errorf("unknown ip_type %q (valid: auto, public, private, psc)", s)
	}
}

// NewDialer creates a Cloud SQL Dialer using the given OAuth2 token source.
// The returned cleanup function must be called when the dialer is no longer needed.
func NewDialer(ctx context.Context, ts oauth2.TokenSource) (*cloudsqlconn.Dialer, func() error, error) {
	d, err := cloudsqlconn.NewDialer(ctx, cloudsqlconn.WithTokenSource(ts))
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return d, func() error { return d.Close() }, nil
}

// NewDialerWithIAM creates a Cloud SQL Dialer with IAM authentication enabled,
// using the given OAuth2 token source. IAM authentication allows passwordless
// connections to Cloud SQL instances.
// The returned cleanup function must be called when the dialer is no longer needed.
func NewDialerWithIAM(ctx context.Context, ts oauth2.TokenSource) (*cloudsqlconn.Dialer, func() error, error) {
	d, err := cloudsqlconn.NewDialer(ctx,
		cloudsqlconn.WithIAMAuthNTokenSources(ts, ts),
		cloudsqlconn.WithIAMAuthN(),
	)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return d, func() error { return d.Close() }, nil
}
