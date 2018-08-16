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
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-github/github"
)

// gitHubAppAuth makes HTTP requests with GitHub application credentials.
// It watches a private key file, which can be rotated.
type gitHubAppAuth struct {
	id   int64
	base http.RoundTripper

	available   <-chan struct{}
	cancelWatch context.CancelFunc
	watchDone   <-chan struct{}

	mu         sync.RWMutex
	privateKey *rsa.PrivateKey
}

func newGitHubAppAuth(id int64, privateKey *runtimevar.Variable, rt http.RoundTripper) *gitHubAppAuth {
	watchDone := make(chan struct{})
	available := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	auth := &gitHubAppAuth{
		id:          id,
		base:        rt,
		available:   available,
		cancelWatch: cancel,
		watchDone:   watchDone,
	}
	go func() {
		// Watch privateKey until Stop cancels the context.
		// available is used to notify waitPublicKey.
		defer close(watchDone)
		auth.watch(ctx, available, privateKey)
	}()
	return auth
}

// Stop stops watching the private key.
func (auth *gitHubAppAuth) Stop() {
	auth.cancelWatch()
	<-auth.watchDone
	auth.mu.Lock()
	auth.privateKey = nil
	auth.mu.Unlock()
}

// watch watches the private key variable until the context is cancelled.
// The available channel is closed on the first successful Watch or after the
// context's cancellation, whichever comes first.
func (auth *gitHubAppAuth) watch(ctx context.Context, available chan<- struct{}, v *runtimevar.Variable) {
	first := true
	for ctx.Err() == nil {
		snap, err := v.Watch(ctx)
		if err != nil {
			log.Println("Watching GitHub private key:", err)
			continue
		}
		curr := snap.Value.(*rsa.PrivateKey)
		auth.mu.Lock()
		auth.privateKey = curr
		auth.mu.Unlock()
		if first {
			close(available)
			first = false
		}
		log.Println("Using new GitHub private key")
	}
	// Stop waitPrivateKey from blocking indefinitely (it will catch the error).
	if first {
		close(available)
	}
}

// CheckHealth returns an error before auth's private key is available.
// It implements the health.Checker interface.
func (auth *gitHubAppAuth) CheckHealth() error {
	auth.mu.RLock()
	ok := auth.privateKey != nil
	auth.mu.RUnlock()
	if !ok {
		return errors.New("GitHub application private key not available")
	}
	return nil
}

// RoundTrip sends a request with the application's credentials, useful for
// listing installations and such.
func (auth *gitHubAppAuth) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isGitHubAPIRequest(req) {
		// Pass through without adding credentials.
		return auth.base.RoundTrip(req)
	}
	forwarded := false
	if req.Body != nil {
		// As per http.RoundTripper interface, the RoundTrip method must always
		// close the Body.
		defer func() {
			if forwarded {
				return
			}
			if err := req.Body.Close(); err != nil {
				log.Println("Closing GitHub API request body:", err)
			}
		}()
	}

	// Compute authorization header.
	ctx := req.Context()
	key, err := auth.waitPrivateKey(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, &jwt.StandardClaims{
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(10 * time.Minute).Unix(),
		Issuer:    strconv.FormatInt(auth.id, 10),
	})
	sig, err := tok.SignedString(key)
	if err != nil {
		return nil, fmt.Errorf("signing GitHub API request: %v", err)
	}

	// RoundTrippers must not modify the passed in request.
	// Clone the headers then add the new authorization header.
	req2 := new(http.Request)
	*req2 = *req
	req2.Header = cloneHeaders(req.Header)
	req2.Header.Set("Authorization", "Bearer "+sig)
	forwarded = true
	return auth.base.RoundTrip(req2)
}

func (auth *gitHubAppAuth) waitPrivateKey(ctx context.Context) (*rsa.PrivateKey, error) {
	select {
	case <-auth.available:
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for GitHub application private key: %v", ctx.Err())
	}
	auth.mu.RLock()
	key := auth.privateKey
	auth.mu.RUnlock()
	if key == nil {
		return nil, errors.New("GitHub application private key unavailable")
	}
	return key, nil
}

func (auth *gitHubAppAuth) forInstall(id int64) *gitHubInstallAuth {
	// TODO(light): Cache instances for the same ID.
	return &gitHubInstallAuth{app: auth, id: id}
}

// installationAuthAccept is the Accept header required to use GitHub
// application installation tokens.
const installationAuthAccept = "application/vnd.github.machine-man-preview+json"

// fetchInstallToken obtains an authentication token for the given installation.
func (auth *gitHubAppAuth) fetchInstallToken(ctx context.Context, id int64) (string, time.Time, error) {
	client := &http.Client{Transport: auth}
	u := fmt.Sprintf("https://api.github.com/installations/%d/access_tokens", id)
	req, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetch GitHub installation token: %v", err)
	}
	req.Header.Set("Content-Length", "0")
	req.Header.Set("Accept", installationAuthAccept)
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetch GitHub installation token: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Println("Close GitHub API response body:", err)
		}
	}()
	if resp.StatusCode != http.StatusCreated {
		e := new(github.ErrorResponse)
		if err := json.NewDecoder(resp.Body).Decode(e); err != nil {
			return "", time.Time{}, fmt.Errorf("fetch GitHub installation token: HTTP %s", resp.Status)
		}
		e.Response = resp
		return "", time.Time{}, fmt.Errorf("fetch GitHub installation token: %v", e)
	}
	var content struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return "", time.Time{}, fmt.Errorf("fetch GitHub installation token: parse response: %v", err)
	}
	expiry, err := time.Parse(time.RFC3339, content.ExpiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetch GitHub installation token: parse response: %v", err)
	}
	return content.Token, expiry, nil
}

// gitHubInstallAuth makes HTTP requests with GitHub application installation credentials.
type gitHubInstallAuth struct {
	app *gitHubAppAuth
	id  int64

	mu     sync.Mutex
	token  string
	expiry time.Time
	cond   chan struct{}
}

// RoundTrip sends a request with an installation's credentials, possibly
// fetching a new access token.
func (auth *gitHubInstallAuth) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isGitHubAPIRequest(req) {
		// Pass through without adding credentials.
		return auth.app.base.RoundTrip(req)
	}
	forwarded := false
	if req.Body != nil {
		// As per http.RoundTripper interface, the RoundTrip method must always
		// close the Body.
		defer func() {
			if forwarded {
				return
			}
			if err := req.Body.Close(); err != nil {
				log.Println("Closing GitHub API request body:", err)
			}
		}()
	}

	// Block if another goroutine is fetching a token.
	ctx := req.Context()
	auth.mu.Lock()
	for auth.cond != nil {
		c := auth.cond
		auth.mu.Unlock()
		select {
		case <-c:
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for GitHub installation token: %v", ctx.Err())
		}
	}
	// Lock held: is token still valid? (Renew a minute ahead of time to reduce
	// clock skew issues.)
	now := time.Now()
	if auth.token == "" || now.After(auth.expiry.Add(60*time.Second)) {
		// Invalid token. Set the condition variable and fetch the new token.
		c := make(chan struct{})
		auth.cond = c
		auth.mu.Unlock()
		tok, expiry, err := auth.app.fetchInstallToken(ctx, auth.id)
		// Release condition variable so even if we fail, others can attempt later.
		auth.mu.Lock()
		close(c)
		auth.cond = nil

		if err != nil {
			auth.mu.Unlock()
			return nil, err
		}
		auth.token = tok
		auth.expiry = expiry
	}
	// Now we have a valid token. Release the lock.
	tok := auth.token
	auth.mu.Unlock()

	// RoundTrippers must not modify the passed in request.
	// Clone the headers then add the new authorization header.
	req2 := new(http.Request)
	*req2 = *req
	req2.Header = cloneHeaders(req.Header)
	req2.Header.Set("Authorization", "token "+tok)
	req2.Header.Set("Accept", installationAuthAccept)
	forwarded = true
	return auth.app.base.RoundTrip(req2)
}

func cloneHeaders(h http.Header) http.Header {
	h2 := make(http.Header, len(h))
	for k, v := range h {
		h2[k] = v
	}
	return h2
}

// isGitHubAPIRequest reports whether r is a request to the GitHub API.
// It is conservative, as this is used to ensure credentials are not
// leaked accidentally to other origins.
func isGitHubAPIRequest(r *http.Request) bool {
	return r.URL.Scheme == "https" && r.URL.Host == "api.github.com"
}
