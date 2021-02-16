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
	"io"
	"os"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
)

type fileLike interface {
	syscall.Conn
	Seek(offset int64, whence int) (ret int64, err error)
	Stat() (os.FileInfo, error)
}

// ioCopy tries optimized copy operations, else falls back to io.Copy.
func (b *bucket) ioCopy(dst io.Writer, src io.Reader) (written int64, err error) {
	if atomic.LoadInt32(&b.fsSupportsReflink) == 0 {
		return io.Copy(dst, src)
	}

	srcFile, okSrc := src.(fileLike)
	dstFile, okDst := dst.(fileLike)
	if !(okSrc && okDst) {
		return io.Copy(dst, src)
	}
	written, err = ioCopyReflink(dstFile, srcFile)
	switch err {
	case nil:
		return written, nil
	case syscall.ENOSYS:
		atomic.StoreInt32(&copyReflinkSupported, 0)
		fallthrough
	case syscall.EOPNOTSUPP, syscall.EIO, syscall.EINVAL, syscall.EBADF, syscall.EPERM:
		// syscall.EIO is thrown on netshares.
		atomic.StoreInt32(&b.fsSupportsReflink, 0)
	}
	return io.Copy(dst, src)
}

// ioCopyReflink copies from srcFile to dstFile using clone/copy-on-write
// system calls.
//
// On errors syscall.{EINVAL, EBADF} don't try again with the given files,
// on syscall.EOPNOTSUPP skip for the filesystem.
func ioCopyReflink(dstFile, srcFile fileLike) (written int64, err error) {
	srcPos, err := srcFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return
	}
	dstPos, err := dstFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return
	}
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return
	}
	srcLength := srcInfo.Size() - srcPos // Remaining size.

	if srcPos == 0 && dstPos == 0 {
		err = withFd(dstFile, srcFile, unix.IoctlFileClone)
	} else {
		srcRange := &unix.FileCloneRange{
			Src_offset:  uint64(srcPos),
			Src_length:  uint64(srcLength),
			Dest_offset: uint64(dstPos),
		}
		err = withFd(dstFile, srcFile, func(dst, src int) (err error) {
			srcRange.Src_fd = int64(src)
			return unix.IoctlFileCloneRange(dst, srcRange)
		})
	}
	if err == nil {
		written = srcLength
	}
	return
}

// withFd wraps innerFn providing it the file descriptors without disqualifying
// them from polling.
//
// See also:
// - https://github.com/golang/go/issues/24331
func withFd(a, b syscall.Conn, innerFn func(aFd, bFd int) error) (err error) {
	ac, err := a.SyscallConn()
	if err != nil {
		return
	}
	bc, err := b.SyscallConn()
	if err != nil {
		return
	}
	// Cascading this way will retain the innermost error.
	errIn := ac.Control(func(aFd uintptr) {
		errIn := bc.Control(func(bFd uintptr) {
			for {
				err = innerFn(int(aFd), int(bFd))
				if err != syscall.EINTR {
					break
				}
			}
		})
		if err == nil {
			err = errIn
		}
	})
	if err == nil {
		err = errIn
	}
	return
}
