package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// defaultModuleHost and defaultOntologyHost seed the module-path namespace
// written into generated pletka.mod / ontology.yaml files. They are the
// public-clean defaults; a hosted deployment overrides them via
// WithModuleHosts so materialized repos stay consistent with its existing
// history (e.g. git.example.org).
const (
	defaultModuleHost   = "pletka.io"
	defaultOntologyHost = "ontology.pletka.io"
)

// Materializer drains weave_change_set/weave_change_log and commits
// changes to a per-project git working tree.
type Materializer struct {
	pool             *pgxpool.Pool
	queries          *sqlcgen.Queries
	baseDir          string
	logger           *slog.Logger
	ontologyImporter VendoredOntologyImporter
	moduleHost       string
	ontologyHost     string
}

// NewMaterializer constructs a Materializer.
// baseDir is the parent directory; project "LA" lives at <baseDir>/LA.
func NewMaterializer(pool *pgxpool.Pool, baseDir string, logger *slog.Logger) *Materializer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Materializer{
		pool:         pool,
		queries:      sqlcgen.New(pool),
		baseDir:      baseDir,
		logger:       logger.With("component", "gitmaterializer"),
		moduleHost:   defaultModuleHost,
		ontologyHost: defaultOntologyHost,
	}
}

// WithOntologyImporter sets the importer used to materialize vendored
// ontology snapshots during restore, and returns m for chaining.
func (m *Materializer) WithOntologyImporter(imp VendoredOntologyImporter) *Materializer {
	m.ontologyImporter = imp
	return m
}

// WithModuleHosts overrides the project/ontology module-path hosts, and
// returns m for chaining. An empty argument leaves the corresponding default
// (set in NewMaterializer) in place, so a partial override is safe.
func (m *Materializer) WithModuleHosts(moduleHost, ontologyHost string) *Materializer {
	if moduleHost != "" {
		m.moduleHost = moduleHost
	}
	if ontologyHost != "" {
		m.ontologyHost = ontologyHost
	}
	return m
}

// ProcessPending drains up to limit unprocessed change sets.
func (m *Materializer) ProcessPending(ctx context.Context, limit int32) (int, error) {
	changeSets, err := m.queries.WeaveListUnprocessedChangeSets(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("list unprocessed change sets: %w", err)
	}

	processed := 0
	for _, cs := range changeSets {
		if err := m.processChangeSet(ctx, cs); err != nil {
			m.logger.Error("failed to process change set",
				"change_set_id", cs.ID, "err", err)
			continue
		}
		processed++
	}
	return processed, nil
}

func (m *Materializer) processChangeSet(ctx context.Context, cs sqlcgen.WeaveChangeSet) (err error) {
	start := time.Now()
	// On any failure, record outcome='failed' + error on the change set
	// WITHOUT marking it processed, so the poller retries it.
	defer func() {
		if err == nil {
			return
		}
		dur := int32(time.Since(start).Milliseconds())
		msg := err.Error()
		if rerr := m.queries.WeaveRecordMaterializationFailure(ctx, sqlcgen.WeaveRecordMaterializationFailureParams{
			ID: cs.ID, MaterializedError: &msg, MaterializedDurationMs: &dur,
		}); rerr != nil {
			m.logger.Error("failed to record materialization failure",
				"change_set_id", cs.ID, "err", rerr)
		}
	}()

	// Implicit CLI/import mutations carry project_id='' (weave store
	// resolveChangeSet fallback) and a project deletion can orphan its
	// pending sets; neither can ever materialize. Mark them processed as
	// 'orphaned' instead of retrying forever.
	orphaned := cs.ProjectID == ""
	if !orphaned {
		if _, perr := m.queries.WeaveGetProjectByID(ctx, cs.ProjectID); perr != nil {
			if !errors.Is(perr, pgx.ErrNoRows) {
				return fmt.Errorf("look up project %s: %w", cs.ProjectID, perr)
			}
			orphaned = true
		}
	}
	if orphaned {
		m.logger.Info("change set orphaned, marking processed",
			"change_set_id", cs.ID, "project_id", cs.ProjectID)
		return m.markChangeSetProcessed(ctx, cs.ID, nil, runMetrics{
			outcome: "orphaned", duration: time.Since(start),
		})
	}

	// Release change sets never touch the default-branch draft tree; they
	// commit to refs/heads/releases and cut annotated tags. These processors
	// acquire the project lock themselves and record their own outcome/failure
	// accounting, so dispatch here — before the draft path's lock acquisition —
	// to avoid deadlocking on a second flock of the same lock file.
	switch cs.Kind {
	case "release":
		m.processReleaseChangeSet(ctx, cs)
		return nil
	case "release_archived":
		m.processReleaseArchivedChangeSet(ctx, cs)
		return nil
	}

	unlock, err := acquireProjectLock(m.baseDir, cs.ProjectID)
	if err != nil {
		return fmt.Errorf("acquire project lock %s: %w", cs.ProjectID, err)
	}
	defer unlock()

	workDir := filepath.Join(m.baseDir, cs.ProjectID)
	git := newGitRunner(workDir, m.logger)

	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		if err := git.Init(ctx); err != nil {
			return fmt.Errorf("git init: %w", err)
		}
	}

	// Scoped rewrite: expand this change_set's change_log entries to the
	// within-project closure of entities whose materialized files derive
	// from them, then rewrite/delete exactly those files. Reuses the same
	// single-entity writers the whole-tree path uses, so scoped and full
	// output stay byte-identical (see TestScopedEqualsFullRebuild_*). This
	// replaces the previous full-tree rebuild-and-diff, which re-read
	// CURRENT DB state on every change_set — when two change_sets landed
	// close together, the first one processed could absorb a later,
	// unrelated change_set's diff and commit it under the wrong message,
	// leaving the later change_set a no-op (see TestNoCrossEntityStealing).
	refs, err := m.closure(ctx, cs)
	if err != nil {
		return fmt.Errorf("compute closure for %s change set %d: %w", cs.ProjectID, cs.ID, err)
	}
	if err := m.scopedRewrite(ctx, workDir, cs.ProjectID, refs); err != nil {
		return fmt.Errorf("scoped rewrite %s change set %d: %w", cs.ProjectID, cs.ID, err)
	}
	// files = scoped write size (this change_set's closure), not whole-tree.
	files := len(refs)
	if err := git.AddAll(ctx); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	// changed = files git actually staged (this commit's diff size).
	changed := git.StagedFileCount(ctx)

	// No diff after re-materializing (e.g. a save that changed nothing on
	// disk, or an audit-only entity type): mark processed, skip the commit.
	if !git.HasStagedChanges(ctx) {
		dur := time.Since(start)
		m.logger.Info("change set materialized",
			"change_set_id", cs.ID, "project_id", cs.ProjectID,
			"outcome", "noop", "duration_ms", dur.Milliseconds(),
			"files", files, "changed", 0)
		return m.markChangeSetProcessed(ctx, cs.ID, nil, runMetrics{
			outcome: "noop", duration: dur, files: files, changed: 0,
		})
	}

	message := fmt.Sprintf("%s\n\nChange-Set: %d", cs.CommitMessage, cs.ID)
	sha, cerr := git.Commit(ctx, message, cs.ActorName, cs.ActorEmail)
	if cerr != nil {
		return fmt.Errorf("git commit: %w", cerr)
	}
	dur := time.Since(start)
	if err := m.markChangeSetProcessed(ctx, cs.ID, &sha, runMetrics{
		outcome: "committed", duration: dur, files: files, changed: changed,
	}); err != nil {
		return err
	}

	m.logger.Info("change set materialized",
		"change_set_id", cs.ID, "project_id", cs.ProjectID,
		"outcome", "committed", "duration_ms", dur.Milliseconds(),
		"files", files, "changed", changed, "commit_sha", sha)
	return nil
}

// Reconcile rebuilds a project's whole tree from current DB and commits any
// drift. It is the backstop for the scoped hot path: a non-empty result means a
// closure gap (logged as a warning) that this call has just healed. Runs off
// the hot path (CLI + nightly sweep).
func (m *Materializer) Reconcile(ctx context.Context, projectID string) (int, error) {
	unlock, err := acquireProjectLock(m.baseDir, projectID)
	if err != nil {
		return 0, fmt.Errorf("acquire project lock %s: %w", projectID, err)
	}
	defer unlock()

	workDir := filepath.Join(m.baseDir, projectID)
	git := newGitRunner(workDir, m.logger)
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		if err := git.Init(ctx); err != nil {
			return 0, fmt.Errorf("git init: %w", err)
		}
	}
	if err := resetWorkTree(workDir); err != nil {
		return 0, fmt.Errorf("reset work tree for %s: %w", projectID, err)
	}
	if err := m.writeProjectTree(ctx, workDir, projectID); err != nil {
		return 0, fmt.Errorf("materialize project tree %s: %w", projectID, err)
	}
	if err := git.AddAll(ctx); err != nil {
		return 0, fmt.Errorf("git add: %w", err)
	}
	if !git.HasStagedChanges(ctx) {
		return 0, nil
	}
	changed := git.StagedFileCount(ctx)
	if _, err := git.Commit(ctx, "reconcile\n\nProject: "+projectID, "pletka-system", "system@pletka.local"); err != nil {
		return 0, fmt.Errorf("git commit: %w", err)
	}
	m.logger.Warn("reconcile healed materialization drift", "project_id", projectID, "changed", changed)
	return changed, nil
}

// projectIDsWithRepos enumerates every project that already has a
// materialized git working tree under m.baseDir (a subdirectory containing
// a .git dir), so the nightly sweep can Reconcile each one.
//
// This deliberately does not enumerate every project row in weave_projects:
// there is no dedicated "projects with a materialized repo" query, and a
// project with no on-disk repo yet has never been through the scoped hot
// path, so it cannot have a closure gap to heal — it gets its first
// materialization the normal way (InitProject / first change set), not by
// being force-initialized here. Filesystem enumeration is also what keeps
// the sweep's cost proportional to what's actually materialized, rather
// than the whole system's project count.
func (m *Materializer) projectIDsWithRepos(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read base dir: %w", err)
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := os.Stat(filepath.Join(m.baseDir, e.Name(), ".git"))
		if err == nil && info.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// resetWorkTree removes every tracked file and directory under workDir
// (except .git) before a full rebuild, so deletions in the DB drop out of
// the tree instead of lingering as stale files that writeProjectTree never
// touches.
func resetWorkTree(workDir string) error {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Errorf("read work dir: %w", err)
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(workDir, e.Name())); err != nil {
			return fmt.Errorf("remove %s: %w", e.Name(), err)
		}
	}
	return nil
}

// runMetrics is the per-run cost persisted on the change set.
type runMetrics struct {
	outcome  string
	duration time.Duration
	files    int
	changed  int
}

// countTreeFiles counts the materialized files under workDir, excluding the
// .git directory. This is the whole-tree write size — the cost of one
// approach-C save — logged alongside duration_ms.
func countTreeFiles(workDir string) int {
	n := 0
	_ = filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		n++
		return nil
	})
	return n
}

// markChangeSetProcessed marks the change set's log rows and the set itself
// processed, recording sha (nil when no commit was made).
func (m *Materializer) markChangeSetProcessed(ctx context.Context, csID int64, sha *string, met runMetrics) error {
	if err := m.queries.WeaveMarkChangeLogProcessed(ctx, csID); err != nil {
		return fmt.Errorf("mark change_log processed: %w", err)
	}
	outcome := met.outcome
	dur := int32(met.duration.Milliseconds())
	files := int32(met.files)
	changed := int32(met.changed)
	if err := m.queries.WeaveMarkChangeSetProcessed(ctx, sqlcgen.WeaveMarkChangeSetProcessedParams{
		ID:                     csID,
		GitCommitSha:           sha,
		MaterializedOutcome:    &outcome,
		MaterializedDurationMs: &dur,
		MaterializedFiles:      &files,
		MaterializedChanged:    &changed,
	}); err != nil {
		return fmt.Errorf("mark change_set processed: %w", err)
	}
	return nil
}
