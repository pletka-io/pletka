# Complete project bundle (schema 3)

`pkg/projectbundle` is the pure transport codec for
[complete project values](project-snapshot-values.md). Encoding and decoding are
implemented. Storage import and the switch of Git writers to this format remain
separate delivery gates. Existing writers still emit their historical formats.

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
