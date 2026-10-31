// Package draftcapture owns opt-in atomic draft mutations. Its callback is for
// database work only; network/filesystem side effects belong after commit.
package draftcapture

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/database/txscope"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

// Service owns capture transactions and immutable receipt retrieval.
type Service struct {
	pool      *pgxpool.Pool
	reads     *readsession.Factory
	assembler *projectread.Assembler
}

// New configures the infrastructure boundary. Callers authorize the root edit;
// source authorization is explicit, as at every other assembly boundary.
func New(pool *pgxpool.Pool, options projectread.Options, sources readsession.SourceAuthorization) (*Service, error) {
	if pool == nil || sources == nil {
		return nil, fmt.Errorf("draft capture requires a pool and source policy")
	}
	a, err := projectread.New(options)
	if err != nil {
		return nil, err
	}
	return &Service{pool: pool, reads: readsession.NewFactory(pool).WithSourceAuthorization(sources), assembler: a}, nil
}

// Request is attributable operation identity, never inferred from changed rows.
type Request struct{ ProjectID, ActorID, ActorName, ActorEmail, Message string }

// Receipt exposes complete before/after values captured by one committed
// transaction. These are retained inputs, not reconstructions from current rows.
type Receipt struct {
	project       string
	revision      int64
	before, after projectsnapshot.CompleteProject
}

// ProjectID returns the owning project.
func (r Receipt) ProjectID() string { return r.project }

// Revision returns the committed per-project sequence number.
func (r Receipt) Revision() int64 { return r.revision }

// Before returns the retained prior snapshot.
func (r Receipt) Before() projectsnapshot.CompleteProject { return r.before }

// After returns the retained resulting snapshot.
func (r Receipt) After() projectsnapshot.CompleteProject { return r.after }

// Enroll creates revision zero explicitly and returns the exact baseline for a
// subsequent Git initialization. Existing enrollment fails instead of resetting
// history. No project is enrolled merely by running a migration or ordinary save.
func (s *Service) Enroll(ctx context.Context, project string) (projectsnapshot.CompleteProject, error) {
	var value projectsnapshot.CompleteProject
	err := s.transaction(ctx, project, func(bound context.Context, q *sqlcgen.Queries) error {
		v, err := s.assemble(bound, project)
		if err != nil {
			return err
		}
		digest, err := storeSnapshot(bound, q, v)
		if err != nil {
			return err
		}
		if err := q.WeaveEnrollDraft(bound, sqlcgen.WeaveEnrollDraftParams{ProjectID: project, SnapshotDigest: digest}); err != nil {
			return err
		}
		value, err = loadSnapshot(bound, q, digest)
		return err
	})
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	return value, nil
}

// Mutate commits store writes, generated/cascaded row effects, immutable snapshot
// objects and the next project revision together. The callback's context must be
// propagated to stores. Losing it fails the enrolled project's database guard.
func (s *Service) Mutate(ctx context.Context, request Request, fn func(context.Context) error) (Receipt, error) {
	if fn == nil {
		return Receipt{}, fmt.Errorf("draft mutation requires a callback")
	}
	return s.run(ctx, request, "edit", fn)
}

// RefreshContext records external actor/default/source-policy changes under an
// explicit operation. It has no write callback and cannot borrow a user's message
// opportunistically. Managed content writes cannot occur outside Mutate.
func (s *Service) RefreshContext(ctx context.Context, request Request) (Receipt, error) {
	return s.run(ctx, request, "context_refresh", nil)
}

func (s *Service) run(ctx context.Context, request Request, purpose string, fn func(context.Context) error) (Receipt, error) {
	if err := validateRequest(ctx, request); err != nil {
		return Receipt{}, err
	}
	var result Receipt
	err := s.transaction(ctx, request.ProjectID, func(bound context.Context, q *sqlcgen.Queries) error {
		state, err := q.WeaveDraftState(bound, request.ProjectID)
		if err != nil {
			return fmt.Errorf("load enrolled draft: %w", err)
		}
		before, err := loadSnapshot(bound, q, state.SnapshotDigest)
		if err != nil {
			return err
		}
		current, err := s.assemble(bound, request.ProjectID)
		if err != nil {
			return err
		}
		if purpose == "edit" {
			digest, err := storeSnapshot(bound, q, current)
			if err != nil {
				return err
			}
			if digest != state.SnapshotDigest {
				return fmt.Errorf("draft context changed outside its revision; explicit context refresh is required")
			}
		}
		row, err := q.WeaveStartDraftMutation(bound, sqlcgen.WeaveStartDraftMutationParams{ProjectID: request.ProjectID, Purpose: purpose, ActorID: request.ActorID, ActorName: request.ActorName, ActorEmail: request.ActorEmail, Message: request.Message})
		if err != nil {
			return err
		}
		if fn != nil {
			bound = domain.ContextWithChangeSetHint(bound, domain.ChangeSetHint{ProjectID: request.ProjectID, ActorID: request.ActorID, ActorName: request.ActorName, ActorEmail: request.ActorEmail, CommitMessage: request.Message})
			if err := fn(bound); err != nil {
				return err
			}
			current, err = s.assemble(bound, request.ProjectID)
			if err != nil {
				return err
			}
		}
		digest, err := storeSnapshot(bound, q, current)
		if err != nil {
			return err
		}
		n, err := q.WeaveSealDraftMutation(bound, sqlcgen.WeaveSealDraftMutationParams{ProjectID: request.ProjectID, Revision: row.Revision, AfterDigest: &digest})
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("draft mutation was already sealed")
		}
		n, err = q.WeaveAdvanceDraftState(bound, sqlcgen.WeaveAdvanceDraftStateParams{ProjectID: request.ProjectID, Revision: row.Revision, SnapshotDigest: digest})
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("draft revision changed concurrently")
		}
		// Return the same canonical order a delayed reader will receive.
		after, err := loadSnapshot(bound, q, digest)
		if err != nil {
			return err
		}
		result = Receipt{project: request.ProjectID, revision: row.Revision, before: before, after: after}
		_, err = result.Patch()
		return err
	})
	if err != nil {
		return Receipt{}, err
	}
	return result, nil
}

func validateRequest(ctx context.Context, request Request) error {
	if strings.TrimSpace(request.ActorName) == "" || strings.TrimSpace(request.ActorEmail) == "" || strings.TrimSpace(request.Message) == "" {
		return fmt.Errorf("draft capture requires explicit actor and message")
	}
	if cs := domain.ChangeSetFromContext(ctx); cs != nil {
		return fmt.Errorf("draft capture must precede change-set creation")
	}
	return nil
}

// Get reads one immutable committed receipt; it never consults hot content.
func (s *Service) Get(ctx context.Context, project string, revision int64) (Receipt, error) {
	if s == nil || s.pool == nil {
		return Receipt{}, fmt.Errorf("draft capture is not configured")
	}
	q := sqlcgen.New(s.pool)
	row, err := q.WeaveDraftMutation(ctx, sqlcgen.WeaveDraftMutationParams{ProjectID: project, Revision: revision})
	if err != nil {
		return Receipt{}, err
	}
	before, err := loadSnapshot(ctx, q, row.BeforeDigest)
	if err != nil {
		return Receipt{}, err
	}
	after, err := loadSnapshot(ctx, q, *row.AfterDigest)
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{project: project, revision: revision, before: before, after: after}, nil
}

func (s *Service) assemble(ctx context.Context, project string) (projectsnapshot.CompleteProject, error) {
	var value projectsnapshot.CompleteProject
	err := s.reads.Read(ctx, auth.Draft(), project, func(session *readsession.Session) error {
		var err error
		value, err = s.assembler.Project(ctx, session)
		return err
	})
	return value, err
}

func (s *Service) transaction(ctx context.Context, project string, fn func(context.Context, *sqlcgen.Queries) error) error {
	if s == nil || s.pool == nil || strings.TrimSpace(project) == "" {
		return fmt.Errorf("draft transaction requires a configured project")
	}
	if _, scoped, err := txscope.Current(ctx, s.pool); err != nil {
		return err
	} else if scoped {
		return fmt.Errorf("draft capture must own the outer transaction")
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	q := sqlcgen.New(conn)
	// Locks precede BEGIN so a waiter does not retain a stale RR snapshot.
	if err := q.WeaveLockDraftCapture(ctx, project); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = conn.Conn().Close(cleanup)
		conn.Release()
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		ok, err := q.WeaveUnlockDraftCapture(cleanup, project)
		if err != nil || ok == nil || !*ok {
			_ = conn.Conn().Close(cleanup)
		}
		conn.Release()
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := txscope.Within(ctx, s.pool, tx, func(bound context.Context) error { return fn(bound, sqlcgen.New(tx)) }); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func storeSnapshot(ctx context.Context, q *sqlcgen.Queries, value projectsnapshot.CompleteProject) (string, error) {
	files, err := projectbundle.Encode(value)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	return q.WeaveStoreDraftSnapshot(ctx, data)
}

func loadSnapshot(ctx context.Context, q *sqlcgen.Queries, digest string) (projectsnapshot.CompleteProject, error) {
	limits := projectbundle.DefaultLimits()
	size, err := q.WeaveDraftSnapshotSize(ctx, digest)
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	if size.Files < 1 || size.Files > int64(limits.MaxFiles) || size.Bytes > limits.MaxBytes {
		return projectsnapshot.CompleteProject{}, fmt.Errorf("stored draft snapshot exceeds budget")
	}
	rows, err := q.WeaveDraftSnapshotFiles(ctx, digest)
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	files := make(projectbundle.Files, len(rows))
	for _, row := range rows {
		files[row.Path] = row.Body
	}
	value, err := projectbundle.Decode(files, limits)
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	return value.Snapshot(), nil
}
