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
// flow and calls into override.Service for the heavy lifting. The
// pattern editor's actual save route (PUT
// /projects/{pid}/{models|collections}/{id}/overrides) is owned by
// pkg/weave/project's saveOverrides, not by model/collection's own
// same-path handlers, which take neither a lock nor a fingerprint and
// exist only because model/collection also expose a flat override CRUD
// surface — see pkg/weave/project/override_write.go for how the router
// resolves the two to the same path.
//
// Save flow (driven by pkg/weave/project's saveOverrides):
//
//  1. The handler authorises the actor and decodes the payload, the
//     commit message, and the fingerprint the editor's last load or save
//     returned.
//  2. Everything from the fingerprint check through the adoption sync
//     runs inside WithEntityLock — a session-level Postgres advisory
//     lock keyed on (entityType, entityID) — so two saves of the same
//     pattern queue instead of racing.
//  3. A non-empty fingerprint is compared against EntityFingerprint's
//     current value; a mismatch refuses the save with 409 before
//     anything is written, so a stale draft loses cleanly instead of
//     silently overwriting a newer save. An empty fingerprint skips the
//     check (ops tooling, git restore, and forks never loaded an editor
//     payload to carry one).
//  4. Service.SaveForEntity loads the existing rows, matches desired rows
//     against them the same way Store.ReplaceForEntity will, and stamps
//     the matched ids onto desired BEFORE computing the Diff — a kept
//     row resent without its id (a raw client payload, or an ops/MCP/CLI
//     writer) must diff as Changed, not Removed+Added. Only then does it
//     run Store.ReplaceForEntity inside ChangeLogRunner.Run and record
//     one ChangeLogEntry per Added/Removed/Changed override.
//  5. The handler recomputes EntityFingerprint once more and returns it,
//     so the editor's next save round-trips the value the save just
//     produced.
//
// Presence — who else is editing right now — is a separate, deliberately
// unlocked concern handled by presence.go's Presence type: an in-memory,
// notice-only registry (Beat/Leave/Others) that blocks nothing and takes
// no lock on the entity. A client heartbeats while the editor stays open,
// and the UI surfaces "Alice is also editing this" as information, not
// enforcement — the owner declined a soft edit-lock (locked-by banner,
// heartbeat takeover, 409 on lock expiry) in favour of this notify-only
// design. The save path's correctness comes entirely from the advisory
// lock and the fingerprint check above; presence plays no part in it.
package override
