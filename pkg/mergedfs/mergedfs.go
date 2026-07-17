// Package mergedfs merges multiple fs.FS values into one overlay filesystem.
// Earlier filesystems take precedence over later filesystems.
package mergedfs

import (
	"errors"
	"io/fs"
	"sort"
)

// Merge returns a filesystem that resolves files from the first source that
// contains them. Directory listings are combined by name with earlier sources
// winning duplicates.
func Merge(filesystems ...fs.FS) fs.FS {
	return &MergedFS{filesystems: filesystems}
}

// MergedFS combines filesystems with first-source-wins semantics.
type MergedFS struct {
	filesystems []fs.FS
}

// Open opens name from the first filesystem that contains it.
func (mfs *MergedFS) Open(name string) (fs.File, error) {
	for _, filesystem := range mfs.filesystems {
		if filesystem == nil {
			continue
		}
		file, err := filesystem.Open(name)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fs.ErrNotExist
}

// ReadDir reads and merges directory entries from all filesystems.
func (mfs *MergedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entriesByName := make(map[string]fs.DirEntry)
	found := false

	for _, filesystem := range mfs.filesystems {
		if filesystem == nil {
			continue
		}
		entries, err := fs.ReadDir(filesystem, name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		found = true
		for _, entry := range entries {
			if _, exists := entriesByName[entry.Name()]; exists {
				continue
			}
			entriesByName[entry.Name()] = entry
		}
	}

	if !found {
		return nil, fs.ErrNotExist
	}

	names := make([]string, 0, len(entriesByName))
	for name := range entriesByName {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, entriesByName[name])
	}
	return entries, nil
}

// IsEmptyFS checks whether fsys has any root entries.
func IsEmptyFS(fsys fs.FS) (bool, error) {
	if fsys == nil {
		return true, nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}
