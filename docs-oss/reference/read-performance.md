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
