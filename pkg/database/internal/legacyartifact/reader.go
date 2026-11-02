// Package legacyartifact streams one bound archive artifact into owned storage.
// It is private to database infrastructure; consumers receive detached evidence.
package legacyartifact

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pletka-io/pletka/pkg/database/internal/legacyartifactsql"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
)

const chunkBytes = 256 << 10

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Reader binds one project/release to its owning read-session transaction.
type Reader struct {
	db     queryer
	params legacyartifactsql.ReadLegacyArtifactChunksParams
}

// New binds an artifact reader. The read session owns transaction lifetime.
func New(db queryer, projectID, version string) *Reader {
	return &Reader{db: db, params: legacyartifactsql.ReadLegacyArtifactChunksParams{
		ChunkBytes: chunkBytes, ProjectID: projectID, Version: version,
	}}
}

// Read returns the exact original bytes. Semantic/digest validation remains in
// the session's preparer; neither a partial artifact nor borrowed bytes escape.
func (r *Reader) Read(ctx context.Context) (ontologyevidence.Artifact, error) {
	if r == nil || r.db == nil || r.params.ProjectID == "" || r.params.Version == "" {
		return ontologyevidence.Artifact{}, fmt.Errorf("archive artifact reader is uninitialized")
	}
	if err := ctx.Err(); err != nil {
		return ontologyevidence.Artifact{}, err
	}
	// sqlc's :many collector would retain all chunks. This private adapter uses
	// its generated SQL, parameters and row fields but consumes one row at a time.
	// A regression test compares bindings/scans with the generated collector.
	p := r.params
	rows, err := r.db.Query(ctx, legacyartifactsql.ReadLegacyArtifactChunks, p.ChunkBytes, p.ProjectID, p.Version)
	if err != nil {
		return ontologyevidence.Artifact{}, fmt.Errorf("read archive artifact chunks: %w", err)
	}
	defer rows.Close()
	var buffer artifactBuffer
	for rows.Next() {
		var row legacyartifactsql.ReadLegacyArtifactChunksRow
		var data pgtype.DriverBytes
		if err := rows.Scan(&row.Digest, &row.ByteLength, &row.ByteOffset, &data); err != nil {
			return ontologyevidence.Artifact{}, fmt.Errorf("scan archive artifact chunk: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return ontologyevidence.Artifact{}, err
		}
		if err := buffer.append(row, data); err != nil {
			return ontologyevidence.Artifact{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return ontologyevidence.Artifact{}, fmt.Errorf("read archive artifact chunks: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return ontologyevidence.Artifact{}, err
	}
	return buffer.finish()
}

type artifactBuffer struct {
	document strings.Builder
	digest   string
	total    int32
	rows     int
}

func (b *artifactBuffer) append(row legacyartifactsql.ReadLegacyArtifactChunksRow, data []byte) error {
	remaining := int(row.ByteLength) - b.document.Len()
	if row.ByteLength < 0 || remaining < 0 || int(row.ByteOffset) != b.document.Len()+1 || data == nil || len(data) != min(chunkBytes, remaining) {
		return fmt.Errorf("invalid archive artifact chunk bounds")
	}
	if b.rows == 0 {
		b.digest, b.total = row.Digest, row.ByteLength
		b.document.Grow(int(row.ByteLength))
	} else if b.digest != row.Digest || b.total != row.ByteLength || remaining == 0 {
		return fmt.Errorf("archive artifact chunk identity changed or repeated")
	}
	// Write copies before the next driver call; String shares only our owned,
	// immutable final buffer. No unsafe conversion or driver storage is retained.
	_, _ = b.document.Write(data) // strings.Builder.Write always succeeds.
	b.rows++
	return nil
}

func (b *artifactBuffer) finish() (ontologyevidence.Artifact, error) {
	if b.rows == 0 {
		return ontologyevidence.Artifact{}, pgx.ErrNoRows
	}
	if b.document.Len() != int(b.total) {
		return ontologyevidence.Artifact{}, fmt.Errorf("incomplete archive artifact chunks")
	}
	return ontologyevidence.Artifact{JSON: b.document.String(), Digest: b.digest}, nil
}
