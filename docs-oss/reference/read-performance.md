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
lookups. Its private contexts cannot be edited by consumers and it does not
promote unresolved evidence. Snapshot evidence also retains its private validated
document for structural consistency checks. These are internal Go values, not new
wire formats or global caches.

Regression checks should cover selection, authorization, immutability, ambiguous
references and draft/release parity. Keep timing thresholds out of small unit
tests; establish explicit workload budgets on a controlled benchmark runner and
record failures as rollout blockers. Local service-boundary timings do not include
HTTP delivery, frontend rendering, WAN latency or mutation/capture contention.
