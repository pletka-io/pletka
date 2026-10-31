# Complete project and entity fragment values

`pkg/projectsnapshot` is the database-free value boundary for the whole-project
assembler in [ADR-0012](../decisions/0012-whole-project-reads-belong-to-an-assembler.md).
Its construction, field coverage and complete-project database assembly through
`pkg/service/projectread` are implemented, including bounded entity loading and
pure contextual composition. The [portable codec](project-bundle-format.md) now
encodes and decodes all captured values; storage restore, Git writer migration
and generator wiring follow separately. Existing export DTOs do not become complete by wrapping them.

## Construction

Assembly infrastructure constructs `CompleteProject`. The conformance guard
allows `NewComplete` and `NewFragment` in this package and `pkg/service/projectread`,
with test-only fixtures exempt. The pure `projectbundle/decode.go` also calls
`NewComplete` to validate imported values, returned as `ImportedProject`; it
cannot create a locally certified release. Consumers receive the finished value. Its zero value is invalid;
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

Each section names its implemented bound loader and portable codec. Storage
restoration remains a separate S4 gate. The [assembly bindings](../../pkg/service/projectread/coverage.go)
register all 22 sections, including tables, owning slice, mapper files and queries.
Eleven sections use collection, override, vocabulary and example readers. Eleven
use temporary session adapters with named replacement APIs and S6 removal tasks.
Converted slices cannot be downgraded to temporary adapters. The
[assembly conformance test](../../pkg/conformance/project_assembly_test.go) checks
actual SQL projections and typed mapper fields against the independent coverage
contract. Portable encoding must independently account for each retained field
and its restore behavior before a full-project tree claims completeness.

Records retain raw authored JSON bytes, import provenance, statuses, array order,
raw paths alongside structured paths, nullable override settings and all repeated
override/placement identities. SQL null is distinct from JSON `null` bytes.
`Decimal` preserves exact numeric text without float64 conversion. `Timestamp`
and `Date` preserve text and infinities. Database loaders emit finite timestamps
as UTC `YYYY-MM-DDTHH:MM:SS.ffffffZ` and dates as `YYYY-MM-DD`, with a ` BC` suffix
for years before the common era. These projections do not depend on session
timezone or `DateStyle`. These scalar types do not infer or repair historical defaults.
The private JSON/base64 buffer freeze is an implementation detail, not a wire
format or a replacement for canonical node serialization.

Input pointers/bytes and returned content are detached. Project and fragment
identity lists have stable ordering. Row arrays preserve the supplied order;
loaders must select deterministic order explicitly. No constructor sorts away
repeated placements or chooses a global winning override.

## Bound database assembly

Construct `projectread.Assembler` once with `New(DefaultOptions())`, adjusting
limits and draft module hosts at wiring time. Within the boundary's
`readsession.Factory` callback, call `assembler.Project(ctx, session)`. The caller
authorizes the root; the factory's explicit source policy authorizes the entire
captured source closure. The assembler accepts no independent project or scope,
pool, transaction factory or arbitrary query capability. Every section, source,
descriptor and ontology input is loaded sequentially on the supplied read-only
repeatable-read transaction. The returned value remains usable after it closes.

Each query has deterministic bounded paging. Defaults are 256 rows per page,
1,000,000 rows, 256 MiB of encoded payload and 256 projects including the root.
Limits must be positive; pages may contain at most 1024 rows. The byte budget
counts encoded row pages and ontology artifact/reference bytes, not descriptors
or peak process memory. It is checked as inputs arrive; it does not prevent a
single large database value from being fetched. Exceeding a limit returns a
`BudgetError` and no partial result. Query, authorization, cancellation
and validation failures also return no partial result.

Draft assembly and release capture share the read-only ontology selection planner
in `pkg/database/projectontology`. Explicit selections, defaults and inherited
published closures must agree. Draft assembly requires verified published
ontology evidence, normalizes adoption source pins and builds descriptors on the
same session. Released assembly dispatches explicitly on the stored content
contract: verified evidence for contract 1, exact legacy evidence for contract 2.
It never recovers a failed verified read by trying legacy or live data.

Required historical evidence that is absent remains `not_in_release`. In
particular, migration 040 adds ontology-link authors to future captured actor
inventories without rewriting historical descriptor documents. A complete read
cannot obtain a missing historical author from today's actor table.

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

`CompleteProject.Entity(kind,id)` derives a closed fragment from an already
complete value. `projectread.Assembler.Entity(ctx, session, kind, id)` loads a
bounded structural closure directly through the same session. It never calls
`Project` and filters afterward. Fragments retain required bodies and per-project interpretation context, including
consumer base overrides for selected foreign fields, empty placements and repeated
contextual overrides. They may contain a superset of the minimal graph. Their
ontology evidence retains its original complete context; that does not make their
row sections complete or permit reconstruction with a complete receipt.

The bound loader supplies explicit entity selections to the existing lossless
paged readers. SQL filters entity bodies and their child rows, then the assembler
expands required refs and consumer base overrides until the closure stops growing.
Every structural query and its selector mapping are checked by conformance.
Project metadata, descriptors, namespaces, ontology evidence and authorized source
inventories remain complete; this is bounded structural loading, not a claim that
interpretation artifacts are entity-sized. Closure rounds may reread an expanded
subset. Row/byte budgets count that query work, including repeated pages, and
reject excessive work rather than truncate it. Dependency authorization still
covers the whole recorded source closure.

`NewFragment` validates loaded subsets and required references independently of
complete receipt row counts. It checks every retained semantic location for each
loaded entity against the original evidence without claiming unrelated entities
were loaded. `NewComplete` continues to require exact complete counts. Project-level
adoption receipts attach only to selected entities; loading metadata cannot pull
every unrelated adoption into a fragment. Stale saved-value exceptions stay the
same as for complete assembly. Draft defaults selection uses the entire project's
semantic inventory, even for an entity containing no references.

## Pure contextual composition

`EntityFragment.Compose()` builds a detached `ContextView` for a model, collection
or field in the requesting consumer's context. `ComposeContext` explicitly selects
another retained context and entity within that fragment. An empty consumer
context does not silently fall back to source-owned overrides.

Each `ContextField` retains the complete source body, original override and
effective settings separately. Every copied override survives, including repeated
occurrences; false, zero, empty and null settings do not become inheritance rules.
As in the existing resolver, expected value type comes from the field body. A
consumer base fixed value or fixed vocabulary entry locks both fixed-value fields;
`FixedBy` records that commitment without rewriting the original override. Groups
retain empty and hidden placements with stable category/collection/field ordering.

Refs resolve required bodies, labels and sealed-list flags from their captured
source contexts. Closed-list choices retain ordered memberships, custom labels,
exact vocabulary entries and the list owner's captured concept namespace. Missing
required bodies are errors, not empty constraints. Returned buffers may be changed
by a consumer without affecting later compositions or the underlying fragment.

This is a value composition boundary, not the existing HTTP response or Git wire
format. UI statistics/shared-prefix presentation, renderer-specific path grafting
and generator migration remain consumer work. No global anchor discovery is part
of composition; later renderers must stay inside the captured graph.
Production assembler integration tests prove actual-cut
field parity, same-session consistency during edits, concrete transitive source
isolation, authorization, legacy/published evidence separation, bounded/complete
fragment parity and detached composition after closing the database pool.
The older PostgreSQL value fixture remains a separate constructor compatibility
check. Database-backed bundle tests now compare every captured row and its
interpretation evidence after portable round-tripping, including published and
legacy source graphs. Storage restoration and consumer migrations remain pending.
