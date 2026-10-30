# Complete project and entity fragment values

`pkg/projectsnapshot` is the database-free value boundary for the whole-project
assembler in [ADR-0012](../decisions/0012-whole-project-reads-belong-to-an-assembler.md).
Its construction, field coverage and complete-project database assembly through
`pkg/service/projectread` are implemented. Pure contextual view composition,
bounded entity loading, Git encoding/restore and generator wiring follow
separately. Existing export DTOs do not become complete by wrapping them.

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

Each section names its implemented bound loader; portable encoding/restore remains
pending. The [assembly bindings](../../pkg/service/projectread/coverage.go)
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
`BudgetError` and no partial complete value. Query, authorization, cancellation
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

`CompleteProject.Entity(kind,id)` currently derives a closed fragment from an
already complete value. It is not the future bounded database assembly path.
Fragments retain required bodies and per-project interpretation context, including
consumer base overrides for selected foreign fields, empty placements and repeated
contextual overrides. They may contain a superset of the minimal graph. Their
ontology evidence retains its original complete context; that does not make their
row sections complete or permit reconstruction with a complete receipt.

Pure contextual view composition is still required: these records preserve every
source body and consumer override, but do not apply display precedence or replace
the existing resolver. Production assembler integration tests now prove actual-cut
field parity, same-session consistency during edits, concrete transitive source
isolation, authorization, legacy/published evidence separation and detached output.
The older PostgreSQL value fixture remains a separate constructor compatibility
check; consumer migrations and portable round-tripping still need their own proof.
