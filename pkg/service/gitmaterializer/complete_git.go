package gitmaterializer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

type completeGitBlob struct {
	path, oid string
	size      int64
}

// Read regular Git blobs only, checking declared sizes before reading bodies.
// Decode then enforces canonical paths/inventory/graph and referenced-byte budgets.
func readCompleteGit(ctx context.Context, git *gitRunner, rev string) (projectbundle.ImportedProject, projectbundle.Catalog, error) {
	var empty projectbundle.ImportedProject
	var catalog projectbundle.Catalog
	listing, err := git.runOut(ctx, nil, "ls-tree", "-r", "-l", "-z", rev)
	if err != nil {
		return empty, catalog, err
	}
	limits := projectbundle.DefaultLimits()
	entries := []completeGitBlob{}
	var total int64
	for _, entry := range strings.Split(listing, "\x00") {
		if entry == "" {
			continue
		}
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 4 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			return empty, catalog, fmt.Errorf("complete Git tree requires regular files")
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil || size < 0 || size > limits.MaxBytes || total > limits.MaxBytes-size-int64(len(path)) || len(entries) >= limits.MaxFiles {
			return empty, catalog, fmt.Errorf("complete Git tree exceeds input budget")
		}
		total += size + int64(len(path))
		entries = append(entries, completeGitBlob{path: path, oid: fields[2], size: size})
	}
	files, err := readCompleteBlobs(ctx, git, entries)
	if err != nil {
		return empty, catalog, err
	}
	overlay := files[projectbundle.CatalogPath]
	delete(files, projectbundle.CatalogPath)
	imported, err := projectbundle.Decode(files, limits)
	if err != nil {
		return empty, catalog, err
	}
	catalog, err = projectbundle.DecodeCatalog(imported.Snapshot(), overlay, limits.MaxBytes)
	return imported, catalog, err
}

// One subprocess streams all pre-budgeted blobs. Git archive is unsuitable:
// export-ignore/export-subst attributes can change a stored tree's exact bytes.
func readCompleteBlobs(ctx context.Context, git *gitRunner, entries []completeGitBlob) (projectbundle.Files, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var input strings.Builder
	for _, entry := range entries {
		input.WriteString(entry.oid + "\n")
	}
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch")
	cmd.Dir = git.workDir
	cmd.Stdin = strings.NewReader(input.String())
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	reader := bufio.NewReader(pipe)
	files := projectbundle.Files{}
	for _, entry := range entries {
		header, err := reader.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
		if string(header) != fmt.Sprintf("%s blob %d\n", entry.oid, entry.size) {
			return nil, fmt.Errorf("complete Git blob identity/size changed")
		}
		data := make([]byte, entry.size)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		end, err := reader.ReadByte()
		if err != nil || end != '\n' {
			return nil, fmt.Errorf("invalid Git blob framing")
		}
		files[entry.path] = data
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return files, nil
}

func (m *Materializer) releaseCatalogOverlay(ctx context.Context, git *gitRunner, tip, project string) ([]byte, error) {
	if tip != "" {
		manifest, present, err := git.ShowFile(ctx, tip, projectbundle.ManifestPath)
		if err != nil {
			return nil, err
		}
		if present {
			format, err := storedProjectFormat(manifest)
			if err != nil {
				return nil, err
			}
			if format == projectbundle.SchemaVersion {
				imported, _, err := readCompleteGit(ctx, git, tip)
				if err != nil {
					return nil, err
				}
				value := imported.Snapshot()
				id, err := value.Root()
				if err != nil || id.IsDraft() || id.ProjectID() != project {
					return nil, fmt.Errorf("release branch has an invalid complete selection")
				}
				var overlay projectbundle.Catalog
				err = m.completeFactory().Read(ctx, auth.Release(id.Version()), project, func(s *readsession.Session) error {
					overlay, err = projectread.Catalog(ctx, s, value)
					return err
				})
				if err != nil {
					return nil, err
				}
				return projectbundle.EncodeCatalog(overlay)
			}
		}
	}
	// Existing schema1/2 trees and index-only branches retain their catalog format.
	idx, err := m.loadReleasesIndex(ctx, project)
	if err != nil {
		return nil, err
	}
	return encodeReleasesIndex(idx)
}
