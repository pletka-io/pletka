// Package override implements the field-override vertical slice. This
// package is a full slice — it owns the overrides/junction tables via its
// own store and service — and is also the one named shared infrastructure
// slice: peer entity slices (field, model, collection, project) import its
// Store type directly instead of through a reader interface (see
// ADR-0006).
//
// Override is an internal data layer. It does not register HTTP routes
// of its own — mutations always arrive through a parent entity (model,
// collection, or field), so the owning slice drives the user-facing
// flow and calls into override.Service for the heavy lifting.
//
// Save flow (called from a parent slice handler):
//
//  1. Parent slice receives PUT /projects/{pid}/{models|collections}/{id}/overrides
//  2. Parent decodes payload + commit message, authorises the actor.
//  3. Parent calls override.Service.SaveForEntity(ctx, entityType, entityID,
//     desired []FieldOverride, commitMessage string).
//  4. Service loads the current set, computes a Diff, runs Store.ReplaceForEntity
//     inside ChangeLogRunner.Run, records one ChangeLogEntry per Added/Removed/
//     Changed override.
//
// ----------------------------------------------------------------------
// TODO(override-editor / svelte): when the frontend override editor lands
// it MUST persist its working state in browser local storage so that an
// in-progress edit survives a page reload, accidental tab close, or
// browser crash. Without this, a user who has spent fifteen minutes
// reordering and tweaking overrides will lose everything on a stray
// reload. Key the local-storage entry by (projectID, entityType,
// entityID, actorID) so multiple drafts coexist. Clear the entry only
// after a successful save (or explicit "discard draft").
//
// TODO(override-editor / edit-lock): overrides are a shared editing
// surface — two users editing the same model/collection at once would
// silently clobber each other's work via the diff path. Before this
// goes to production we need an edit-lock gatekeeper:
//   - When an actor opens the editor, server records a soft lock
//     (entityType, entityID, actorID, expires_at).
//   - Other actors loading the editor see "Locked by Alice — read only".
//   - Lock auto-expires after N minutes of inactivity; client sends
//     heartbeats while editing.
//   - Save path verifies the actor still holds the lock and rejects
//     with 409 if it has expired or been taken over.
//
// Implementation is deferred until the model + collection slices land
// and we know exactly which routes mount the editor. Track in the
// versioning / overrides design doc when added.
// ----------------------------------------------------------------------
package override
