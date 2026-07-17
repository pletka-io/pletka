package gitmaterializer

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type pletkaSumEntry struct {
	Module  string
	Version string
	TreeSHA string
	GitSHA  string
}

func encodePletkaSum(entries []pletkaSumEntry) []byte {
	sorted := append([]pletkaSumEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Module != sorted[j].Module {
			return sorted[i].Module < sorted[j].Module
		}
		return sorted[i].Version < sorted[j].Version
	})

	var b strings.Builder
	for _, entry := range sorted {
		b.WriteString(entry.Module)
		b.WriteByte(' ')
		b.WriteString(strings.TrimSpace(entry.Version))
		if entry.GitSHA != "" {
			b.WriteString(" git:")
			b.WriteString(strings.TrimSpace(entry.GitSHA))
		}
		if entry.TreeSHA != "" {
			b.WriteString(" tree:sha256:")
			b.WriteString(strings.TrimSpace(entry.TreeSHA))
		}
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func writePletkaSum(workDir string, entries []pletkaSumEntry) error {
	payload := encodePletkaSum(entries)
	return writeEntityFile(workDir, "pletka.sum", payload)
}

func hashDirectoryTree(root string) (string, error) {
	h := sha256.New()

	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk tree %s: %w", root, err)
	}
	sort.Strings(files)

	for _, rel := range files {
		if _, err := io.WriteString(h, rel); err != nil {
			return "", err
		}
		if _, err := h.Write([]byte{0}); err != nil {
			return "", err
		}
		f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", fmt.Errorf("open %s for hashing: %w", rel, err)
		}
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return "", fmt.Errorf("hash %s: %w", rel, err)
		}
		_ = f.Close()
		if _, err := h.Write([]byte{0}); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
