# Complete project bundle (schema 3)

`pkg/projectbundle` is the pure transport codec for
[complete project values](project-snapshot-values.md). Encoding and decoding are
implemented, along with atomic immutable-history restoration through the CLI and
admin restore runner. Fresh release commits and new release-repoint objects now
use this format. Automatic draft updates, draft initialization and reconciliation
still use their existing format until bounded draft patching is migrated.

The encoder accepts only `projectsnapshot.CompleteProject`. Draft and released
values share one implementation. It has no database, filesystem, configuration,
clock or network access. An entity fragment cannot satisfy its input type.

## File contract

All paths are relative and use forward slashes:

| Path | Content |
| --- | --- |
| `project.yaml` | Canonical JSON (a YAML subset): schema version, explicit root selector and ordered project-context file references |
| `projects/<selector-sha256>/context.json` | Project selection, complete dependency and foreign-membership inventories, evidence and all 22 section inventories |
| `projects/<selector-sha256>/rows/<section>/<key-sha256>.json` | One complete captured row, with every column explicitly present |
| `ontologies/published/<digest>.json` | Exact sealed publication document, including its body and pins |
| `ontologies/legacy/<digest>.json` | Exact legacy document, preserving ambiguity and source contexts |

Selectors have `project_id`, `version` and `draft`. Empty version never implies
draft. The selector path hashes its compact JSON representation in that field
order. A row path hashes a compact JSON array of its full composite primary-key
values in the [coverage registry](../../pkg/projectsnapshot/coverage.go) order.
Neither path substitutes for persisted IDs. References retain the original IDs
and source selection; no UUID, integer-ID remapping or source rebinding occurs.

All section arrays are explicit, including empty arrays. Every file reference
carries its exact byte SHA256. Contexts sort by project identity, row references by
path, dependencies by project identity, memberships by kind/ID, publications by
artifact path, and semantic resolutions by entity kind/ID/location. Array ordering
inside retained record fields remains unchanged. Outer row order has no semantic
meaning; explicit ordering columns are retained.

Canonical envelope and row files use two-space JSON indentation, sorted map keys,
Go JSON string escaping and one final newline. Ontology artifact files are exact
original bytes, including whitespace; they are never pretty-printed. Authored JSON
columns use nullable UTF-8 JSON **text**: SQL NULL is `null`, while JSON null is
`"null"`. This preserves original formatting, key order and arbitrary numeric
precision. Numeric/date/timestamp value types remain strings; integer IDs remain
exact JSON integers and must not pass through floating-point intermediates.

## Evidence and trust

Each context carries its full captured actor/module descriptor document. Draft
contexts have no capture receipt. Contract1 carries its original storage receipt,
publication closure and semantic resolutions. Contract2 carries its original
structural receipt and exact legacy ontology document, without semantic promotion.
Source projects retain their own interpretation contexts.

`Decode` returns `ImportedProject`, which exposes a detached snapshot for inspection
and rendering. It does not authenticate a sender, grant access or certify a local
release. Portable file hashes detect inconsistent transport; stored receipt hashes
refer to PostgreSQL row values and are preserved, not verified by the pure codec.
The storage import must validate those against restored rows and retain imported
provenance separately from a fresh local capture.

Decoding validates all existing complete-value invariants, then requires canonical
re-encoding to reproduce every supplied file byte. Unknown, duplicate or missing
fields, nullable/nonnullable coercion, altered paths/hashes, extra files, mixed
formats and incomplete section/source inventories fail. A hand-edited document
must satisfy the same canonical contract; permissive JSON/YAML loading is not an
alternative completeness path.

## I/O and limits

`LoadCompleteProjectBundle` in `pkg/service/gitmaterializer` loads schema 3 trees
using `os.Root`, rejects symlinks and nonregular files, and limits bytes and visited
directory entries before allocating file bodies. `.git` is excluded.
`releases/index.yaml` is returned separately as an opaque catalog overlay; it is
not frozen project content. All other files must be covered by the codec.

The pure decoder requires positive byte/file/project limits. Defaults are 256 MiB,
1,100,000 files and 256 projects. Stored bytes include path bytes; cumulative referenced
bytes also count against the byte limit, including shared evidence read by multiple
contexts. Repeated structural files fail before graph reconstruction. These are
input/work limits, not a bound on peak Go heap allocation. Filesystem loading
counts directories too, so its entry limit can be reached before the pure file
limit. Callers must supply a stable checkout, without concurrent mutation.

Historical formats 1/2 remain on `LoadProjectSnapshot`; their missing sections do
not acquire completeness. That loader rejects schema 3 immediately. Published tags
must continue to be verified using their stored format and corresponding evidence;
a new-format tree must never be compared with old-format tag bytes.

## Release Git writer and verification

The release materializer opens one authorized read session, assembles the complete
root/source graph, and loads an explicit catalog on that same snapshot through
`projectread.Catalog`. The bound session supplies all root release headers and only
the selected header for each source. Encoding and filesystem writes happen after
the session closes; no writer performs its own entity or recursive vendor queries.
The annotated tag message comes from that same detached catalog.

Verification dispatches on the tag's stored `project.yaml` schema. Schema3 uses the
complete serializer and validates/reuses the tag's frozen catalog bytes. It does
not load today's catalog. Schema1/2 use their compatibility serializer and existing
archive prerequisites. Unknown formats and unavailable evidence fail explicitly;
known-format tree differences remain a persistent mismatch. Verification never
rewrites tags.

Archiving a release preserves the releases branch's stored content and catalog
format, replacing only `releases/index.yaml`. Schema3 retains source-release
headers. Git blobs are budgeted before loading and streamed in one `cat-file`
process; export attributes cannot alter their bytes. An interrupted repoint also
verifies already-prepared objects against their stored format and frozen catalog,
even after a writer upgrade or later title/retirement edits.

| Transitional adapter | Owner and boundary | Retirement condition |
| --- | --- | --- |
| `writeLegacyReleaseSnapshot`, `versioned_tree.go`, recursive vendor writers | `gitmaterializer`; schema1/2 stored-tag and prepared-object verification only on release paths. They never supply a complete value. | Replace with a value-only historical-format serializer during adapter retirement; retain old-tag verification without rewriting tags. |
| Existing draft tree/closure writers | `gitmaterializer`; draft initialization, reconciliation and change-set processing only. | Switch together after bounded patching covers every retained section and proves `NoCrossEntityStealing`. |

Draft migration must not replace a scoped change-set write with a full current
project rebuild. It needs an existing complete baseline, explicit ownership of
replacement/deleted rows and coverage for examples, vocabulary/concepts, credits
and placements as well as fields/models/collections. Baseline format migration
must be distinct from an individual user's change-set commit.

## Release catalog overlay (schema 2)

`NewCatalog` takes a complete snapshot and explicit `ReleaseRecord` values.
`EncodeCatalog` writes the separate `releases/index.yaml` overlay: canonical JSON
with `schema_version: 2`, `root_project_id`, and records ordered by project/version.
Each record preserves project/version, title, description, original creation time
and creator ID, optional retirement time, and retirement message. Retirement and
other catalog edits do not change the frozen project bundle.

Every selected concrete release needs its original catalog record, including each
source project. Creator identities must occur in that release's captured descriptor
context. Missing dates or authors are errors; the codec never invents import-time
replacements. Additional root-project history may be listed, but
`Catalog.SelectedReleases` returns only releases with complete bodies in this
snapshot. An index entry alone cannot create an empty historical release. Unselected
source records and substituted versions fail validation.

`DecodeCatalog` checks byte limits, root/selection coverage and canonical encoding.
Legacy schema 1 catalogs remain on the legacy path; they do not contain the source
metadata required for complete history import. The filesystem loader returns the
overlay bytes separately. A storage importer must require and validate this overlay
before any writes; plain content inspection does not require a catalog.

## Immutable history restoration

`database/projectrestore.Restore` accepts only the decoded import and validated
catalog. One transaction restores original release headers, every archived row,
source pins/membership, descriptors and exact ontology evidence, then creates
working copies. Both typed row equality and the original PostgreSQL receipt hashes
are checked. SQL NULL and JSON null remain distinct through the storage conversion.
Imported finalization does not compare historical values with current hot rows.

An immutable ledger identifies imported history separately from a local release
cut. Normal release-scoped readers load the restored archives; the bound session's
`Origin` method reports the local import time outside the frozen content value.
Imports cannot commit unfinished releases or modify already finalized archives.

Restoration requires absent targets or exact prior imports and preserves original
IDs. Exact retries leave later working-copy edits intact; conflicts and retargeting
fail. Working copies have draft status. Missing instance-local staging attachments
are cleared and reported, while archives retain them. Current global namespace and
ontology defaults remain current; historical interpretation stays in release
evidence. Sequence advances can leave gaps after rollback. See
[ADR-0019](../decisions/0019-imported-project-history-keeps-its-origin.md) for the
transaction, provenance and working-copy rules.
