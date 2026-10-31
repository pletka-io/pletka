package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/rebuildlease"
)

func (m *Materializer) capturedDraft(ctx context.Context, project string) (bool, error) {
	return m.queries.WeaveDraftEnrolled(ctx, project)
}

// initCapturedDraft is explicit enrollment or resumption. All three managed
// entry points use the same revision protocol; none calls the legacy rewriter.
func (m *Materializer) initCapturedDraft(ctx context.Context, project string, enroll bool) error {
	release, err := rebuildlease.Runtime(ctx, m.pool)
	if err != nil {
		return err
	}
	defer release()
	_, err = m.withCapturedRepo(ctx, project, func(git *gitRunner) (int, error) {
		active, err := m.capturedDraft(ctx, project)
		if err != nil {
			return 0, err
		}
		if !active {
			if !enroll {
				return 0, fmt.Errorf("project is not enrolled")
			}
			tip, _, err := git.RefSHA(ctx, CapturedDraftRef)
			if err != nil {
				return 0, err
			}
			if tip != "" {
				return 0, fmt.Errorf("unowned captured draft ref already exists")
			}
			capture, err := m.captureService()
			if err != nil {
				return 0, err
			}
			if _, err := capture.Enroll(ctx, project); err != nil {
				return 0, err
			}
		}
		return m.drainCapturedDraft(ctx, git, project, 0, true)
	})
	return err
}

func (m *Materializer) withCapturedRepo(ctx context.Context, project string, fn func(*gitRunner) (int, error)) (int, error) {
	if m.baseDir == "" || !rebuildProjectID.MatchString(project) {
		return 0, fmt.Errorf("captured draft requires a safe project and repository base")
	}
	if err := os.MkdirAll(m.baseDir, 0750); err != nil {
		return 0, err
	}
	path, err := m.rebuildPath(project, "0.0.0")
	if err != nil {
		return 0, err
	}
	unlock, err := acquireProjectLock(m.baseDir, project)
	if err != nil {
		return 0, err
	}
	defer unlock()
	git := newGitRunner(path, m.logger)
	if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		if err := git.Init(ctx); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	if err := git.checkDraftRef(ctx); err != nil {
		return 0, err
	}
	return fn(git)
}

func (m *Materializer) drainCapturedDraft(ctx context.Context, git *gitRunner, project string, limit int32, repairMissing bool) (int, error) {
	target, err := m.queries.WeaveDraftCommittedRevision(ctx, project)
	if err != nil {
		return 0, err
	}
	processed := 0
	for limit <= 0 || processed < int(limit) {
		last, err := m.queries.WeaveLastDraftDelivery(ctx, project)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return processed, err
		}
		if err == nil && last.AcknowledgedAt.Valid {
			current, _, err := git.RefSHA(ctx, CapturedDraftRef)
			if err != nil {
				return processed, err
			}
			if current == "" && repairMissing {
				// Reconstruct and verify from retained input, never from current hot rows.
				saved := last
				saved.AcknowledgedAt.Valid = false
				if _, err := m.prepareDraft(ctx, git, project, &saved); err != nil {
					return processed, err
				}
				if err := git.publishDraft(ctx, "", last.CommitSha); err != nil {
					return processed, err
				}
			} else if current != last.CommitSha {
				return processed, fmt.Errorf("captured draft ref differs from acknowledged checkpoint")
			}
			if last.Revision >= target {
				return processed, nil
			}
		}
		n, err := m.deliverDraft(ctx, git, project)
		processed += n
		if err != nil {
			return processed, err
		}
	}
	return processed, nil
}

func (m *Materializer) processCapturedPending(ctx context.Context, limit int32) (int, error) {
	projects, err := m.queries.WeavePendingDraftProjects(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, project := range projects {
		remaining := limit - int32(processed)
		if remaining <= 0 {
			break
		}
		n, err := m.withCapturedRepo(ctx, project, func(git *gitRunner) (int, error) { return m.drainCapturedDraft(ctx, git, project, remaining, false) })
		processed += n
		if err != nil {
			m.logger.Error("captured draft delivery pending", "project_id", project, "err", err)
		}
	}
	return processed, nil
}

func (m *Materializer) reconcileCaptured(ctx context.Context, project string) (int, error) {
	release, err := rebuildlease.Runtime(ctx, m.pool)
	if err != nil {
		return 0, err
	}
	defer release()
	return m.withCapturedRepo(ctx, project, func(git *gitRunner) (int, error) {
		before, _, err := git.RefSHA(ctx, CapturedDraftRef)
		if err != nil {
			return 0, err
		}
		if _, err := m.drainCapturedDraft(ctx, git, project, 0, true); err != nil {
			return 0, err
		}
		after, _, err := git.RefSHA(ctx, CapturedDraftRef)
		if err != nil {
			return 0, err
		}
		if before == after {
			return 0, nil
		}
		var paths string
		if before == "" {
			paths, err = git.runOut(ctx, nil, "ls-tree", "-r", "--name-only", after)
		} else {
			paths, err = git.runOut(ctx, nil, "diff", "--name-only", before, after)
		}
		if err != nil {
			return 0, err
		}
		if paths == "" {
			return 0, nil
		}
		return len(strings.Split(paths, "\n")), nil
	})
}
