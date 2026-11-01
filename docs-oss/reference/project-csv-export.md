# Project CSV verification exports

Project members can download eight CSVs at `/projects/{projectID}/exports/`,
individually at `/{type}.csv` or together at `/all.zip`. The omitted `version`
parameter deliberately selects draft; `?version=1.0.0` selects that release.
Anonymous/public fallback access remains denied. Current ownership and membership
control access independently of archived content.

## One captured input

The HTTP service accepts one complete-project `Source`, implemented by the shared
`service/generatorread` adapter over `readsession` and `projectread`. It closes the
read transaction before returning. The page, each CSV download and each ZIP request
capture exactly once, check the returned selection, and use `csvexport.Build`.
No writer has a store, a session, per-row callbacks or an ontology lookup.

Selected content supplies page names, filenames, rows, foreign-field semantic IDs,
reference order and linked ontology metadata. Foreign field lookup follows retained
membership at its recorded source release. The current auth project cannot supply
export metadata. Missing/unavailable release content uses `not_in_release`; lookup
or decoding failures do not become empty rows, successful partial files or draft.

CSV and ZIP bytes are prepared before HTTP success headers. ZIP output has stable
entry order and metadata, and filenames cannot introduce archive paths. Assembly
budgets apply instead of the old per-list 10,000-row cap. ZIP buffering and complete
assembly for a single CSV or the index page still need large-project performance
qualification; this contract does not promise streaming or constant memory.

## Contents and compatibility

The eight existing formats and headers remain:

- `fields`, `models`, `collections`, `categories`: owned definitions.
- `model-field-overrides`, `collection-field-overrides`, `base-field-overrides`:
  owned raw overrides, with selected foreign-field identifiers where referenced.
- `ontologies`: directly linked versions, using retained publication bodies or
  legacy evidence and the selected project's original link metadata.

Rows use stable identity order; refs use position and stable identity tie-breaks.
Field paths retain authored prefixes, primary and subfield paths. These are
verification rows, not generator-resolved semantics: unresolved legacy meaning
is not silently repaired. Raw override values are not replaced by composed
inherited defaults. The historically omitted `expected_value_type` override and
`ontology_uri` cells now contain their retained values.

`is_active` is external mutable ontology policy, absent from current immutable
artifact bodies. Its cell is blank when not recorded, and preserves a boolean
only when the retained evidence explicitly contains one. Blank does not mean
false. `added_at` retains the captured timestamp text, including precision and
special values; it is not reconstructed from current link rows.

CSV still has its existing null/empty and optional-boolean conventions and omits
many complete-project sections. It is not a backup or a lossless restore format;
use schema3 project transport for that purpose. No schema migration or mutation
capture enrollment is required by this reader conversion.

See [the detached consumer decision](../decisions/0022-generators-consume-detached-inputs.md)
and [whole-project assembly](../decisions/0012-whole-project-reads-belong-to-an-assembler.md).
