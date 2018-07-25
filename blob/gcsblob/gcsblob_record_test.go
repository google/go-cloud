// +build record

package gcsblob

import (
	"context"
	"net/http"

	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/replay"
)

func newGCSClient(ctx context.Context, logf func(string, ...interface{}), filepath string) (*gcp.HTTPClient, func(), error) {

	mode := recorder.ModeRecording
	r, done, err := replay.NewGCSRecorder(logf, mode, filepath)
	if err != nil {
		return nil, nil, err
	}

	c := &gcp.HTTPClient{Client: http.Client{Transport: r}}
	if mode == recorder.ModeRecording {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			return nil, nil, err
		}
		c, err = gcp.NewHTTPClient(r, gcp.CredentialsTokenSource(creds))
		if err != nil {
			return nil, nil, err
		}
	}

	return c, done, err
}
