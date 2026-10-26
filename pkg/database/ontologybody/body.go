// Package ontologybody owns sealed local ontology bodies. A seal freezes body
// bytes and their interpretation context; it does not certify a dependency
// closure or reconstruct the content seen by historical project releases.
package ontologybody

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// OntologyID identifies the owner of a body, independently of project identity.
type OntologyID string

// VersionID is a stable storage identity, not an ontology's version label.
type VersionID string

// Reference cannot be retargeted after validation. Ontology IDs are deliberately
// not project IDs, and version labels are never interpreted as global identities.
type Reference struct {
	ontology OntologyID
	version  VersionID
}

// NewReference validates both identity components before a reader is constructed.
func NewReference(ontology OntologyID, version VersionID) (Reference, error) {
	if strings.TrimSpace(string(ontology)) == "" || strings.TrimSpace(string(version)) == "" {
		return Reference{}, fmt.Errorf("ontology body requires an ontology and version identity")
	}
	return Reference{ontology: ontology, version: version}, nil
}

// OntologyID returns the bound owner.
func (r Reference) OntologyID() OntologyID { return r.ontology }

// VersionID returns the bound version identity.
func (r Reference) VersionID() VersionID { return r.version }
func (r Reference) valid() bool          { return r.ontology != "" && r.version != "" }

// Body format 1 uses PostgreSQL's canonical JSONB text, with sorted record
// arrays, hashed as UTF-8 SHA256. This is a local storage evidence format, not
// the final portable vendor/restore format. Policy and audit times are excluded.
type Body struct {
	Reference     Reference
	FormatVersion int16
	CanonicalJSON string
	Digest        string
	SealedAt      time.Time
}

// Verify checks the stored format and digest before exposing body values.
func (b Body) Verify() error {
	if b.FormatVersion != 1 || fmt.Sprintf("%x", sha256.Sum256([]byte(b.CanonicalJSON))) != b.Digest {
		return fmt.Errorf("ontology body format or digest mismatch")
	}
	return nil
}

type sealQueries interface {
	WeaveSealOntologyVersionBody(context.Context, sqlcgen.WeaveSealOntologyVersionBodyParams) error
}

// Seal is for transaction-owning publication infrastructure. Supply the same
// transaction used to finish body writes. Never auto-certify historical data.
// Storage recomputes the body/digest, locks all writers out and rejects resealing.
func Seal(ctx context.Context, q sealQueries, ref Reference) error {
	if !ref.valid() {
		return fmt.Errorf("cannot seal an invalid ontology reference")
	}
	return MapError(q.WeaveSealOntologyVersionBody(ctx, sqlcgen.WeaveSealOntologyVersionBodyParams{OntologyID: string(ref.ontology), VersionID: string(ref.version)}))
}

type sealedError struct{ cause error }

func (e *sealedError) Error() string           { return "ontology version body is sealed; create a new version" }
func (e *sealedError) Unwrap() error           { return e.cause }
func (e *sealedError) ConflictMessage() string { return e.Error() }

// MapError preserves the database cause and supplies the canonical conflict
// adapter for every writer, including callers of the legacy ontology store.
func MapError(err error) error {
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && (pgerr.ConstraintName == "ontology_body_sealed" || (pgerr.Code == "23503" && strings.HasPrefix(pgerr.ConstraintName, "weave_ontology_body_seals_"))) {
		return &sealedError{cause: err}
	}
	return err
}

type missingError struct{ ref Reference }

func (e *missingError) Error() string {
	return fmt.Sprintf("ontology %s version %s has no sealed body", e.ref.ontology, e.ref.version)
}
func (e *missingError) IsNotInRelease() bool { return true }

func readError(err error, ref Reference) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &missingError{ref: ref}
	}
	return err
}
