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

// Package sftpblob provides a blob implementation that uses the SFTP protocol.
// Use OpenBucket to construct a *blob.Bucket.
//
// To avoid partial writes, sftpblob writes to a temporary file and then renames
// the temporary file to the final path on Close. It creates these
// temporary files next to the actual files to ensure they are on the same
// remote filesystem, allowing atomic Rename operations.
//
// By default sftpblob stores blob metadata in "sidecar" files under the original
// filename with an additional ".attrs" suffix.
// This behaviour can be changed via Options.Metadata;
// writing of those metadata files can be suppressed by setting it to
// MetadataDontWrite or its equivalent "metadata=skip" in the URL for the opener.
// In either case, absent any stored metadata many blob.Attributes fields
// will be set to default values.
//
// # URLs
//
// For blob.OpenBucket, sftpblob registers for the scheme "sftp".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # Escaping
//
// Go CDK supports all UTF-8 strings; to make this work with services lacking
// full UTF-8 support, strings must be escaped (during writes) and unescaped
// (during reads). The following escapes are performed for sftpblob:
//   - Blob keys: ASCII characters 0-31 are escaped to "__0x<hex>__".
//     Additionally, the "/" in "../", the trailing "/" in "//", and a trailing
//     "/" in key names are escaped in the same way.
//     The characters "\<>:"|?*" are also escaped for safety.
//
// # As
//
// sftpblob exposes the following types for As:
//   - Bucket: *sftp.Client
//   - ListObject: fs.FileInfo
//   - Reader: io.Reader
//   - ReaderOptions.BeforeRead: *sftp.File
//   - Attributes: fs.FileInfo
//   - CopyOptions.BeforeCopy: *sftp.File
//   - WriterOptions.BeforeWrite: *sftp.File
package sftpblob // import "gocloud.dev/blob/sftpblob"

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/escape"
	"gocloud.dev/internal/gcerr"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const defaultPageSize = 1000
const attrsExt = ".attrs"

func init() {
	blob.DefaultURLMux().RegisterBucket(Scheme, &URLOpener{})
}

// Scheme is the URL scheme sftpblob registers its URLOpener under on blob.DefaultMux.
const Scheme = "sftp"

// URLOpener opens sftp bucket URLs like "sftp://user:pass@host:port/foo/bar/baz".
//
// The URL's host is the SFTP server to connect to. If the port is omitted, it
// defaults to 22. The URL's path is the directory to use as the bucket root.
//
// The following query parameters are supported:
//
//   - private_key_path: path to read for the SSH private key (PEM encoded) used
//     for authentication. If provided, this overrides password authentication.
//   - create_dir: (any non-empty value) the directory is created (using MkdirAll)
//     if it does not already exist.
//   - insecure_skip_verify: if set to any non-empty value, disables SSH host key
//     verification. Otherwise, host key verification is naturally enabled.
//   - known_hosts_path: path to the known_hosts file. If not provided and
//     insecure_skip_verify is not set, it defaults to ~/.ssh/known_hosts.
//   - metadata: if set to "skip", won't write metadata such as blob.Attributes
//     as per the package docstring.
//   - timeout: specifies the maximum amount of time for the TCP
//     connection to establish. If not specified, it is inferred from the
//     provided parent Context, defaulting to 15s.
//
// Here are some example URLs:
//
//   - sftp://user:password@example.com/a/directory
//     -> Connects to example.com:22 with password authentication, using
//     "/a/directory" as the bucket root.
//   - sftp://user@example.com:2222/a/directory?private_key_path=/path/to/id_rsa
//     -> Connects to example.com:2222 with public key authentication, reading
//     the private key from "/path/to/id_rsa".
//   - sftp://user@[2001:db8::1]/
//     -> Connects to the IPv6 address with agent authentication (if SSH_AUTH_SOCK
//     is set).
type URLOpener struct {
	Options Options
}

type MetadataOption string

const (
	MetadataInSidecar MetadataOption = ""
	MetadataDontWrite MetadataOption = "skip"
)

// OpenBucketURL opens a blob.Bucket based on u.
func (o *URLOpener) OpenBucketURL(ctx context.Context, u *url.URL) (*blob.Bucket, error) {
	opts := new(Options)
	*opts = o.Options

	q := u.Query()
	if q.Get("create_dir") != "" {
		opts.CreateDir = true
	}
	if metadataVal := q["metadata"]; len(metadataVal) > 0 {
		switch MetadataOption(metadataVal[0]) {
		case MetadataDontWrite:
			opts.Metadata = MetadataDontWrite
		case MetadataInSidecar:
			opts.Metadata = MetadataInSidecar
		default:
			return nil, errors.New("sftpblob.OpenBucket: unsupported value for query parameter 'metadata'")
		}
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "22"
	}

	var hostKeyCallback ssh.HostKeyCallback
	if q.Get("insecure_skip_verify") != "" {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		knownHostsPath := q.Get("known_hosts_path")
		if knownHostsPath == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("sftpblob: cannot locate home directory for default known_hosts: %v", err)
			}
			knownHostsPath = filepath.Join(homeDir, ".ssh", "known_hosts")
		}
		cb, err := knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("sftpblob: failed to parse known_hosts file %q: %v", knownHostsPath, err)
		}
		hostKeyCallback = cb
	}

	var timeout time.Duration
	if tstr := q.Get("timeout"); tstr != "" {
		t, err := time.ParseDuration(tstr)
		if err != nil {
			return nil, fmt.Errorf("sftpblob: invalid timeout %q: %v", tstr, err)
		}
		timeout = t
	} else if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, context.DeadlineExceeded
		}
	} else {
		timeout = 15 * time.Second
	}

	config := &ssh.ClientConfig{
		User:            u.User.Username(),
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	var closers closerList
	defer func() {
		_ = closers.Close()
	}()

	if keyPath := q.Get("private_key_path"); keyPath != "" {
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("sftpblob: failed to read private key %q: %v", keyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("sftpblob: failed to parse private key %q: %v", keyPath, err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	} else if pass, hasPass := u.User.Password(); hasPass {
		config.Auth = append(config.Auth, ssh.Password(pass))
	} else if authSock := os.Getenv("SSH_AUTH_SOCK"); authSock != "" {
		if agentConn, err := net.Dial("unix", authSock); err == nil {
			closers = append(closers, agentConn)
			config.Auth = append(config.Auth, ssh.PublicKeysCallback(agent.NewClient(agentConn).Signers))
		}
	}

	sshClient, err := ssh.Dial("tcp", host+":"+port, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial ssh: %v", err)
	}
	closers = append(closers, sshClient)

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create sftp client: %v", err)
	}

	bucketPath := u.Path
	if bucketPath == "" {
		bucketPath = "/"
	}

	drv, err := openBucket(sftpClient, bucketPath, opts)
	if err != nil {
		sftpClient.Close()
		return nil, err
	}
	drv.closers = closers
	closers = nil

	return blob.NewBucket(drv), nil
}

// Options sets options for constructing a *blob.Bucket backed by sftpblob.
type Options struct {
	// CreateDir specifies whether the bucket root directory should be created
	// if it does not already exist.
	CreateDir bool

	Metadata MetadataOption
}

type bucket struct {
	client  *sftp.Client
	dir     string
	opts    *Options
	closers closerList
}

// openBucket creates a driver.Bucket backed by an sftp.Client.
func openBucket(client *sftp.Client, dir string, opts *Options) (*bucket, error) {
	if opts == nil {
		opts = &Options{}
	}
	// normalize path to absolute remote path using path.Clean
	absdir := path.Clean(dir)

	if opts.CreateDir {
		if err := client.MkdirAll(absdir); err != nil {
			return nil, err
		}
	}

	info, err := client.Stat(absdir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", absdir)
	}

	return &bucket{client: client, dir: absdir, opts: opts}, nil
}

// OpenBucket creates a *blob.Bucket backed by an sftp.Client and rooted at dir.
func OpenBucket(client *sftp.Client, dir string, opts *Options) (*blob.Bucket, error) {
	drv, err := openBucket(client, dir, opts)
	if err != nil {
		return nil, err
	}
	return blob.NewBucket(drv), nil
}

func (b *bucket) Close() error {
	errClient := b.client.Close()
	errClosers := b.closers.Close()
	return errors.Join(errClient, errClosers)
}

type closerList []io.Closer

func (l closerList) Close() error {
	var errs []error
	for i := len(l) - 1; i >= 0; i-- {
		if c := l[i]; c != nil {
			errs = append(errs, c.Close())
		}
	}
	return errors.Join(errs...)
}

func (b *bucket) ErrorCode(err error) gcerrors.ErrorCode {
	if errors.Is(err, context.Canceled) {
		return gcerrors.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return gcerrors.DeadlineExceeded
	}
	if os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist) {
		return gcerrors.NotFound
	}
	if os.IsPermission(err) || errors.Is(err, fs.ErrPermission) {
		return gcerrors.PermissionDenied
	}
	if os.IsExist(err) || errors.Is(err, fs.ErrExist) {
		return gcerrors.AlreadyExists
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return gcerrors.Internal
	}

	var statusErr *sftp.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.Code {
		case 2: // SSH_FX_NO_SUCH_FILE
			return gcerrors.NotFound
		case 3, 27: // SSH_FX_PERMISSION_DENIED, SSH_FX_WRITE_PROTECT
			return gcerrors.PermissionDenied
		case 8: // SSH_FX_OP_UNSUPPORTED
			return gcerrors.Unimplemented
		case 11: // SSH_FX_FILE_ALREADY_EXISTS
			return gcerrors.AlreadyExists
		case 6, 7: // SSH_FX_NO_CONNECTION, SSH_FX_CONNECTION_LOST
			return gcerrors.Internal
		case 21, 22, 24: // DIR_NOT_EMPTY, NOT_A_DIRECTORY, FILE_IS_A_DIRECTORY
			return gcerrors.FailedPrecondition
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return gcerrors.DeadlineExceeded
		}
		return gcerrors.Internal
	}

	return gcerrors.Unknown
}

// escapeKey does required escaping for UTF-8 strings.
func escapeKey(s string) string {
	s = escape.HexEscape(s, func(r []rune, i int) bool {
		c := r[i]
		switch {
		case c < 32:
			return true
		case c == '\\':
			return true
		case i > 1 && c == '/' && r[i-1] == '.' && r[i-2] == '.':
			return true
		case i > 0 && c == '/' && r[i-1] == '/':
			return true
		case c == '/' && i == len(r)-1:
			return true
		case c == '>' || c == '<' || c == ':' || c == '"' || c == '|' || c == '?' || c == '*':
			return true
		}
		return false
	})
	return s
}

// unescapeKey reverses escapeKey.
func unescapeKey(s string) string {
	return escape.HexUnescape(s)
}

func (b *bucket) fullPath(key string) (string, error) {
	if key == "" {
		return "", errors.New("sftpblob: empty key")
	}
	ek := escapeKey(key)
	p := path.Join(b.dir, ek)
	dirPrefix := b.dir
	if !strings.HasSuffix(dirPrefix, "/") {
		dirPrefix += "/"
	}
	if !strings.HasPrefix(p+"/", dirPrefix) {
		return "", fmt.Errorf("sftpblob: key %q escapes bucket root", key)
	}
	if strings.HasSuffix(p, attrsExt) {
		return "", fmt.Errorf("file extension %q is reserved", attrsExt)
	}
	return p, nil
}

// xattrs stores extended attributes
type xattrs struct {
	CacheControl       string            `json:"user.cache_control"`
	ContentDisposition string            `json:"user.content_disposition"`
	ContentEncoding    string            `json:"user.content_encoding"`
	ContentLanguage    string            `json:"user.content_language"`
	ContentType        string            `json:"user.content_type"`
	Metadata           map[string]string `json:"user.metadata"`
	MD5                []byte            `json:"md5"`
}

func getAttrs(ctx context.Context, client *sftp.Client, fullPath string) (xattrs, error) {
	if err := ctx.Err(); err != nil {
		return xattrs{}, err
	}
	f, err := client.Open(fullPath + attrsExt)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist) {
			return xattrs{ContentType: "application/octet-stream"}, nil
		}
		return xattrs{}, err
	}
	defer f.Close()
	xa := new(xattrs)
	err = json.NewDecoder(f).Decode(xa)
	return *xa, err
}

func setAttrs(ctx context.Context, client *sftp.Client, fullPath string, xa xattrs) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := client.Create(fullPath + attrsExt)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(xa); err != nil {
		f.Close()
		client.Remove(f.Name())
		return err
	}
	return f.Close()
}

func (b *bucket) forKey(ctx context.Context, key string) (string, fs.FileInfo, *xattrs, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, nil, err
	}
	p, err := b.fullPath(key)
	if err != nil {
		return "", nil, nil, err
	}
	info, err := b.client.Stat(p)
	if err != nil {
		return "", nil, nil, err
	}
	if info.IsDir() {
		return "", nil, nil, os.ErrNotExist
	}
	xa, err := getAttrs(ctx, b.client, p)
	if err != nil {
		return "", nil, nil, err
	}
	return p, info, &xa, nil
}

func (b *bucket) Attributes(ctx context.Context, key string) (*driver.Attributes, error) {
	_, info, xa, err := b.forKey(ctx, key)
	if err != nil {
		return nil, err
	}
	return &driver.Attributes{
		CacheControl:       xa.CacheControl,
		ContentDisposition: xa.ContentDisposition,
		ContentEncoding:    xa.ContentEncoding,
		ContentLanguage:    xa.ContentLanguage,
		ContentType:        xa.ContentType,
		Metadata:           xa.Metadata,
		ModTime:            info.ModTime(),
		Size:               info.Size(),
		MD5:                xa.MD5,
		ETag:               fmt.Sprintf("\"%x-%x\"", info.ModTime().UnixNano(), info.Size()),
		AsFunc: func(i any) bool {
			if p, ok := i.(*fs.FileInfo); ok {
				*p = info
				return true
			}
			return false
		},
	}, nil
}
// ListPaged implements driver.Bucket.
//
// Note: Due to limitations in the SFTP protocol natively restricting server-side
// directory cursors, pagination requires sequentially re-walking the remote tree
// starting from the prefix root for each page request. Consequently, ListPaged
// executes structurally as an O(N) operation over the network per page and should
// not be relied upon for production instances hosting large volume file counts.
func (b *bucket) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
	var pageToken string
	if len(opts.PageToken) > 0 {
		pageToken = string(opts.PageToken)
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}

	var root string
	if i := strings.LastIndex(opts.Prefix, "/"); i > -1 {
		root = path.Join(b.dir, opts.Prefix[:i])
	} else {
		root = b.dir
	}

	walker := clientWalker{b.client}
	var result driver.ListPage
	var lastPrefix string

	dirPrefix := b.dir
	if !strings.HasSuffix(dirPrefix, "/") {
		dirPrefix += "/"
	}

	err := walkDir(walker, root, func(p string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(p, attrsExt) || p == b.dir {
			return nil
		}

		key := unescapeKey(strings.TrimPrefix(p, dirPrefix))

		if info.IsDir() {
			key += "/"
			if len(key) > len(opts.Prefix) && !strings.HasPrefix(key, opts.Prefix) {
				return fs.SkipDir
			}
			if lastPrefix != "" && strings.HasPrefix(key, lastPrefix) {
				return fs.SkipDir
			}

			if pageToken != "" && key < pageToken && !strings.HasPrefix(pageToken, key) {
				return fs.SkipDir
			}

			return nil
		}

		if !strings.HasPrefix(key, opts.Prefix) {
			return nil
		}

		asFunc := func(i any) bool {
			p, ok := i.(*fs.FileInfo)
			if !ok {
				return false
			}
			*p = info
			return true
		}

		obj := &driver.ListObject{
			Key:     key,
			ModTime: info.ModTime(),
			Size:    info.Size(),
			AsFunc:  asFunc,
		}

		if opts.Delimiter != "" {
			keyWithoutPrefix := key[len(opts.Prefix):]
			if idx := strings.Index(keyWithoutPrefix, opts.Delimiter); idx != -1 {
				prefix := opts.Prefix + keyWithoutPrefix[0:idx+len(opts.Delimiter)]
				if prefix == lastPrefix {
					return nil
				}
				obj = &driver.ListObject{
					Key:    prefix,
					IsDir:  true,
					AsFunc: asFunc,
				}
				lastPrefix = prefix
			}
		}

		if pageToken != "" && obj.Key <= pageToken {
			return nil
		}

		if !obj.IsDir {
			if xa, err := getAttrs(ctx, b.client, p); err == nil {
				obj.MD5 = xa.MD5
			}
		}

		result.Objects = append(result.Objects, obj)
		sort.Slice(result.Objects, func(i, j int) bool {
			return result.Objects[i].Key < result.Objects[j].Key
		})

		// Return EOF to halt physical walking when we have verified page coverage seamlessly
		if len(result.Objects) == pageSize+1 {
			return io.EOF
		}

		return nil
	})

	if err != nil && err != io.EOF {
		return nil, err
	}

	if len(result.Objects) > pageSize {
		result.Objects = result.Objects[0:pageSize]
		result.NextPageToken = []byte(result.Objects[pageSize-1].Key)
	}
	return &result, nil
}

type clientWalker struct {
	client *sftp.Client
}

func walkDir(w clientWalker, root string, walkFn func(string, fs.FileInfo, error) error) error {
	info, err := w.client.Stat(root)
	if err != nil {
		return walkFn(root, nil, err)
	}
	err = walkFn(root, info, nil)
	if err != nil {
		if info.IsDir() && err == fs.SkipDir {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return nil
	}
	infos, err := w.client.ReadDir(root)
	if err != nil {
		return walkFn(root, info, err)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name() < infos[j].Name()
	})
	for _, fi := range infos {
		err = walkDir(w, path.Join(root, fi.Name()), walkFn)
		if err != nil {
			if !fi.IsDir() || err != fs.SkipDir {
				return err
			}
		}
	}
	return nil
}

func (b *bucket) As(i any) bool {
	if p, ok := i.(**sftp.Client); ok {
		*p = b.client
		return true
	}
	return false
}

func (b *bucket) ErrorAs(err error, i any) bool {
	switch v := err.(type) {
	case *sftp.StatusError:
		if p, ok := i.(**sftp.StatusError); ok {
			*p = v
			return true
		}
	case *os.PathError:
		if p, ok := i.(**os.PathError); ok {
			*p = v
			return true
		}
	}
	return false
}

func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *driver.ReaderOptions) (driver.Reader, error) {
	p, info, xa, err := b.forKey(ctx, key)
	if err != nil {
		return nil, err
	}
	f, err := b.client.Open(p)
	if err != nil {
		return nil, err
	}

	if opts.BeforeRead != nil {
		if err := opts.BeforeRead(func(i any) bool {
			if p, ok := i.(**sftp.File); ok {
				*p = f
				return true
			}
			return false
		}); err != nil {
			f.Close()
			return nil, err
		}
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
	}
	r := io.Reader(f)
	if length >= 0 {
		r = io.LimitReader(r, length)
	}
	return &reader{
		ctx: ctx,
		r:   r,
		c:   f,
		attrs: driver.ReaderAttributes{
			ContentType: xa.ContentType,
			ModTime:     info.ModTime(),
			Size:        info.Size(),
		},
		stop: watchContextDeadline(ctx, f),
	}, nil
}

type reader struct {
	ctx   context.Context
	r     io.Reader
	c     io.Closer
	attrs driver.ReaderAttributes
	stop  func()
}

func (r *reader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.r == nil {
		return 0, io.EOF
	}
	return r.r.Read(p)
}

func (r *reader) Close() error {
	if r.stop != nil {
		r.stop()
	}
	if r.c == nil {
		return nil
	}
	return r.c.Close()
}

func (r *reader) Attributes() *driver.ReaderAttributes {
	return &r.attrs
}

func (r *reader) As(i any) bool {
	if p, ok := i.(*io.Reader); ok {
		*p = r.r
		return true
	}
	return false
}

func watchContextDeadline(ctx context.Context, f *sftp.File) func() {
	if x, ok := interface{}(f).(interface{ SetDeadline(time.Time) error }); ok {
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				x.SetDeadline(time.Now().Add(-1 * time.Second))
			case <-done:
			}
		}()
		var closeOnce sync.Once
		return func() { closeOnce.Do(func() { close(done) }) }
	}
	return func() {}
}

func (b *bucket) NewTypedWriter(ctx context.Context, key, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	if key == "" {
		return nil, errors.New("sftpblob: invalid key (empty string)")
	}
	p, err := b.fullPath(key)
	if err != nil {
		return nil, err
	}
	if err := b.client.MkdirAll(path.Dir(p)); err != nil {
		return nil, err
	}

	var f *sftp.File
	var tempPath string
	var initErr error

	if opts.IfNotExist {
		f, err = b.client.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL)
		if err != nil {
			if _, statErr := b.client.Stat(p); statErr == nil {
				initErr = gcerr.Newf(gcerr.FailedPrecondition, err, "sftpblob: blob already exists")
				// f remains nil, operations will return initErr
			} else {
				return nil, err
			}
		}
	} else {
		tempPath = p + "." + strconv.FormatInt(time.Now().UnixNano(), 16) + ".tmp"
		f, err = b.client.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_EXCL)
		if err != nil {
			return nil, err
		}
	}

	if opts.BeforeWrite != nil && initErr == nil {
		if err := opts.BeforeWrite(func(i any) bool {
			if p, ok := i.(**sftp.File); ok {
				*p = f
				return true
			}
			return false
		}); err != nil {
			f.Close()
			if tempPath != "" {
				b.client.Remove(tempPath)
			}
			return nil, err
		}
	}

	if b.opts.Metadata == MetadataDontWrite {
		return &writer{
			ctx:        ctx,
			client:     b.client,
			f:          f,
			path:       p,
			tmp:        tempPath,
			contentMD5: opts.ContentMD5,
			md5hash:    md5.New(),
			ifNotExist: opts.IfNotExist,
			mu:         &sync.Mutex{},
			initErr:    initErr,
			stop:       watchContextDeadline(ctx, f),
		}, nil
	}

	var metadata map[string]string
	if len(opts.Metadata) > 0 {
		metadata = opts.Metadata
	}
	attrs := xattrs{
		CacheControl:       opts.CacheControl,
		ContentDisposition: opts.ContentDisposition,
		ContentEncoding:    opts.ContentEncoding,
		ContentLanguage:    opts.ContentLanguage,
		ContentType:        contentType,
		Metadata:           metadata,
	}

	return &writerWithSidecar{
		ctx:        ctx,
		client:     b.client,
		f:          f,
		path:       p,
		tmp:        tempPath,
		attrs:      attrs,
		contentMD5: opts.ContentMD5,
		md5hash:    md5.New(),
		ifNotExist: opts.IfNotExist,
		mu:         &sync.Mutex{},
		initErr:    initErr,
		stop:       watchContextDeadline(ctx, f),
	}, nil
}

type writer struct {
	ctx        context.Context
	client     *sftp.Client
	f          *sftp.File
	path       string
	tmp        string
	contentMD5 []byte
	md5hash    hash.Hash
	mu         *sync.Mutex
	closed     bool
	ifNotExist bool
	initErr    error
	stop       func()
	failed     bool
}

func (w *writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("sftpblob: already closed")
	}
	if w.initErr != nil {
		return 0, w.initErr
	}
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.f.Write(p)
	if err != nil {
		w.md5hash.Write(p[:n])
		w.failed = true
		return n, err
	}
	if _, err := w.md5hash.Write(p); err != nil {
		w.failed = true
		return n, err
	}
	return n, nil
}

func (w *writer) Close() error {
	if w.stop != nil {
		w.stop()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("sftpblob: already closed")
	}
	w.closed = true

	if w.initErr != nil {
		return w.initErr
	}

	if err := w.f.Close(); err != nil {
		return err
	}

	var removeTmp bool
	if w.tmp != "" {
		removeTmp = true
		tempname := w.tmp
		defer func() {
			if removeTmp {
				w.client.Remove(tempname)
			}
		}()
	}

	if w.failed {
		return errors.New("sftpblob: refusing to commit incomplete file write")
	}

	if err := w.ctx.Err(); err != nil {
		return err
	}

	if len(w.contentMD5) > 0 && !bytes.Equal(w.contentMD5, w.md5hash.Sum(nil)) {
		return gcerr.Newf(gcerr.FailedPrecondition, nil, "sftpblob: MD5 checksum mismatch")
	}

	if w.tmp == "" {
		return nil
	}

	err := w.client.PosixRename(w.tmp, w.path)
	if err == nil {
		removeTmp = false
	}
	return err
}

type writerWithSidecar struct {
	ctx        context.Context
	client     *sftp.Client
	f          *sftp.File
	path       string
	tmp        string
	attrs      xattrs
	contentMD5 []byte
	md5hash    hash.Hash
	mu         *sync.Mutex
	closed     bool
	ifNotExist bool
	initErr    error
	stop       func()
	failed     bool
}

func (w *writerWithSidecar) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("sftpblob: already closed")
	}
	if w.initErr != nil {
		return 0, w.initErr
	}
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.f.Write(p)
	if err != nil {
		w.md5hash.Write(p[:n])
		w.failed = true
		return n, err
	}
	if _, err := w.md5hash.Write(p); err != nil {
		w.failed = true
		return n, err
	}
	return n, nil
}

func (w *writerWithSidecar) Close() error {
	if w.stop != nil {
		w.stop()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("sftpblob: already closed")
	}
	w.closed = true

	if w.initErr != nil {
		return w.initErr
	}

	if err := w.f.Close(); err != nil {
		return err
	}

	var removeTmp bool
	if w.tmp != "" {
		removeTmp = true
		tempname := w.tmp
		defer func() {
			if removeTmp {
				w.client.Remove(tempname)
			}
		}()
	}

	if w.failed {
		return errors.New("sftpblob: refusing to commit incomplete file write")
	}

	if err := w.ctx.Err(); err != nil {
		return err
	}

	sum := w.md5hash.Sum(nil)
	if len(w.contentMD5) > 0 && !bytes.Equal(w.contentMD5, sum) {
		return gcerr.Newf(gcerr.FailedPrecondition, nil, "sftpblob: MD5 checksum mismatch")
	}
	w.attrs.MD5 = sum

	if w.tmp == "" {
		return setAttrs(w.ctx, w.client, w.path, w.attrs)
	}

	err := setAttrs(w.ctx, w.client, w.tmp, w.attrs)
	if err != nil {
		return err
	}

	err = w.client.PosixRename(w.tmp, w.path)
	if err != nil {
		_ = w.client.Remove(w.tmp + attrsExt)
		return err
	}

	err = w.client.PosixRename(w.tmp+attrsExt, w.path+attrsExt)
	if err == nil {
		removeTmp = false
	}
	return err
}

func (b *bucket) Copy(ctx context.Context, dstKey, srcKey string, opts *driver.CopyOptions) error {
	if dstKey == "" || srcKey == "" {
		return errors.New("sftpblob: empty key")
	}
	srcPath, err := b.fullPath(srcKey)
	if err != nil {
		return err
	}
	dstPath, err := b.fullPath(dstKey)
	if err != nil {
		return err
	}

	srcFile, err := b.client.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if opts.BeforeCopy != nil {
		if err := opts.BeforeCopy(func(i any) bool {
			if p, ok := i.(**sftp.File); ok {
				*p = srcFile
				return true
			}
			return false
		}); err != nil {
			return err
		}
	}

	if err := b.client.MkdirAll(path.Dir(dstPath)); err != nil {
		return err
	}
	tempPath := dstPath + "." + strconv.FormatInt(time.Now().UnixNano(), 16) + ".tmp"
	dstFile, err := b.client.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		dstFile.Close()
		b.client.Remove(tempPath)
		return err
	}

	_, err = io.Copy(dstFile, srcFile)
	errClose := dstFile.Close()
	if err == nil {
		err = errClose
	}
	if err != nil {
		b.client.Remove(tempPath)
		return err
	}

	var wroteSidecar bool
	if b.opts.Metadata != MetadataDontWrite {
		xa, err := getAttrs(ctx, b.client, srcPath)
		if err != nil {
			b.client.Remove(tempPath)
			return err
		}
		if err := setAttrs(ctx, b.client, tempPath, xa); err != nil {
			b.client.Remove(tempPath)
			return err
		}
		wroteSidecar = true
	}

	err = b.client.PosixRename(tempPath, dstPath)
	if err != nil {
		if wroteSidecar {
			_ = b.client.Remove(tempPath + attrsExt)
		}
		b.client.Remove(tempPath)
		return err
	}

	if wroteSidecar {
		err = b.client.PosixRename(tempPath+attrsExt, dstPath+attrsExt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *bucket) Delete(ctx context.Context, key string) error {
	p, err := b.fullPath(key)
	if err != nil {
		return err
	}
	err = b.client.Remove(p)
	if err != nil {
		return err
	}
	_ = b.client.Remove(p + attrsExt)
	return nil
}

func (b *bucket) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {
	return "", gcerr.Newf(gcerr.Unimplemented, nil, "sftpblob: SignedURL not implemented")
}
