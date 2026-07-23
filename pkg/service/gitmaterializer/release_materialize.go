package gitmaterializer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// releaseSystemName and releaseSystemEmail are the git identity every release
// and archive commit/tag is authored under. Releases are system-cut, never
// attributed to the actor who requested them.
const (
	releaseSystemName  = "pletka-system"
	releaseSystemEmail = "system@pletka.local"
	releasesBranchRef  = "refs/heads/releases"
)

// buildReleaseTree writes the full release snapshot for version plus the live
// releases/index.yaml into dstDir. It is the deterministic unit both the fresh
// commit path and the determinism test build against: the same (projectID,
// version) always yields a byte-identical directory.
func (m *Materializer) buildReleaseTree(ctx context.Context, dstDir, projectID, version string) error {
	if err := m.writeProjectTreeVersion(ctx, dstDir, projectID, version); err != nil {
		return fmt.Errorf("write release snapshot %s@%s: %w", projectID, version, err)
	}
	idx, err := m.loadReleasesIndex(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load releases index %s: %w", projectID, err)
	}
	data, err := encodeReleasesIndex(idx)
	if err != nil {
		return fmt.Errorf("encode releases index %s: %w", projectID, err)
	}
	return writeEntityFile(dstDir, releasesIndexPath, data)
}

// processReleaseChangeSet materializes a release change set onto
// refs/heads/releases: a commit whose tree is the version snapshot plus the
// releases index, and an annotated tag v<version> pointing at it. Re-runs are
// idempotent — an existing tag is verified against its frozen tree rather than
// re-cut, and a content mismatch is recorded as a persistent outcome, never a
// silent re-tag. It records its own success/failure accounting because the
// dispatch returns before processChangeSet's deferred failure recorder runs.
func (m *Materializer) processReleaseChangeSet(ctx context.Context, cs sqlcgen.WeaveChangeSet) {
	start := time.Now()

	unlock, err := acquireProjectLock(m.baseDir, cs.ProjectID)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("acquire project lock %s: %w", cs.ProjectID, err), start)
		return
	}
	defer unlock()

	_, git, err := m.releaseGitRunner(ctx, cs.ProjectID)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, err, start)
		return
	}

	version := cs.ReleaseVersion
	tagName := "v" + version
	tagRef := "refs/tags/" + tagName

	tagSHA, exists, err := git.RefSHA(ctx, tagRef)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("resolve tag %s: %w", tagRef, err), start)
		return
	}

	if exists {
		m.verifyExistingReleaseTag(ctx, git, cs, tagName, tagSHA, start)
		return
	}

	m.commitFreshRelease(ctx, git, cs, tagName, version, start)
}

// verifyExistingReleaseTag re-derives the snapshot tree for an already-tagged
// release and compares it to the tag's frozen tree. A match is an idempotent
// no-op; a mismatch is recorded loudly as release_tag_mismatch and the tag is
// never re-cut.
func (m *Materializer) verifyExistingReleaseTag(ctx context.Context, git *gitRunner, cs sqlcgen.WeaveChangeSet, tagName, tagSHA string, start time.Time) {
	tmp, err := os.MkdirTemp("", "pletka-rel-verify-")
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("create verify temp dir: %w", err), start)
		return
	}
	defer os.RemoveAll(tmp)

	// Build the snapshot only; the live index may have moved on (archives
	// append entries), so inject the tag's OWN frozen index — the tree the tag
	// actually froze — rather than the current index.
	if err := m.writeProjectTreeVersion(ctx, tmp, cs.ProjectID, cs.ReleaseVersion); err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("write verify snapshot %s@%s: %w", cs.ProjectID, cs.ReleaseVersion, err), start)
		return
	}
	frozen, ok, err := git.ShowFile(ctx, tagSHA, releasesIndexPath)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("read frozen index at %s: %w", tagSHA, err), start)
		return
	}
	if ok {
		if err := writeEntityFile(tmp, releasesIndexPath, frozen); err != nil {
			m.recordReleaseFailure(ctx, cs.ID, err, start)
			return
		}
	}

	computed, err := git.WriteTreeFrom(ctx, tmp)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("write verify tree: %w", err), start)
		return
	}
	tagTree, _, err := git.TreeOfRev(ctx, tagSHA)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("resolve tag tree %s: %w", tagSHA, err), start)
		return
	}

	if computed == tagTree {
		m.logger.Info("release already materialized, no-op",
			"change_set_id", cs.ID, "project_id", cs.ProjectID, "tag", tagName)
		if err := m.markChangeSetProcessed(ctx, cs.ID, nil, runMetrics{outcome: "noop", duration: time.Since(start)}); err != nil {
			m.logger.Error("mark release no-op processed", "change_set_id", cs.ID, "err", err)
		}
		return
	}

	// Persistent state: the tag froze a different tree than the archive now
	// yields. Retrying forever would spin the poller, so mark processed and let
	// the materialized_outcome column be the alarm. NEVER re-tag.
	msg := fmt.Sprintf("tag %s tree %s != archive-derived tree %s", tagName, tagTree, computed)
	m.logger.Error("release tag content mismatch",
		"change_set_id", cs.ID, "project_id", cs.ProjectID, "tag", tagName,
		"tag_tree", tagTree, "computed_tree", computed)
	if err := m.markReleaseProcessed(ctx, cs.ID, nil, "release_tag_mismatch", msg, time.Since(start)); err != nil {
		m.logger.Error("mark release tag mismatch processed", "change_set_id", cs.ID, "err", err)
	}
}

// commitFreshRelease writes the snapshot + live index as a new commit on
// refs/heads/releases and cuts the annotated tag.
func (m *Materializer) commitFreshRelease(ctx context.Context, git *gitRunner, cs sqlcgen.WeaveChangeSet, tagName, version string, start time.Time) {
	tmp, err := os.MkdirTemp("", "pletka-rel-")
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("create release temp dir: %w", err), start)
		return
	}
	defer os.RemoveAll(tmp)

	if err := m.buildReleaseTree(ctx, tmp, cs.ProjectID, version); err != nil {
		m.recordReleaseFailure(ctx, cs.ID, err, start)
		return
	}
	tree, err := git.WriteTreeFrom(ctx, tmp)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("write release tree: %w", err), start)
		return
	}
	parent, _, err := git.RefSHA(ctx, releasesBranchRef)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("resolve releases branch: %w", err), start)
		return
	}

	message := cs.CommitMessage + "\n\nChange-Set: " + strconv.FormatInt(cs.ID, 10)
	commit, err := git.CommitTree(ctx, tree, parent, message, releaseSystemName, releaseSystemEmail)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("commit release tree: %w", err), start)
		return
	}
	if err := git.UpdateRef(ctx, releasesBranchRef, commit); err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("update releases branch: %w", err), start)
		return
	}

	idx, err := m.loadReleasesIndex(ctx, cs.ProjectID)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("load releases index for tag message: %w", err), start)
		return
	}
	if err := git.TagAnnotated(ctx, tagName, releaseTagMessage(idx, version), commit, releaseSystemName, releaseSystemEmail); err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("tag %s: %w", tagName, err), start)
		return
	}

	dur := time.Since(start)
	if err := m.markChangeSetProcessed(ctx, cs.ID, &commit, runMetrics{outcome: "committed", duration: dur, files: countTreeFiles(tmp), changed: countTreeFiles(tmp)}); err != nil {
		m.logger.Error("mark release committed processed", "change_set_id", cs.ID, "err", err)
		return
	}
	m.logger.Info("release materialized",
		"change_set_id", cs.ID, "project_id", cs.ProjectID,
		"tag", tagName, "commit_sha", commit, "duration_ms", dur.Milliseconds())
}

// processReleaseArchivedChangeSet records a release archival as an index-only
// commit on refs/heads/releases. The tag is never touched — the archived flag
// lives in the index, and the frozen tag tree keeps its pre-archive index.
func (m *Materializer) processReleaseArchivedChangeSet(ctx context.Context, cs sqlcgen.WeaveChangeSet) {
	start := time.Now()

	unlock, err := acquireProjectLock(m.baseDir, cs.ProjectID)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("acquire project lock %s: %w", cs.ProjectID, err), start)
		return
	}
	defer unlock()

	_, git, err := m.releaseGitRunner(ctx, cs.ProjectID)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, err, start)
		return
	}

	idx, err := m.loadReleasesIndex(ctx, cs.ProjectID)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("load releases index %s: %w", cs.ProjectID, err), start)
		return
	}
	data, err := encodeReleasesIndex(idx)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("encode releases index %s: %w", cs.ProjectID, err), start)
		return
	}
	message := cs.CommitMessage + "\n\nChange-Set: " + strconv.FormatInt(cs.ID, 10)

	tip, exists, err := git.RefSHA(ctx, releasesBranchRef)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("resolve releases branch: %w", err), start)
		return
	}

	if !exists {
		// Archive recorded before any release was materialized: seed the
		// releases branch with an index-only root commit. Backfill will still
		// tag the release later.
		tmp, terr := os.MkdirTemp("", "pletka-rel-arch-")
		if terr != nil {
			m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("create archive temp dir: %w", terr), start)
			return
		}
		defer os.RemoveAll(tmp)
		if err := writeEntityFile(tmp, releasesIndexPath, data); err != nil {
			m.recordReleaseFailure(ctx, cs.ID, err, start)
			return
		}
		tree, err := git.WriteTreeFrom(ctx, tmp)
		if err != nil {
			m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("write archive tree: %w", err), start)
			return
		}
		commit, err := git.CommitTree(ctx, tree, "", message, releaseSystemName, releaseSystemEmail)
		if err != nil {
			m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("commit archive tree: %w", err), start)
			return
		}
		if err := git.UpdateRef(ctx, releasesBranchRef, commit); err != nil {
			m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("update releases branch: %w", err), start)
			return
		}
		m.finishArchive(ctx, cs, commit, start)
		return
	}

	tree, err := git.WriteTreeReplacingFile(ctx, tip, releasesIndexPath, data)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("rewrite index on releases tip: %w", err), start)
		return
	}
	tipTree, _, err := git.TreeOfRev(ctx, tip)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("resolve releases tip tree: %w", err), start)
		return
	}
	if tree == tipTree {
		m.logger.Info("release archive already materialized, no-op",
			"change_set_id", cs.ID, "project_id", cs.ProjectID, "version", cs.ReleaseVersion)
		if err := m.markChangeSetProcessed(ctx, cs.ID, nil, runMetrics{outcome: "noop", duration: time.Since(start)}); err != nil {
			m.logger.Error("mark archive no-op processed", "change_set_id", cs.ID, "err", err)
		}
		return
	}

	commit, err := git.CommitTree(ctx, tree, tip, message, releaseSystemName, releaseSystemEmail)
	if err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("commit archive tree: %w", err), start)
		return
	}
	if err := git.UpdateRef(ctx, releasesBranchRef, commit); err != nil {
		m.recordReleaseFailure(ctx, cs.ID, fmt.Errorf("update releases branch: %w", err), start)
		return
	}
	m.finishArchive(ctx, cs, commit, start)
}

// finishArchive records a committed archive outcome.
func (m *Materializer) finishArchive(ctx context.Context, cs sqlcgen.WeaveChangeSet, commit string, start time.Time) {
	dur := time.Since(start)
	if err := m.markChangeSetProcessed(ctx, cs.ID, &commit, runMetrics{outcome: "committed", duration: dur}); err != nil {
		m.logger.Error("mark archive committed processed", "change_set_id", cs.ID, "err", err)
		return
	}
	m.logger.Info("release archive materialized",
		"change_set_id", cs.ID, "project_id", cs.ProjectID,
		"version", cs.ReleaseVersion, "commit_sha", commit, "duration_ms", dur.Milliseconds())
}

// releaseGitRunner returns the per-project work dir and a git runner, running
// git init when the repo does not exist yet (a release can be a project's first
// materialization).
func (m *Materializer) releaseGitRunner(ctx context.Context, projectID string) (string, *gitRunner, error) {
	workDir := filepath.Join(m.baseDir, projectID)
	git := newGitRunner(workDir, m.logger)
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		if err := git.Init(ctx); err != nil {
			return "", nil, fmt.Errorf("git init: %w", err)
		}
	}
	return workDir, git, nil
}

// markReleaseProcessed marks a release change set processed with an outcome and
// an accompanying materialized_error (used for release_tag_mismatch).
func (m *Materializer) markReleaseProcessed(ctx context.Context, csID int64, sha *string, outcome, errMsg string, dur time.Duration) error {
	if err := m.markChangeSetProcessed(ctx, csID, sha, runMetrics{outcome: outcome, duration: dur}); err != nil {
		return err
	}
	if errMsg == "" {
		return nil
	}
	if _, err := m.pool.Exec(ctx, `UPDATE weave_change_set SET materialized_error = $2 WHERE id = $1`, csID, errMsg); err != nil {
		return fmt.Errorf("record materialized error: %w", err)
	}
	return nil
}

// recordReleaseFailure records a transient materialization failure without
// marking the change set processed, so the poller retries it — mirroring the
// draft path's failure accounting.
func (m *Materializer) recordReleaseFailure(ctx context.Context, csID int64, err error, start time.Time) {
	msg := err.Error()
	dur := int32(time.Since(start).Milliseconds())
	if rerr := m.queries.WeaveRecordMaterializationFailure(ctx, sqlcgen.WeaveRecordMaterializationFailureParams{
		ID: csID, MaterializedError: &msg, MaterializedDurationMs: &dur,
	}); rerr != nil {
		m.logger.Error("failed to record release materialization failure",
			"change_set_id", csID, "err", rerr)
	}
	m.logger.Error("release materialization failed", "change_set_id", csID, "err", err)
}

// EnqueueMissingReleases walks weave_releases and enqueues a 'release' change
// set for every version whose tag is absent from the project's git repo.
// Idempotent: safe to run once per instance after deploy (the fleet's ~45
// pre-stage-1 releases predate the outbox 'release' kind and have no change
// set at all), and safe to re-run mid-drain or later to heal future drift —
// a version already tagged, or one with an unprocessed 'release' change set
// still pending, is skipped rather than re-enqueued.
func (m *Materializer) EnqueueMissingReleases(ctx context.Context) (enqueued, skipped int, err error) {
	rows, err := m.pool.Query(ctx, `
		SELECT project_id, version FROM weave_releases
		ORDER BY project_id, created_at, version`)
	if err != nil {
		return 0, 0, fmt.Errorf("list releases: %w", err)
	}
	type releaseRow struct {
		projectID string
		version   string
	}
	var releases []releaseRow
	for rows.Next() {
		var r releaseRow
		if err := rows.Scan(&r.projectID, &r.version); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("scan release row: %w", err)
		}
		releases = append(releases, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("iterate release rows: %w", err)
	}

	for _, r := range releases {
		tagged, err := m.releaseTagExists(ctx, r.projectID, r.version)
		if err != nil {
			return enqueued, skipped, err
		}
		if tagged {
			skipped++
			continue
		}

		var pending bool
		if err := m.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM weave_change_set
				WHERE project_id = $1 AND kind = 'release' AND release_version = $2
				  AND processed_at IS NULL
			)`, r.projectID, r.version).Scan(&pending); err != nil {
			return enqueued, skipped, fmt.Errorf("check pending release change set %s@%s: %w", r.projectID, r.version, err)
		}
		if pending {
			skipped++
			continue
		}

		message := fmt.Sprintf("Release v%s (backfill)", r.version)
		if _, err := m.pool.Exec(ctx, `
			INSERT INTO weave_change_set (project_id, actor_id, actor_name, actor_email,
				commit_message, started_at, closed_at, kind, release_version)
			VALUES ($1, '', 'pletka-system', 'system@pletka.local', $2, NOW(), NOW(), 'release', $3)
		`, r.projectID, message, r.version); err != nil {
			return enqueued, skipped, fmt.Errorf("enqueue backfill release %s@%s: %w", r.projectID, r.version, err)
		}
		enqueued++
	}
	return enqueued, skipped, nil
}

// releaseTagExists reports whether projectID has a git repo in m.baseDir with
// tag v<version> already present. A project with no repo yet on disk (never
// materialized) is treated as not tagged, never initialized here — scanning
// must not have the side effect of creating repos for untouched projects.
func (m *Materializer) releaseTagExists(ctx context.Context, projectID, version string) (bool, error) {
	workDir := filepath.Join(m.baseDir, projectID)
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", workDir, err)
	}
	git := newGitRunner(workDir, m.logger)
	_, exists, err := git.RefSHA(ctx, "refs/tags/v"+version)
	if err != nil {
		return false, fmt.Errorf("resolve tag v%s for %s: %w", version, projectID, err)
	}
	return exists, nil
}

// releaseTagMessage builds the annotated-tag message for version from its index
// entry (title + description), falling back to "Release v<version>".
func releaseTagMessage(idx releasesIndex, version string) string {
	for _, e := range idx.Releases {
		if e.Version != version {
			continue
		}
		msg := strings.TrimSpace(strings.TrimSpace(e.Title) + "\n\n" + strings.TrimSpace(e.Description))
		if msg != "" {
			return msg
		}
	}
	return "Release v" + version
}
