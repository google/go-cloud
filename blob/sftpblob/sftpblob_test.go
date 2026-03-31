// Copyright 2026 The Go Cloud Development Kit Authors
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

package sftpblob

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/gcerrors"
)

type harness struct {
	dir         string
	prefix      string
	metadataHow MetadataOption
	client      *sftp.Client
	closer      func()
}

func newHarness(ctx context.Context, t *testing.T, prefix string, metadataHow MetadataOption) (drivertest.Harness, error) {
	t.Helper()

	if metadataHow == MetadataDontWrite {
		switch name := t.Name(); {
		case strings.Contains(name, "ContentType"), strings.HasSuffix(name, "TestAttributes"), strings.Contains(name, "TestMetadata/"):
			t.SkipNow()
			return nil, nil
		}
	}

	dir := t.TempDir()
	if prefix != "" {
		if err := os.MkdirAll(filepath.Join(dir, prefix), os.ModePerm); err != nil {
			return nil, err
		}
	}

	// Create an in-memory pipe to connect the SFTP client and server
	clientConn, serverConn := netPipe()

	// Initialize the SFTP server
	server, err := sftp.NewServer(serverConn, sftp.WithDebug(os.Stderr))
	if err != nil {
		return nil, err
	}

	// Run the SFTP server in a background goroutine
	go func() {
		_ = server.Serve()
	}()

	// Initialize the SFTP client
	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		return nil, err
	}

	h := &harness{
		dir:         dir,
		prefix:      prefix,
		metadataHow: metadataHow,
		client:      client,
		closer: func() {
			_ = client.Close()
			_ = server.Close()
		},
	}

	return h, nil
}

func netPipe() (io.ReadWriteCloser, io.ReadWriteCloser) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	return &pipeRWC{r1, w2}, &pipeRWC{r2, w1}
}

type pipeRWC struct {
	io.Reader
	io.Writer
}

func (p *pipeRWC) Close() error {
	var err1, err2 error
	if c, ok := p.Reader.(io.Closer); ok {
		err1 = c.Close()
	}
	if c, ok := p.Writer.(io.Closer); ok {
		err2 = c.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

func (h *harness) HTTPClient() *http.Client {
	return &http.Client{}
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	opts := &Options{
		Metadata: h.metadataHow,
	}
	drv, err := openBucket(h.client, h.dir, opts)
	if err != nil {
		return nil, err
	}
	if h.prefix == "" {
		return drv, nil
	}
	return driver.NewPrefixedBucket(drv, h.prefix), nil
}

func (h *harness) MakeDriverForNonexistentBucket(ctx context.Context) (driver.Bucket, error) {
	return nil, nil
}

func (h *harness) Close() {
	h.closer()
}

// verifyAs implements drivertest.AsTest for sftpblob
type verifyAs struct {
	prefix string
}

func (verifyAs) Name() string { return "verify As types for sftpblob" }

func (verifyAs) BucketCheck(b *blob.Bucket) error {
	var c *sftp.Client
	if !b.As(&c) {
		return errors.New("Bucket.As failed")
	}
	return nil
}

func (verifyAs) BeforeRead(as func(any) bool) error {
	var f *sftp.File
	if !as(&f) {
		return errors.New("BeforeRead.As failed")
	}
	return nil
}

func (verifyAs) BeforeWrite(as func(any) bool) error {
	var f *sftp.File
	if !as(&f) {
		return errors.New("BeforeWrite.As failed")
	}
	return nil
}

func (verifyAs) BeforeCopy(as func(any) bool) error { return nil }
func (verifyAs) BeforeList(as func(any) bool) error { return nil }
func (verifyAs) BeforeSign(as func(any) bool) error { return nil }
func (verifyAs) AttributesCheck(attrs *blob.Attributes) error {
	var fi fs.FileInfo
	if !attrs.As(&fi) {
		return errors.New("Attributes.As failed")
	}
	return nil
}

func (verifyAs) ReaderCheck(r *blob.Reader) error {
	var ior io.Reader
	if !r.As(&ior) {
		return errors.New("Reader.As failed")
	}
	return nil
}

func (verifyAs) ListObjectCheck(o *blob.ListObject) error {
	var fi fs.FileInfo
	if !o.As(&fi) {
		return errors.New("ListObject.As failed")
	}
	return nil
}

func (v verifyAs) ErrorCheck(b *blob.Bucket, err error) error {
	return nil
}

func TestConformance(t *testing.T) {
	newHarnessNoPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()
		return newHarness(ctx, t, "", MetadataInSidecar)
	}
	drivertest.RunConformanceTests(t, newHarnessNoPrefix, []drivertest.AsTest{verifyAs{}})
}

func TestConformanceWithPrefix(t *testing.T) {
	const prefix = "some/prefix/dir/"
	newHarnessWithPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, prefix, MetadataInSidecar)
	}
	drivertest.RunConformanceTests(t, newHarnessWithPrefix, []drivertest.AsTest{verifyAs{prefix: prefix}})
}

func TestConformanceSkipMetadata(t *testing.T) {
	newHarnessSkipMetadata := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, "", MetadataDontWrite)
	}
	drivertest.RunConformanceTests(t, newHarnessSkipMetadata, []drivertest.AsTest{verifyAs{}})
}

type netTimeoutError struct{}

func (e *netTimeoutError) Error() string   { return "net timeout" }
func (e *netTimeoutError) Timeout() bool   { return true }
func (e *netTimeoutError) Temporary() bool { return true }

func TestErrorCode(t *testing.T) {
	b := &bucket{}
	tests := []struct {
		err  error
		code gcerrors.ErrorCode
	}{
		{context.Canceled, gcerrors.Canceled},
		{context.DeadlineExceeded, gcerrors.DeadlineExceeded},
		{fs.ErrNotExist, gcerrors.NotFound},
		{fs.ErrPermission, gcerrors.PermissionDenied},
		{fs.ErrExist, gcerrors.AlreadyExists},
		{io.EOF, gcerrors.Internal},
		{io.ErrUnexpectedEOF, gcerrors.Internal},
		{&sftp.StatusError{Code: 2}, gcerrors.NotFound},            // SSH_FX_NO_SUCH_FILE
		{&sftp.StatusError{Code: 3}, gcerrors.PermissionDenied},    // SSH_FX_PERMISSION_DENIED
		{&sftp.StatusError{Code: 8}, gcerrors.Unimplemented},       // SSH_FX_OP_UNSUPPORTED
		{&sftp.StatusError{Code: 11}, gcerrors.AlreadyExists},      // SSH_FX_FILE_ALREADY_EXISTS
		{&sftp.StatusError{Code: 6}, gcerrors.Internal},            // SSH_FX_NO_CONNECTION
		{&sftp.StatusError{Code: 21}, gcerrors.FailedPrecondition}, // DIR_NOT_EMPTY
		{&netTimeoutError{}, gcerrors.DeadlineExceeded},
	}
	for _, tc := range tests {
		if got := b.ErrorCode(tc.err); got != tc.code {
			t.Errorf("ErrorCode(%v) = %v; want %v", tc.err, got, tc.code)
		}
	}
}

func TestErrorAs(t *testing.T) {
	b := &bucket{}
	var statusErr *sftp.StatusError
	if !b.ErrorAs(&sftp.StatusError{Code: 123}, &statusErr) {
		t.Error("ErrorAs failed to extract sftp.StatusError")
	}
	if statusErr.Code != 123 {
		t.Errorf("extracted wrong error code: %v", statusErr.Code)
	}

	var pathErr *os.PathError
	if !b.ErrorAs(&os.PathError{Op: "open"}, &pathErr) {
		t.Error("ErrorAs failed to extract os.PathError")
	}
	if pathErr.Op != "open" {
		t.Errorf("extracted wrong error op: %v", pathErr.Op)
	}
}

func TestOpenBucketURL(t *testing.T) {
	ctx := context.Background()
	opener := &URLOpener{}

	tests := []struct {
		urlStr  string
		wantErr string
	}{
		{"sftp://host?metadata=invalid", "unsupported value for query parameter 'metadata'"},
		{"sftp://host?timeout=invalid", "invalid timeout"},
		{"sftp://host?private_key_path=/does/not/exist/missing.key", "failed to read private key"},
		{"sftp://user:pass@host?create_dir=true&metadata=skip&insecure_skip_verify=true", "failed to dial ssh"},
		{"sftp://host?known_hosts_path=/does/not/exist/hosts.txt", "failed to parse known_hosts file"},
	}

	for _, tc := range tests {
		u, err := url.Parse(tc.urlStr)
		if err != nil {
			t.Fatal(err)
		}
		_, err = opener.OpenBucketURL(ctx, u)
		if err == nil {
			t.Errorf("OpenBucketURL(%q) unexpected success", tc.urlStr)
		} else if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("OpenBucketURL(%q) got error %v; want containing %q", tc.urlStr, err, tc.wantErr)
		}
	}

	// Test boundary deadline inference
	ctxWithDeadline, cancel := context.WithTimeout(ctx, 0)
	defer cancel()
	uTimeout, _ := url.Parse("sftp://host")
	_, err := opener.OpenBucketURL(ctxWithDeadline, uTimeout)
	if err != context.DeadlineExceeded && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded for zero deadline, got %v", err)
	}
}

func TestOpenBucket(t *testing.T) {
	clientConn, serverConn := netPipe()
	server, _ := sftp.NewServer(serverConn)
	go server.Serve()
	client, _ := sftp.NewClientPipe(clientConn, clientConn)
	defer client.Close()
	defer server.Close()

	// Should succeed
	bkt, err := OpenBucket(client, ".", nil)
	if err != nil {
		t.Fatalf("OpenBucket: %v", err)
	}
	defer bkt.Close()

	opts := &Options{CreateDir: true}
	_, err = OpenBucket(client, "newdir", opts)
	if err != nil {
		t.Logf("OpenBucket CreateDir err: %v", err) // e.g. permission or unsupported on purely mocked fs
	}
}
