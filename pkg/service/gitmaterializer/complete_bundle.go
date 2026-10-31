package gitmaterializer

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/pletka-io/pletka/pkg/projectbundle"
)

const bundleGitMetadata = ".git"

// CompleteBundle contains imported format-3 values and the separately supplied
// release catalog. The catalog is presentation/history metadata, never frozen
// project content. Loading grants no permission to create a local release cut.
type CompleteBundle struct {
	Project projectbundle.ImportedProject
	Catalog []byte
}

// LoadCompleteProjectBundle reads an explicit format-3 tree with bounded file
// reads and no symlink traversal outside its root. Git metadata is excluded; all
// other files must be accounted for by the codec or the single catalog overlay.
// Formats 1/2 remain on LoadProjectSnapshot and never become complete values.
func LoadCompleteProjectBundle(rootDir string, limits projectbundle.Limits) (CompleteBundle, error) {
	if limits.MaxFiles <= 0 || limits.MaxBytes <= 0 || limits.MaxProjects <= 0 {
		return CompleteBundle{}, fmt.Errorf("bundle limits must be positive")
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return CompleteBundle{}, err
	}
	defer func() { _ = root.Close() }()
	files := projectbundle.Files{}
	var catalog []byte
	remaining := limits.MaxBytes
	entries := 0
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		if path == bundleGitMetadata {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		entries++
		if entries > limits.MaxFiles {
			return fmt.Errorf("bundle directory entry budget exceeded")
		}
		if int64(len(path)) > remaining {
			return fmt.Errorf("bundle path byte budget exceeded")
		}
		remaining -= int64(len(path))
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("bundle symlink %q is unsupported", path)
		}
		if entry.IsDir() {
			return nil
		}
		data, err := readBundleFile(root, path, remaining)
		if err != nil {
			return err
		}
		remaining -= int64(len(data))
		if path == releasesIndexPath {
			catalog = data
		} else {
			files[path] = data
		}
		return nil
	})
	if err != nil {
		return CompleteBundle{}, err
	}
	imported, err := projectbundle.Decode(files, limits)
	if err != nil {
		return CompleteBundle{}, err
	}
	return CompleteBundle{Project: imported, Catalog: catalog}, nil
}

func readBundleFile(root *os.Root, path string, remaining int64) ([]byte, error) {
	info, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("bundle file %q is not regular", path)
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > remaining {
		return nil, fmt.Errorf("bundle file %q exceeds byte budget or is not regular", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, remaining))
	if err != nil {
		return nil, err
	}
	// Check a file that grew after Stat without overflowing a limit+1 calculation.
	var extra [1]byte
	n, err := file.Read(extra[:])
	if n != 0 {
		return nil, fmt.Errorf("bundle file %q grew beyond byte budget", path)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data, nil
}
