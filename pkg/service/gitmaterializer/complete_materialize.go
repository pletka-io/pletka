package gitmaterializer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

// The materializer is a trusted system job over authorized outbox/rebuild work.
// Source traversal is explicit here; public request readers use request policy.
func (m *Materializer) completeFactory() *readsession.Factory {
	return readsession.NewFactory(m.pool).WithSourceAuthorization(func(context.Context, auth.Resource) error { return nil })
}

func (m *Materializer) readComplete(ctx context.Context, project string, scope auth.ReadScope, withCatalog bool) (value projectsnapshot.CompleteProject, catalog projectbundle.Catalog, err error) {
	opts := projectread.DefaultOptions()
	opts.Hosts.Project, opts.Hosts.Ontology = m.moduleHost, m.ontologyHost
	assembler, err := projectread.New(opts)
	if err != nil {
		return value, catalog, err
	}
	err = m.completeFactory().Read(ctx, scope, project, func(s *readsession.Session) error {
		value, err = assembler.Project(ctx, s)
		if err == nil && withCatalog {
			catalog, err = projectread.Catalog(ctx, s, value)
		}
		return err
	})
	return value, catalog, err
}

func writeCompleteFiles(dir string, files projectbundle.Files) error {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		if err := writeEntityFile(dir, path, files[path]); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) buildReleaseTree(ctx context.Context, dir, project, version string) error {
	_, err := m.writeCompleteRelease(ctx, dir, project, version)
	return err
}

func (m *Materializer) writeCompleteRelease(ctx context.Context, dir, project, version string) (string, error) {
	value, catalog, err := m.readComplete(ctx, project, auth.Release(version), true)
	if err != nil {
		return "", err
	}
	// All reads have ended. Encoding and filesystem output consume detached values.
	files, err := projectbundle.Encode(value)
	if err != nil {
		return "", err
	}
	files[projectbundle.CatalogPath], err = projectbundle.EncodeCatalog(catalog)
	if err != nil {
		return "", err
	}
	if err := writeCompleteFiles(dir, files); err != nil {
		return "", err
	}
	records, err := catalog.SelectedReleases(value)
	if err != nil {
		return "", err
	}
	for _, record := range records {
		if record.ProjectID == project && record.Version == version {
			if message := strings.TrimSpace(record.Title + "\n\n" + record.Description); message != "" {
				return message, nil
			}
		}
	}
	return "Release v" + version, nil
}

func storedProjectFormat(data []byte) (int, error) {
	var header struct {
		Version int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return 0, err
	}
	switch header.Version {
	case 1, 2, projectbundle.SchemaVersion:
		return header.Version, nil
	default:
		return 0, fmt.Errorf("unsupported stored project format %d", header.Version)
	}
}

// Historical tag verification selects its stored serializer. It never uses the
// current catalog, rewrites a tag, or promotes partial old files to completeness.
func (m *Materializer) writeVerificationTree(ctx context.Context, git *gitRunner, rev, dir, project, version string) error {
	manifest, ok, err := git.ShowFile(ctx, rev, projectbundle.ManifestPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("tag has no project manifest")
	}
	format, err := storedProjectFormat(manifest)
	if err != nil {
		return err
	}
	frozen, present, err := git.ShowFile(ctx, rev, projectbundle.CatalogPath)
	if err != nil {
		return err
	}
	if format == projectbundle.SchemaVersion {
		value, _, err := m.readComplete(ctx, project, auth.Release(version), false)
		if err != nil {
			return err
		}
		if _, err := projectbundle.DecodeCatalog(value, frozen, projectbundle.DefaultLimits().MaxBytes); err != nil {
			return fmt.Errorf("invalid frozen catalog: %w", err)
		}
		files, err := projectbundle.Encode(value)
		if err != nil {
			return err
		}
		if err := writeCompleteFiles(dir, files); err != nil {
			return err
		}
	} else {
		// Transitional schema1/2 verification adapter. Its archive prerequisites
		// still fail closed; it cannot produce or restore schema3 content.
		if err := m.writeLegacyReleaseSnapshot(ctx, dir, project, version); err != nil {
			return err
		}
	}
	if present {
		return writeEntityFile(dir, projectbundle.CatalogPath, frozen)
	}
	return nil
}
