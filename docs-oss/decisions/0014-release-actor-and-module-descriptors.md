# ADR-0014: Capture actor and module descriptors with a release

Date: 2026-10-27
Status: Accepted; implemented for release capture and Git materialization.

## Context

An archived project body and pinned ontology artifacts do not freeze all output.
Project manifests, adoption receipts and fork receipts previously looked up actor
names and slugs in current tables. Project module paths used current actor types
and slugs, while both project and ontology paths used the materializer's current
host configuration. Changing those inputs could change an old release tree.

## Decision

The release transaction captures a format-versioned descriptor document after
archiving bodies and resolving project/ontology pins. It records:

- Actor ID, type, slug and display name for the archived owner/creator, release
  author, adoption/fork authors, ontology-link authors, project attributions and
  project actor links.
- The owning project's concrete module identifier and those of every captured
  source project, taken from each source release's own descriptors.
- Concrete ontology module identifiers for the pinned ontology closure. Source
  projects' existing ontology identifiers take precedence over the current host
  configuration. Conflicting source identifiers for one ontology version reject
  the cut; a single vendor artifact cannot have two identities in this format.

App and CLI supply release creation with the same host configuration used for
draft materialization. Defaults and naming rules are shared. The release service
owns one repeatable-read transaction; descriptors, bodies, pins and outbox commit
together. A missing required actor or uncaptured source rejects the whole cut.

Migration 029 stores documents in `weave_release_descriptors`. A single
`weave_release_actor_references` view supplies the capture inventory and the
storage validation of required actor values. Storage validates captured source
identifiers and ontology coverage, rejects updates, direct deletion and truncation,
and permits deletion through release ownership. Capture must precede finalized
archive ownership. There are no foreign keys to mutable actor rows.

`readsession.Session.Descriptors` exposes a detached immutable value through the
archive SELECT capability. It is bound to the session's project and release;
draft/closed sessions cannot use it. Values expose copies and lookups only within
the captured inventory. The snapshot contains no database handle or current policy.

Released Git writers use those values. Shared receipt encoders receive an actor
resolver explicitly; released callers supply the captured resolver. Conformance
tests prohibit the released identity writers from calling live actor/host helpers.
Pinned source archive existence is still checked independently of module identity.

## Historical releases and limits

Existing releases remain uncaptured. Reading their descriptors returns the
canonical `not_in_release` error; materialization does not infer old names or hosts
from current state. Retained Git artifacts remain historical evidence. There is
no automatic backfill, recertification or rewrite of existing tags. Downgrade
refuses to discard stored descriptor snapshots.

Migration 040 extends the shared actor-reference view with ontology-link
`added_by_id`, which complete-project assembly needs to interpret its retained
link rows. New cuts capture those authors. Upgrading leaves historical descriptor
bytes unchanged; when an older document lacks a required link author, complete
assembly returns `not_in_release` rather than reading current actor data.

This supplies identity inputs for ADR-0012's assembler. It does not complete that
assembler, migrate every project-page reader, add credit serialization where it
was absent, or inventory ontology QName/URI references without explicit version
selectors. Those remain separate coverage work. The releases index retains its
existing policy: current when building a new tree, frozen from the tag when
verifying an existing tag.

## Verification

Integration tests compare complete parent/child release trees and load the
serialized snapshot after actor and host changes, including receipts, ontology
vendors and sums. They also cover transaction snapshot consistency, rollback,
missing historical descriptors, storage guards and downgrade refusal.
Assembler regressions also cover new ontology-link author capture and unchanged
historical documents across the migration 040 upgrade.
