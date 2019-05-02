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
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/xerrors"
)

// waitForDirChange blocks until the directory changes or the context's Done
// channel is closed.
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
	entries []*dirSnapshotEntry // sorted by name
}

// newDirSnapshot converts a list of os.FileInfo into a directory snapshot.
// It does not fill in any of the entries' snapshot fields.
func newDirSnapshot(contents []os.FileInfo) *dirSnapshot {
	snap := &dirSnapshot{
		entries: make([]*dirSnapshotEntry, 0, len(contents)),
	}
	for _, info := range contents {
		name := info.Name()
		if isIgnoredForWatch(name) {
			continue
		}
		snap.entries = append(snap.entries, &dirSnapshotEntry{
			name:    name,
			mode:    info.Mode(),
			modTime: info.ModTime(),
		})
	}
	sort.Slice(snap.entries, func(i, j int) bool {
		return snap.entries[i].name < snap.entries[j].name
	})
	return snap
}

// snapshotTree recursively snapshots a directory.
func snapshotTree(path string) (*dirSnapshot, error) {
	// Unfortunately filepath.Walk does not work well for this because
	// we need to keep track of the parent snapshot variable to write to.

	type walkNode struct {
		dst  **dirSnapshot
		path string
	}

	// Depth-first search over the directory structure.
	var root *dirSnapshot
	stack := []walkNode{{&root, path}}
	for len(stack) > 0 {
		// Pop next node off stack.
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// Read directory and create snapshot.
		contents, err := ioutil.ReadDir(curr.path)
		if err != nil {
			return nil, xerrors.Errorf("snapshot tree %s: %w", path, err)
		}
		snap := newDirSnapshot(contents)
		// Save snapshot in requested variable.
		*curr.dst = snap

		// Push subdirectories onto stack in reverse order, so they are popped in
		// ascending order.
		for i := len(snap.entries) - 1; i >= 0; i-- {
			ent := snap.entries[i]
			if !ent.mode.IsDir() {
				continue
			}
			stack = append(stack, walkNode{&ent.snapshot, filepath.Join(curr.path, ent.name)})
		}
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
	return snap.recursiveEqual(currSnap), nil
}

// equal compares two directory snapshots for equality non-recursively.
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
	for i := range snap.entries {
		if !snap.entries[i].equal(snap2.entries[i]) {
			return false
		}
	}
	return true
}

// recursiveEqual compares two directory snapshots for equality recursively.
func (snap *dirSnapshot) recursiveEqual(snap2 *dirSnapshot) bool {
	type walkNode struct {
		snap1, snap2 *dirSnapshot
	}

	// Depth-first traversal.
	stack := []walkNode{{snap, snap2}}
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !curr.snap1.equal(curr.snap2) {
			return false
		}
		for i := range curr.snap1.entries {
			if curr.snap1.entries[i].snapshot == nil {
				continue
			}
			stack = append(stack, walkNode{
				curr.snap1.entries[i].snapshot,
				curr.snap2.entries[i].snapshot,
			})
		}
	}
	return true
}

type dirSnapshotEntry struct {
	name    string
	mode    os.FileMode
	modTime time.Time

	// snapshot is filled in by snapshotTree if the entry is a directory.
	snapshot *dirSnapshot
}

// equal compares two entries for equality non-recursively.
func (ent *dirSnapshotEntry) equal(ent2 *dirSnapshotEntry) bool {
	if ent == nil && ent2 == nil {
		return true
	}
	if ent == nil || ent2 == nil {
		return false
	}
	return ent.name == ent2.name && ent.mode == ent2.mode && ent.modTime.Equal(ent2.modTime)
}

// isIgnoredForWatch reports whether a given filename is ignored for the
// purposes of snapshotting a directory.
func isIgnoredForWatch(basename string) bool {
	return basename == ".git" ||
		basename == ".hg" ||
		basename == ".svn" ||
		basename == ".DS_Store"
}
