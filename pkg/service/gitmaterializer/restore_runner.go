package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// HydrateRestorePlan runs the whole restore pipeline for plan's project:
// vendored dependencies, project shell, entities, then overrides and
// provenance, in that order. It takes plan's project's restore lock ONCE,
// as the very first thing it does — before the vendored phase, not just
// before overrides+provenance — and holds it across every phase, releasing
// only once the whole pipeline finishes.
//
// This is the fix for the version of this lock that only wrapped
// overrides+provenance: a busy project failed only after vendored/shell/
// entities had already committed, leaving shell, categories, fields,
// models and collections at snapshot state while overrides, refs,
// adoptions and forks stayed at pre-restore state, with nothing to roll
// either half back. Those earlier phases are safe to run unlocked in
// isolation — HydrateProjectShell/HydrateProjectEntities are upsert-only
// and id-preserving, so they don't lose or orphan anything a concurrent
// save could be mid-write on — but that safety is a property of what they
// do today, not of which tables they touch: the day a "delete entities
// absent from the snapshot" sweep is added (a natural step for restore
// fidelity), that upsert-only property breaks and the seam reopens one
// phase earlier, with dangling rows instead of merely lost ones. Acquiring
// the lock unconditionally, before any phase, means that day doesn't
// require touching this function again.
//
// Once this lock is held, it calls hydrateProjectOverridesAndProvenanceLocked
// directly for plan's own project — NOT the self-locking
// HydrateProjectOverridesAndProvenance — since this function already holds
// that project's lock; see hydrateProjectOverridesAndProvenanceLocked's
// doc comment for why calling the self-locking entry point here would
// self-block.
func (m *Materializer) HydrateRestorePlan(ctx context.Context, plan *RestorePlan) error {
	if plan == nil {
		return fmt.Errorf("hydrate restore plan: missing plan")
	}
	projectID := strings.TrimSpace(plan.ProjectID)
	if projectID == "" {
		return fmt.Errorf("hydrate restore plan: missing project id")
	}

	release, err := m.LockProjectForRestore(ctx, projectID)
	if err != nil {
		return fmt.Errorf("hydrate restore plan: %w", err)
	}
	defer func() {
		if unlockErr := release(); unlockErr != nil {
			m.logger.Warn("hydrate restore plan: release restore lock", "project_id", projectID, "err", unlockErr)
		}
	}()

	if err := m.HydrateVendored(ctx, plan); err != nil {
		return err
	}
	if err := m.HydrateProjectShell(ctx, plan); err != nil {
		return err
	}
	if err := m.HydrateProjectEntities(ctx, plan); err != nil {
		return err
	}
	if err := m.hydrateProjectOverridesAndProvenanceLocked(ctx, plan); err != nil {
		return err
	}
	return nil
}

// HydrateVendored materializes every vendored ontology and vendored parent
// project referenced by plan's snapshot. It is the single ordering source
// for the vendored phase of a restore — both HydrateRestorePlan (used by the
// CLI and end-to-end tests) and pkg/weave/gitrestoreadmin's HTTP-driven
// restore runner call this method so the two entry points can never drift
// on when vendored dependencies are hydrated relative to the project shell.
// It must run before HydrateProjectShell: the shell hydration links the
// project to ontology versions and parent projects that must already exist.
func (m *Materializer) HydrateVendored(ctx context.Context, plan *RestorePlan) error {
	if plan == nil {
		return fmt.Errorf("hydrate vendored: missing plan")
	}
	if err := m.hydrateVendoredOntologies(ctx, plan); err != nil {
		return err
	}
	if err := m.hydrateVendoredProjects(ctx, plan); err != nil {
		return err
	}
	return nil
}

func PrepareRestorePlan(rootDir, targetProjectID string) (*RestorePlan, error) {
	snapshot, err := LoadProjectSnapshot(rootDir)
	if err != nil {
		if strings.TrimSpace(targetProjectID) == "" {
			return nil, fmt.Errorf("load snapshot: %w", err)
		}
		return nil, fmt.Errorf("load snapshot for %s: %w", targetProjectID, err)
	}
	if strings.TrimSpace(targetProjectID) != "" {
		snapshot, err = retargetProjectSnapshot(snapshot, targetProjectID)
		if err != nil {
			return nil, fmt.Errorf("retarget snapshot: %w", err)
		}
	}
	return BuildRestorePlan(snapshot), nil
}

func (m *Materializer) HydrateProjectSnapshot(ctx context.Context, rootDir string) (*RestorePlan, error) {
	plan, err := PrepareRestorePlan(rootDir, "")
	if err != nil {
		return nil, fmt.Errorf("hydrate project snapshot: %w", err)
	}
	if err := m.HydrateRestorePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("hydrate project snapshot: hydrate restore plan: %w", err)
	}
	return plan, nil
}

func (m *Materializer) HydrateProjectSnapshotAs(ctx context.Context, rootDir, targetProjectID string) (*RestorePlan, error) {
	plan, err := PrepareRestorePlan(rootDir, targetProjectID)
	if err != nil {
		return nil, fmt.Errorf("hydrate project snapshot as %s: %w", targetProjectID, err)
	}
	if err := m.HydrateRestorePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("hydrate project snapshot as %s: hydrate restore plan: %w", targetProjectID, err)
	}
	return plan, nil
}

func retargetProjectSnapshot(snapshot *ProjectSnapshot, targetProjectID string) (*ProjectSnapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("missing snapshot")
	}
	targetProjectID = strings.TrimSpace(targetProjectID)
	if targetProjectID == "" {
		return nil, fmt.Errorf("missing target project id")
	}
	if snapshot.Manifest.Project.ID == targetProjectID {
		return snapshot, nil
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot clone: %w", err)
	}
	var cloned ProjectSnapshot
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot clone: %w", err)
	}

	sourceProjectID := strings.TrimSpace(cloned.Manifest.Project.ID)
	cloned.Manifest.Project.ID = targetProjectID
	if cloned.Mod != nil {
		cloned.Mod.Module.ProjectID = targetProjectID
		if sourceProjectID != "" {
			suffix := "/projects/" + sourceProjectID
			if strings.HasSuffix(cloned.Mod.Module.Path, suffix) {
				cloned.Mod.Module.Path = strings.TrimSuffix(cloned.Mod.Module.Path, suffix) + "/projects/" + targetProjectID
			}
		}
	}
	if cloned.Adoptions != nil {
		for path, receipt := range cloned.Adoptions.Receipts {
			for i := range receipt.Adoption.Contexts {
				ctx := &receipt.Adoption.Contexts[i]
				if strings.EqualFold(strings.TrimSpace(ctx.ContextEntityType), "project") && strings.TrimSpace(ctx.ContextEntityID) == sourceProjectID {
					ctx.ContextEntityID = targetProjectID
				}
			}
			cloned.Adoptions.Receipts[path] = receipt
		}
	}
	return &cloned, nil
}
