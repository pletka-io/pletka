# Measuring retained read performance

Correct selection and bounded assembly do not establish acceptable latency or
memory use. Measure the [browser detail boundary](browser-detail-reads.md),
project summaries/inventories, generator fragments and complete exports separately.
Use a disposable realistic database copy with current migrations and planner
statistics. Include small, median and large **selected dependency graphs**: a
small owner can inherit most of its export work from another release.

Record successful calls and semantic failures separately. An ambiguous legacy
reference must still fail; timing that error does not qualify a successful
generator. Test draft, verified release and retained legacy selections separately.
Never obtain a successful sample by consulting current data for a missing pin.

Useful measurements include complete-call latency, query count, returned rows,
allocations, process peak RSS, serialized bytes and browser latency while bulk
exports share the pool. Returned rows are not database rows scanned. Use
`EXPLAIN (ANALYZE, BUFFERS)` on representative queries to distinguish server work
from transport, validation and copying. Exercise prepared generic plans as well
as one-off literal queries; large parameter arrays can have very different costs.

Record the connection topology as well. Container port forwarding can dominate
client query time even when the server plan is cheap. Compare the same binary,
query parameters and database over the forwarded path and a direct connection
inside the container network before attributing that gap to SQL execution. Keep
these as separate environment measurements; a faster connection is not an
application optimization. Direct loopback also does not represent deployment
network latency. A pgx query trace includes socket waits and result decoding.

Large text results have a separate memory cost: the driver buffers a PostgreSQL
row, then scanning into a retained string copies it. A one-row result can therefore
be expensive. Smaller row-count pages do not split one large ontology document.
Borrowed driver buffers cannot back an immutable value after the next read. Exact
artifact bytes and independent lifetime must survive any transport change. Evaluate
transport changes for byte identity, bounded buffering, cancellation, the existing
transaction and generated-query enforcement. They do not remove the memory needed
for the retained document and its decoded values.

Retained legacy artifacts now use a single named archive query that emits byte
chunks of at most 256 KiB. A private reader copies each borrowed chunk into an
owned string builder before advancing the driver. It checks offsets, lengths,
consistent headers, completion and cancellation, and never returns partial
evidence. The session then performs the existing exact-byte digest, identity,
body, assessment and closure validation. Missing evidence never selects another
query or current data.

The query retains the original toasted text, converts it to UTF-8 once on the
usual UTF-8 database path, and sorts integer offsets before projecting chunks.
Keep the projection barrier and outer ordering: sorting the projected byte
payloads can spill the entire artifact to temporary files at ordinary `work_mem`.
Byte counts also account for conversion on other database encodings. Check real
generic/custom plans after query changes. Statement count remains one per
artifact; protocol returned-row counts increase because one artifact spans
multiple rows. Structural counts and encoded evidence budgets are unchanged.

Collecting every chunk with a generated `:many` method would retain an additional
full copy. The guarded private adapter therefore uses sqlc's exported query and
types with a streaming scan. Complete artifacts and decoded values are still
retained; bounded protocol rows do not bound total request memory or RSS.

`projectread.Limits.MaxBytes` counts encoded rows and ontology evidence. It is
**not a bound on heap allocation or process RSS**. Immutable value construction,
validation, defensive copies and output preparation also consume memory. Complete
CSV and pack generation buffer output before delivery. Test concurrent operations
before increasing request limits or enabling broad export/capture workloads.

Retained legacy evidence validates exact body hashes, source contexts and every
semantic assessment. Equivalent references can share assessment work within one
immutable context, while each recorded location must still be checked. Generator
projection uses `Artifact.Resolver()` to validate once for a batch of semantic
lookups. For a complete read operation, `readsession` owns an
`ontologyevidence.Preparer` shared by its selected source sessions. It validates
exact document/body bytes once, retaining source-closure and bundle consistency
checks. A digest alone cannot authorize changed bytes or a different body identity.
Only successfully validated closures produce a `Prepared` capability.

Snapshot evidence carries that capability through structural checks and generator
projection. Consumers use `Evidence.LegacyInterpretation()`, then `Document()` or
`Resolver()`. Returned collections are independent copies; immutable strings and
private checked contexts can be shared. The capability owns no database access and
can outlive the read transaction. The preparer/session is single-threaded and
operation-local; a cached lookup still checks cancellation and session lifetime.
No unresolved assessment is promoted. Raw `Legacy()`/`Artifact()` remain available
for exact transport, not repeated interpretation in renderers. Generator boundary
tests guard that distinction. These are internal Go values, not new wire formats
or global caches.

Local ontology summaries, usage counts and reference matching read the immutable
`context_document` retained beside a legacy snapshot. Capture and history import
validate that context against the owned archive. Queries still require the selected
project/version and matching full-snapshot receipt; they do not parse embedded
source contexts for a local facet. Parent summaries bind each source separately.
Complete assembly and transport retain the full document and its closure checks.

Evidence hashing uses bounded scratch space while preserving the exact SHA256
bytes. Reference assessment scans project and body alias registries separately,
avoiding a combined copy for every reference. Both registries remain complete;
conflicting aliases still produce ambiguity. Allocation profiles should distinguish
these temporary copies from retained graph memory and serialized-byte budgets.

Local-context consistency digests stream the existing JSON representation directly
into SHA256, excluding embedded sources exactly as before. This avoids retaining
another encoded context just to hash it. The codec uses the pinned Go toolchain's
standard-library JSON streaming API with `encoding/json` compatibility options;
original artifacts, body bytes and their exact-byte digests remain unchanged.

Evidence decoding streams into fresh private values. Legacy JSON error reporting
would prevalidate and buffer the entire input, so it is disabled on the streaming
success path. If that path fails, the original decoder processes the same bytes
into a fresh target to preserve its error and acceptance behavior. Trailing input
is checked, and failed preparation returns no capability. This never changes the
selected evidence or reads another store. Large individual strings and retained
decoded values still allocate memory; streaming is not a heap or RSS bound.

Structural snapshot freezing serializes each typed row once and owns that buffer
privately. Complete and fragment getters decode rows directly without assembling
or storing another whole-project JSON envelope. The shared codec checks every
section against the coverage registry; input and output pointers/bytes remain
independent, including during concurrent reads. Removing whole-project copies
reduces temporary allocation but does not bound ontology decoding, graph indexes,
database transport or buffered export memory. Measure cumulative allocation and
peak RSS separately after any codec change.

Captured dependency reads validate the complete source closure, current source
authorization and every member's source pin before checking archived bodies.
One query per nonempty retained inventory finds the first missing member in
kind/ID order. It reads that inventory and the exact source archives on the same
transaction; it does not build caller-supplied identity arrays or select latest
versions. A missing/unknown kind, wrong owner/version or query failure returns no
partial plan. Source-project checks still run separately. Capture and draft
planning retain their individual identity lookups; a conformance test keeps the
scalar and batch archive identity definitions aligned. Query-count regressions
exercise a large captured membership without timing thresholds in unit tests.

Entity assembly discovers dependencies using only newly selected identities in
each round and queries the sections affected by those selectors. Already returned
vocabulary child identities do not trigger another discovery read when the central
registry confirms that they select no additional section. Once the closure stops
growing, assembly materializes that restricted selection once in database order,
including metadata. This preserves ordering/collation without merging independently
ordered pages in Go. It is not a whole-project read/filter fallback. Some rows can
appear under multiple parent selectors, and discovery rows are read again in the
final pass; **both passes count toward row and encoded-byte budgets**. A final
containment check rejects new references outside the discovered selection. An
exhaustive test compares the discovery registry with draft/archive SQL selectors
so new sections and selector changes cannot silently bypass discovery.

Regression checks should cover selection, authorization, immutability, ambiguous
references and draft/release parity. Keep timing thresholds out of small unit
tests; establish explicit workload budgets on a controlled benchmark runner and
record failures as rollout blockers. Local service-boundary timings do not include
HTTP delivery, frontend rendering, WAN latency or mutation/capture contention.
