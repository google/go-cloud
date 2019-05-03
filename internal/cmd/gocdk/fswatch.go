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

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// waitForDirChange blocks until the directory changes or the context's Done
// channel is closed. waitForDirChange returns nil only if it detected a change.
func waitForDirChange(ctx context.Context, path string, pollInterval time.Duration) error {
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	initSnapshot, err := snapshotTree(path)
	if err != nil {
		return xerrors.Errorf("wait for %s to change: %w", path, err)
	}
	for {
		select {
		case <-tick.C:
			same, err := initSnapshot.sameAs(path)
			if err != nil {
				// TODO(light): Log error.
				continue
			}
			if !same {
				return nil
			}
		case <-ctx.Done():
			return xerrors.Errorf("wait for %s to change: %w", path, ctx.Err())
		}
	}
}

// dirSnapshot is a snapshot of metadata for a directory.
type dirSnapshot struct {
	entries map[string]*dirSnapshotEntry
}

// snapshotTree recursively snapshots a directory.
func snapshotTree(path string) (*dirSnapshot, error) {
	root := &dirSnapshot{entries: make(map[string]*dirSnapshotEntry)}
	err := filepath.Walk(path, func(curr string, info os.FileInfo, err error) error {
		if isIgnoredForWatch(curr) {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err != nil {
			// Only return error for files that aren't ignored.
			return err
		}
		rel := strings.TrimPrefix(curr, path+string(filepath.Separator))
		root.entries[rel] = &dirSnapshotEntry{
			mode:    info.Mode(),
			modTime: info.ModTime(),
		}
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("snapshot tree %s: %w", path, err)
	}
	return root, nil
}

// sameAs compares the directory snapshot to the given filesystem directory.
func (snap *dirSnapshot) sameAs(path string) (bool, error) {
	// TODO(light): This doesn't require a full walk in the case where the
	// directory changed.
	currSnap, err := snapshotTree(path)
	if err != nil {
		return false, xerrors.Errorf("compare directory %s to snapshot: %w", path, err)
	}
	return snap.equal(currSnap), nil
}

// equal compares two directory snapshots for equality.
func (snap *dirSnapshot) equal(snap2 *dirSnapshot) bool {
	if snap == nil && snap2 == nil {
		return true
	}
	if snap == nil || snap2 == nil {
		return false
	}
	if len(snap.entries) != len(snap2.entries) {
		return false
	}
	for k, ent := range snap.entries {
		if !ent.equal(snap2.entries[k]) {
			return false
		}
	}
	return true
}

type dirSnapshotEntry struct {
	mode    os.FileMode
	modTime time.Time
}

// equal compares two entries for equality.
func (ent *dirSnapshotEntry) equal(ent2 *dirSnapshotEntry) bool {
	if ent == nil && ent2 == nil {
		return true
	}
	if ent == nil || ent2 == nil {
		return false
	}
	return ent.mode == ent2.mode && ent.modTime.Equal(ent2.modTime)
}

// isIgnoredForWatch reports whether a given filename is ignored for the
// purposes of snapshotting a directory.
func isIgnoredForWatch(path string) bool {
	basename := filepath.Base(path)
	return basename == ".git" ||
		basename == ".hg" ||
		basename == ".svn" ||
		basename == ".DS_Store"
}
