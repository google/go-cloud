// Copyright 2021 The Go Cloud Development Kit Authors
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

package fileblob

import (
	"os"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	// Accessed atomically, guards the use of the unix.Openat2 syscall.
	openat2Supported int32 = 1

	howOpenReadonly = &unix.OpenHow{
		Flags:   unix.O_RDONLY,
		Resolve: unix.RESOLVE_BENEATH,
	}
)

func (b *bucket) osOpen(name string) (*os.File, error) {
	if atomic.LoadInt32(&openat2Supported) == 0 {
		return os.Open(name)
	}

	relativeName := name[len(b.dir)+1:]
	fd, err := unix.Openat2(int(b.rootDirFd), relativeName, howOpenReadonly)
	if err != nil {
		return nil, &os.PathError{Op: "openat2", Path: relativeName, Err: err}
	}
	return os.NewFile(uintptr(fd), name), nil
}

// initFeatures examines OS support to toggle feature flags.
func (b *bucket) initFeatures() error {
	if atomic.LoadInt32(&openat2Supported) == 0 {
		return nil
	}

	// Deliberately use openat2 to tell whether it is supported at all.
	// With b.dir an absolute path, and absent any special flags, dirfd=0 gets ignored.
	dirfd, err := unix.Openat2(0, b.dir, &unix.OpenHow{
		// O_CLOEXEC follows precedent in Go's stdlib,
		// O_PATH is opportune and predates openat2.
		Flags: unix.O_PATH | unix.O_CLOEXEC,
	})
	switch err {
	case nil:
		b.rootDirFd = uintptr(dirfd)
	case syscall.ENOSYS, syscall.EPERM:
		// EPERM – as openBucket already interacted with the directory –
		// will be the result of shambolic syscall whitelists.
		atomic.StoreInt32(&openat2Supported, 0)
		err = nil
	}
	return err
}

func (b *bucket) Close() error {
	if dirfd := int(b.rootDirFd); dirfd != 0 {
		b.rootDirFd = 0
		unix.Close(dirfd)
	}
	return nil
}
