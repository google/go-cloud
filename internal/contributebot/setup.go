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

// +build wireinject

package main

import (
	"context"
	"crypto/rsa"
	"errors"
	"net/http"

	"cloud.google.com/go/pubsub"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/health"
	"github.com/google/go-cloud/requestlog"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/filevar"
	"github.com/google/go-cloud/server"
	"github.com/google/go-cloud/wire"
	"go.opencensus.io/trace"
	"google.golang.org/api/option"
)

func setup(ctx context.Context, cfg flagConfig) (*worker, *server.Server, func(), error) {
	ws, cleanup, err := inject(ctx, cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	return ws.worker, ws.server, cleanup, nil
}

type workerAndServer struct {
	worker *worker
	server *server.Server
}

func inject(ctx context.Context, cfg flagConfig) (workerAndServer, func(), error) {
	wire.Build(
		gcp.CredentialsTokenSource,
		gcp.DefaultCredentials,
		gitHubAppAuthFromConfig,
		healthChecks,
		newPubSubClient,
		projectFromConfig,
		server.Set,
		subscriptionFromConfig,
		trace.NeverSample,
		wire.InterfaceValue(new(http.RoundTripper), http.DefaultTransport),
		wire.InterfaceValue(new(requestlog.Logger), (requestlog.Logger)(nil)),
		wire.InterfaceValue(new(trace.Exporter), (trace.Exporter)(nil)),
		workerAndServer{},
		worker{},
	)
	return workerAndServer{}, nil, errors.New("will be replaced by Wire")
}

func gitHubAppAuthFromConfig(rt http.RoundTripper, cfg flagConfig) (*gitHubAppAuth, func(), error) {
	d := runtimevar.NewDecoder(new(rsa.PrivateKey), func(p []byte, val interface{}) error {
		key, err := jwt.ParseRSAPrivateKeyFromPEM(p)
		if err != nil {
			return err
		}
		*(val.(**rsa.PrivateKey)) = key
		return nil
	})
	v, err := filevar.New(cfg.keyPath, d, nil)
	if err != nil {
		return nil, nil, err
	}
	auth := newGitHubAppAuth(cfg.gitHubAppID, v, rt)
	return auth, func() {
		auth.Stop()
		v.Close()
	}, nil
}

func newPubSubClient(ctx context.Context, id gcp.ProjectID, ts gcp.TokenSource) (*pubsub.Client, func(), error) {
	c, err := pubsub.NewClient(ctx, string(id), option.WithTokenSource(ts))
	if err != nil {
		return nil, nil, err
	}
	return c, func() { c.Close() }, nil
}

func subscriptionFromConfig(client *pubsub.Client, cfg flagConfig) *pubsub.Subscription {
	return client.SubscriptionInProject(cfg.subscription, cfg.project)
}

func projectFromConfig(cfg flagConfig) gcp.ProjectID {
	return gcp.ProjectID(cfg.project)
}

func healthChecks(w *worker) []health.Checker {
	return []health.Checker{w}
}
