// Package filewatch provides snapshot-based file change detection between agent turns.
package filewatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileInfo stores metadata about a file at snapshot time.
type FileInfo struct {
	Path    string
	ModTime time.Time
	Size    int64
}

// ChangeSet describes what changed between two snapshots.
type ChangeSet struct {
	Created  []string
	Modified []string
	Deleted  []string
}

// IsEmpty returns true if no changes were detected.
func (cs *ChangeSet) IsEmpty() bool {
	return len(cs.Created) == 0 && len(cs.Modified) == 0 && len(cs.Deleted) == 0
}

// String formats the changeset for injection into agent prompts.
func (cs *ChangeSet) String() string {
	if cs.IsEmpty() {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Files Changed Since Last Turn\n\n")
	for _, f := range cs.Created {
		fmt.Fprintf(&b, "- **created**: %s\n", f)
	}
	for _, f := range cs.Modified {
		fmt.Fprintf(&b, "- **modified**: %s\n", f)
	}
	for _, f := range cs.Deleted {
		fmt.Fprintf(&b, "- **deleted**: %s\n", f)
	}
	return b.String()
}

// Watcher takes filesystem snapshots and diffs them between agent turns.
type Watcher struct {
	rootDir  string
	ignore   []string
	last     map[string]FileInfo
	mu       sync.Mutex
}

// NewWatcher creates a watcher for the given directory.
func NewWatcher(rootDir string, ignore []string) *Watcher {
	return &Watcher{
		rootDir: rootDir,
		ignore:  ignore,
		last:    make(map[string]FileInfo),
	}
}

// TakeSnapshot captures current filesystem state and returns changes since last snapshot.
func (w *Watcher) TakeSnapshot() (*ChangeSet, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	current, err := w.scan()
	if err != nil {
		return nil, err
	}

	cs := &ChangeSet{}

	// Find created and modified files
	for path, info := range current {
		old, exists := w.last[path]
		if !exists {
			cs.Created = append(cs.Created, path)
		} else if info.ModTime != old.ModTime || info.Size != old.Size {
			cs.Modified = append(cs.Modified, path)
		}
	}

	// Find deleted files
	for path := range w.last {
		if _, exists := current[path]; !exists {
			cs.Deleted = append(cs.Deleted, path)
		}
	}

	w.last = current
	return cs, nil
}

// InitialSnapshot takes a baseline snapshot without producing a changeset.
func (w *Watcher) InitialSnapshot() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	current, err := w.scan()
	if err != nil {
		return err
	}
	w.last = current
	return nil
}

func (w *Watcher) scan() (map[string]FileInfo, error) {
	files := make(map[string]FileInfo)

	err := filepath.Walk(w.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable files
		}

		// Get relative path
		rel, err := filepath.Rel(w.rootDir, path)
		if err != nil {
			return nil
		}

		// Check ignore patterns
		for _, pattern := range w.ignore {
			if matched, _ := filepath.Match(pattern, info.Name()); matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// Also check directory components
			if info.IsDir() && info.Name() == pattern {
				return filepath.SkipDir
			}
		}

		if info.IsDir() {
			return nil
		}

		files[rel] = FileInfo{
			Path:    rel,
			ModTime: info.ModTime(),
			Size:    info.Size(),
		}
		return nil
	})

	return files, err
}
