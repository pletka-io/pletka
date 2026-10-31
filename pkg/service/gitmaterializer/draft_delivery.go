package gitmaterializer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/draftcapture"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

type draftOperation struct {
	revision                     int64
	digest, name, email, message string
	at                           time.Time
	value                        projectsnapshot.CompleteProject
}

func (m *Materializer) captureService() (*draftcapture.Service, error) {
	opts := projectread.DefaultOptions()
	opts.Hosts.Project, opts.Hosts.Ontology = m.moduleHost, m.ontologyHost
	return draftcapture.New(m.pool, opts, func(context.Context, auth.Resource) error { return nil })
}

func (m *Materializer) draftOperation(ctx context.Context, project string, revision int64, git *gitRunner, parent string) (draftOperation, error) {
	op := draftOperation{revision: revision}
	capture, err := m.captureService()
	if err != nil {
		return op, err
	}
	if revision == 0 {
		baseline, err := m.queries.WeaveDraftBaseline(ctx, project)
		if err != nil {
			return op, err
		}
		op.digest, op.at = baseline.SnapshotDigest, baseline.CreatedAt
		op.name, op.email = releaseSystemName, releaseSystemEmail
		op.message = "Initialize captured draft for project " + project
		op.value, err = capture.Baseline(ctx, project)
		return op, err
	}
	row, err := m.queries.WeaveDraftMutation(ctx, sqlcgen.WeaveDraftMutationParams{ProjectID: project, Revision: revision})
	if err != nil {
		return op, err
	}
	op.digest, op.name, op.email, op.message, op.at = *row.AfterDigest, row.ActorName, row.ActorEmail, row.Message, row.CreatedAt
	receipt, err := capture.Get(ctx, project, revision)
	if err != nil {
		return op, err
	}
	files, err := readCompleteGitFiles(ctx, git, parent)
	if err != nil {
		return op, err
	}
	base, err := projectbundle.Decode(files, projectbundle.DefaultLimits())
	if err != nil {
		return op, err
	}
	before, err := projectbundle.Encode(receipt.Before())
	if err != nil {
		return op, err
	}
	if !sameDraftFiles(files, before) {
		return op, fmt.Errorf("git parent does not match captured before snapshot for revision %d", revision)
	}
	patch, err := receipt.Patch()
	if err != nil {
		return op, err
	}
	op.value, err = projectsnapshot.PatchDraft(base.Snapshot(), patch)
	return op, err
}

func sameDraftFiles(a, b projectbundle.Files) bool {
	if len(a) != len(b) {
		return false
	}
	for path, data := range a {
		if !bytes.Equal(data, b[path]) {
			return false
		}
	}
	return true
}

func (m *Materializer) prepareDraft(ctx context.Context, git *gitRunner, project string, last *sqlcgen.WeaveDraftDelivery) (sqlcgen.WeaveDraftDelivery, error) {
	var empty sqlcgen.WeaveDraftDelivery
	revision := int64(0)
	parent := ""
	if last != nil {
		if last.AcknowledgedAt.Valid {
			return empty, fmt.Errorf("prepare requires an unacknowledged operation or no prior preparation")
		}
		revision, parent = last.Revision, last.ParentCommit
	} else {
		prior, err := m.queries.WeaveLastDraftDelivery(ctx, project)
		if err == nil {
			revision, parent = prior.Revision+1, prior.CommitSha
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return empty, err
		}
		if revision == 0 {
			var err error
			parent, _, err = git.RefSHA(ctx, "HEAD")
			if err != nil {
				return empty, err
			}
		}
	}
	op, err := m.draftOperation(ctx, project, revision, git, parent)
	if err != nil {
		return empty, err
	}
	files, err := projectbundle.Encode(op.value)
	if err != nil {
		return empty, err
	}
	tree, err := git.writeDraftTree(ctx, files)
	if err != nil {
		return empty, err
	}
	message := fmt.Sprintf("%s\n\nDraft-Project: %s\nDraft-Revision: %d\nDraft-Snapshot: %s", op.message, project, revision, op.digest)
	commit, err := git.commitDraftTree(ctx, tree, parent, message, op.name, op.email, op.at)
	if err != nil {
		return empty, err
	}
	// Read back exact Git bytes before moving a ref or acknowledging anything.
	actual, err := readCompleteGitFiles(ctx, git, commit)
	if err != nil {
		return empty, err
	}
	if !sameDraftFiles(files, actual) {
		return empty, fmt.Errorf("prepared draft bytes differ")
	}
	if last != nil {
		if last.CommitSha != commit || last.TreeSha != tree || last.SnapshotDigest != op.digest {
			return empty, fmt.Errorf("prepared draft operation differs from retained receipt")
		}
		return *last, nil
	}
	return m.queries.WeavePrepareDraftDelivery(ctx, sqlcgen.WeavePrepareDraftDeliveryParams{ProjectID: project, Revision: revision, SnapshotDigest: op.digest, ParentCommit: parent, CommitSha: commit, TreeSha: tree})
}

func (m *Materializer) deliverDraft(ctx context.Context, git *gitRunner, project string) (int, error) {
	var prepared *sqlcgen.WeaveDraftDelivery
	last, err := m.queries.WeaveLastDraftDelivery(ctx, project)
	if err == nil && !last.AcknowledgedAt.Valid {
		prepared = &last
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	next, err := m.prepareDraft(ctx, git, project, prepared)
	if err != nil {
		return 0, err
	}
	expected := next.ParentCommit
	if next.Revision == 0 {
		expected = ""
	}
	if err := git.publishDraft(ctx, expected, next.CommitSha); err != nil {
		return 0, err
	}
	acknowledged, err := m.queries.WeaveAcknowledgeDraftDelivery(ctx, sqlcgen.WeaveAcknowledgeDraftDeliveryParams{ProjectID: project, Revision: next.Revision, CommitSha: next.CommitSha})
	if err != nil {
		return 0, err
	}
	if !acknowledged {
		return 0, nil
	}
	return 1, nil
}
