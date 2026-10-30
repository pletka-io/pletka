# Complete project and entity fragment values

`pkg/projectsnapshot` is the database-free value boundary for the whole-project
assembler in [ADR-0012](../decisions/0012-whole-project-reads-belong-to-an-assembler.md).
Its construction and field coverage are implemented. Production database assembly,
pure contextual view composition, Git encoding/restore and generator wiring still
follow separately. Existing export DTOs do not become complete by wrapping them.

## Construction

Only assembly infrastructure constructs `CompleteProject`. The conformance guard
allows `NewComplete` in this package and `pkg/service/projectread`, with test-only
fixtures exempt. Consumers receive the finished value. Its zero value is invalid;
`EntityFragment` is a different type and cannot satisfy a complete-project API.
These are value contracts, not an authentication or database provenance mechanism.
The bound assembler must authorize sources and load every input on one session.

A `ProjectInput` supplies typed `Content`, interpretation `Evidence`, counts,
selected source identities and direct foreign membership. Every section and both
dependency inventories must be explicitly loaded: nil is missing, an empty slice
is loaded-empty. All 22 section counts are checked; a release also requires its
verified or legacy receipt. Exactly one owned project row, full composite row
keys, ownership/version, descriptors, required structural references and semantic
coverage are checked before construction succeeds. Counts alone cannot detect
substituted row bytes: the production loader must read the certified immutable
archives, not fabricate inputs or use receipt counts as a field hash.

`Draft(project)` is explicit; an empty release version never selects draft.
Sources always have concrete release identities. Construction rejects conflicting
versions, cycles, missing transitive source inventories, absent foreign bodies,
unknown membership and unreachable extra projects. Every source retains its own
namespace and ontology context. Legacy/verified capture contracts remain distinct;
legacy ambiguity is preserved, not promoted by a primary flag or a namespace map.

## Retained records and coverage

The [coverage registry](../../pkg/projectsnapshot/coverage.go) maps every reviewed
archive column to its typed record field, under the same column name. The
[conformance test](../../pkg/conformance/project_values_test.go) compares that
mapping and nullability/types with the independent
[capture ledger](../../pkg/weave/release/testdata/capture_inventory.json) and
schema model. All 272 current archive columns are retained; there are no additional
value-level exclusions. JSON validation covers every byte column, checked by the
same ratchet. Do not mechanically regenerate the capture ledger from these values.

The registry explicitly marks each section's loader as pending S3 and portable
encoding/restore as pending S4. Those are open gates, not implemented adapters.
When introducing a loader, replace its pending disposition with the actual bound
capability and field mapping. Register transitional adapters with tables, owning
slice, replacement API and removal phase. Converted slices retain their existing
Reader boundary. Portable encoding must independently account for each retained
field and its restore behavior before a full-project tree claims completeness.

Records retain raw authored JSON bytes, import provenance, statuses, array order,
raw paths alongside structured paths, nullable override settings and all repeated
override/placement identities. SQL null is distinct from JSON `null` bytes.
`Decimal` preserves exact numeric text without float64 conversion. `Timestamp`
and `Date` preserve text and infinities; loaders must use a fixed UTC timestamp
representation. These scalar types do not infer or repair historical defaults.
The private JSON/base64 buffer freeze is an implementation detail, not a wire
format or a replacement for canonical node serialization.

Input pointers/bytes and returned content are detached. Project and fragment
identity lists have stable ordering. Row arrays preserve the supplied order;
loaders must select deterministic order explicitly. No constructor sorts away
repeated placements or chooses a global winning override.

## Closure and interpretation

Required refs use owned rows or direct captured membership in a selected source.
Concept membership/hierarchy, fixed entries, collection placements, linked
examples and override targets require bodies. Intentionally stale saved-value
field/placement refs remain evidence; their optional linked example still requires
a body. Fork provenance and project parent metadata are retained without inventing
extra dependency edges. Actor and module identities use captured descriptors. Source module identities
must match their source context, and shared concrete ontology versions must agree
on body, exact publication bytes and module across contexts.

Published interpretation checks every authored scope, primary/subfield path,
additional type and datatype against its retained resolution and sealed body.
`ontologyevidence.Assessor` decodes an explicit inventory once for a batch; it has
no latest-version or live-lookup capability. Legacy content checks its exact
original elements, aliases, selected bodies/primary flags and source membership
against the retained artifact while preserving unresolved assessments.

`CompleteProject.Entity(kind,id)` currently derives a closed fragment from an
already complete value. It is not the future bounded database assembly path.
Fragments retain required bodies and per-project interpretation context, including
consumer base overrides for selected foreign fields, empty placements and repeated
contextual overrides. They may contain a superset of the minimal graph. Their
ontology evidence retains its original complete context; that does not make their
row sections complete or permit reconstruction with a complete receipt.

Pure contextual view composition is still required: these records preserve every
source body and consumer override, but do not apply display precedence or replace
the existing resolver. Likewise the same-session concurrency guarantee belongs to
S3. The PostgreSQL value fixture proves compatibility with real captured rows; its
test-only adapter does not certify production assembly.
