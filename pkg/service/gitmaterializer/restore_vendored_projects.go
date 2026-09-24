package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Investigation (Task 5, Step 1): does a vendored project snapshot carry the
// full entity tree and its own nested Vendor set?
//
// Full entity tree: yes. loadProjectSnapshot (snapshot_loader.go) always
// calls scanProjectEntities(rootDir) unconditionally — including for
// vendored projects, which are loaded via loadVendorSnapshots ->
// loadProjectSnapshot(entry.root, false) (snapshot_loader.go:282). A
// vendored project directory is written by writeProjectTree
// (vendor_snapshot.go:104/108), the exact same function used for the
// top-level project — it writes categories, fields, models, collections,
// scoped overrides, and adopted-entity markers. So vendored project
// snapshots are NOT manifest-only; this task is not BLOCKED.
//
// Nested Vendor set: never populated on the loaded struct, and never
// produced as a nested vendor/ subtree on disk either:
//   - Write side: vendorDependenciesForProject (vendor_snapshot.go:78)
//     recurses with the SAME rootDir it was first called with — see the
//     recursive call at vendor_snapshot.go:121, which passes rootDir
//     unchanged. Every project and ontology dependency, at any depth, is
//     written as a sibling under the single top-level rootDir/vendor/projects/
//     and rootDir/vendor/ontologies/ trees, never nested under
//     vendor/projects/<module>/vendor/. Grepping vendorDependenciesForProject
//     confirms no code path constructs a nested vendor directory.
//   - Read side: loadVendorSnapshots (snapshot_loader.go:274) loads each
//     vendored project with loadProjectSnapshot(entry.root, false) —
//     includeVendor is hard-coded false, so VendoredProjectSnapshot.Snapshot.Vendor
//     is always the zero value.
//
// Consequence: recursion in hydrateVendoredProjects over a vendored
// project's own Vendor.Projects can never find further nested vendor
// dependencies in the current snapshot format. The depth cap below is
// therefore defensive only — it guards against a future change to the
// snapshot shape, not a case reachable today. Likewise, a vendored parent's
// own vendored ontologies are already covered by hydrateVendoredOntologies
// running once against the top-level snapshot (whose Vendor.Ontologies is
// the flattened, transitive closure of every project in the tree) — this
// file's call into hydrateVendoredOntologies on the parent's sub-plan is a
// no-op today for the same reason, kept only so the pipeline stays
// symmetric if the snapshot shape ever changes to support real nesting.
const maxVendoredProjectDepth = 3

// hydrateVendoredProjects restores every vendored parent project in plan's
// snapshot that is not already present in weave_projects, before the
// top-level project's own shell/entities/overrides are hydrated. This must
// run after hydrateVendoredOntologies (which imports every ontology in the
// whole tree, including those required by vendored parents) and before
// HydrateProjectShell (which writes weave_project_inheritance rows that
// require the parent project to already exist).
//
// Restoring a vendored parent is idempotent: a parent already present in
// weave_projects is skipped entirely (its own entities/overrides are not
// re-hydrated by this step — a present parent is assumed to already be
// correct, matching the same assumption made for vendored ontologies).
func (m *Materializer) hydrateVendoredProjects(ctx context.Context, plan *RestorePlan) error {
	// lockedProjectID is whichever project's restore lock the top of this
	// pipeline already holds (HydrateRestorePlan, or gitrestoreadmin's job
	// runner) — always plan.ProjectID at depth 0, since this is only ever
	// reached from HydrateVendored, which only ever receives the
	// TOP-level plan. Threaded down unchanged through recursion so the
	// depth-2+ guard below still compares against the ORIGINAL target, not
	// whichever vendored parent a deeper recursion level happens to be
	// processing.
	lockedProjectID := strings.TrimSpace(plan.ProjectID)
	return m.hydrateVendoredProjectsAtDepth(ctx, plan, 0, lockedProjectID)
}

func (m *Materializer) hydrateVendoredProjectsAtDepth(ctx context.Context, plan *RestorePlan, depth int, lockedProjectID string) error {
	if plan == nil || plan.Snapshot == nil {
		return nil
	}
	deps := plan.Snapshot.Vendor.Projects
	if len(deps) == 0 {
		return nil
	}
	if depth >= maxVendoredProjectDepth {
		return fmt.Errorf("hydrate vendored projects: exceeded max recursion depth %d at %s", maxVendoredProjectDepth, plan.ProjectID)
	}

	// Collect the vendored parents not already present, keyed by project id.
	// deps arrives sorted by module path — an arbitrary order relative to
	// inheritance (DHI sorts before its own parent LA).
	pendingIdx := map[string]int{}
	var pendingIDs []string
	for i, dep := range deps {
		if dep.Snapshot == nil {
			continue
		}
		projectID := strings.TrimSpace(dep.Snapshot.Manifest.Project.ID)
		if projectID == "" {
			return fmt.Errorf("hydrate vendored projects: %s@%s missing project id", dep.Module, dep.Version)
		}
		if _, err := m.queries.WeaveGetProjectByID(ctx, projectID); err == nil {
			continue // already present: skip, idempotent
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("hydrate vendored projects: get project %s: %w", projectID, err)
		}
		pendingIdx[projectID] = i
		pendingIDs = append(pendingIDs, projectID)
	}

	ordered, err := topoOrderVendoredProjects(deps, pendingIdx, pendingIDs)
	if err != nil {
		return err
	}

	// Pass 1: shell + entities for every pending parent, in dependency order.
	// Overrides are deferred so every vendored field row exists before any
	// override resolves — a parent's override may reference another vendored
	// project's field (SRD -> LAF.10) regardless of inheritance order.
	subPlans := make(map[string]*RestorePlan, len(ordered))
	for _, projectID := range ordered {
		dep := deps[pendingIdx[projectID]]
		subPlan := BuildRestorePlan(dep.Snapshot)
		if err := m.hydrateVendoredProjectsAtDepth(ctx, subPlan, depth+1, lockedProjectID); err != nil {
			return err
		}
		if err := m.hydrateVendoredOntologies(ctx, subPlan); err != nil {
			return fmt.Errorf("hydrate vendored projects: hydrate ontologies for %s: %w", projectID, err)
		}
		if err := m.HydrateProjectShell(ctx, subPlan); err != nil {
			return fmt.Errorf("hydrate vendored projects: hydrate shell for %s: %w", projectID, err)
		}
		if err := m.HydrateProjectEntities(ctx, subPlan); err != nil {
			return fmt.Errorf("hydrate vendored projects: hydrate entities for %s: %w", projectID, err)
		}
		subPlans[projectID] = subPlan
	}

	// Pass 2: overrides + provenance, after all vendored entities exist.
	for _, projectID := range ordered {
		if projectID == lockedProjectID {
			// Guard: a vendored dependency's project id coincides with the
			// project whose restore lock the top of this pipeline already
			// holds. Not reachable in practice today — a project cannot
			// legitimately vendor itself as its own dependency — but the
			// id space isn't otherwise enforced to exclude it, and calling
			// the self-locking HydrateProjectOverridesAndProvenance here
			// would try to re-acquire the SAME exclusive key the caller
			// already holds, self-blocking for the full
			// restoreProjectLockTimeout before failing. Use the
			// already-locked variant instead, exactly as the top-level
			// caller does for the target project itself.
			if err := m.hydrateProjectOverridesAndProvenanceLocked(ctx, subPlans[projectID]); err != nil {
				return fmt.Errorf("hydrate vendored projects: hydrate overrides for %s: %w", projectID, err)
			}
			continue
		}
		if err := m.HydrateProjectOverridesAndProvenance(ctx, subPlans[projectID]); err != nil {
			return fmt.Errorf("hydrate vendored projects: hydrate overrides for %s: %w", projectID, err)
		}
	}

	return nil
}

// topoOrderVendoredProjects orders the pending vendored project ids so a
// project's own vendored parent hydrates first — HydrateProjectShell writes
// weave_project_inheritance rows that require the parent project row to
// exist. Parents outside the pending set (already in the database) impose no
// ordering. pendingIdx maps a pending project id to its index in deps;
// pendingIDs preserves deps order as the deterministic traversal seed.
func topoOrderVendoredProjects(deps []VendoredProjectSnapshot, pendingIdx map[string]int, pendingIDs []string) ([]string, error) {
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	ordered := make([]string, 0, len(pendingIDs))
	var visit func(projectID string) error
	visit = func(projectID string) error {
		idx, pending := pendingIdx[projectID]
		if !pending || state[projectID] == done {
			return nil
		}
		if state[projectID] == visiting {
			return fmt.Errorf("hydrate vendored projects: inheritance cycle involving %s", projectID)
		}
		state[projectID] = visiting
		if inh := deps[idx].Snapshot.Manifest.Inheritance; inh != nil {
			for _, parent := range inh.Parents {
				if err := visit(strings.TrimSpace(parent.ProjectID)); err != nil {
					return err
				}
			}
		}
		state[projectID] = done
		ordered = append(ordered, projectID)
		return nil
	}
	for _, projectID := range pendingIDs {
		if err := visit(projectID); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
