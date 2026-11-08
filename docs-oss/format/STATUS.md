# Implementation status

Where the code (`pkg/service/gitmaterializer`, `cmd/project*.go`) stands
against this specification, as of 2026-11-08. Update this file with every
change that closes a gap.

Legend: **done** — as specified; **partial** — exists, differs as noted;
**none** — not built.

## Tree

| Rule | State | Today |
|---|---|---|
| Layout, one file per entity, semantic-id names | done | `init.go`, `vendor_snapshot.go` `writeProjectTree` |
| Compact authoring forms (qname paths, prefix+version links, omitted keys) | done | `restore_entities.go` `decodePathElement`, `restore_hydrator.go` |
| Concept lists in the tree | partial | the list only; entries stay in the database |
| `source_mode`/`source_version` removed from `project.yaml` | none | still written and cross-checked against `pletka.mod` (`snapshot_loader.go` `validateParentDependencySelectors`) |
| `source.version` removed from receipts | none | written from the database |
| Database ids optional in output | partial | written beside prefixes; `ontology_version_id` also inside path elements |
| Strict decoding | none | decode goes YAML → `any` → JSON, unknown keys dropped (`snapshot_loader.go` `decodeCanonicalYAML`) |
| `schema_version` checked | none | written as `1`, never read |
| Canonical encoder | done | `pkg/weave/canonical/yaml.go` |
| `path_elements[].uri` as full IRI | partial | writer emits qnames as `uri`; the IRI form is a pending normalisation |

## Modules

| Rule | State | Today |
|---|---|---|
| `pletka.mod` written | done | `pletka_mod.go`; versioned variant `versioned_tree.go` |
| Module path = repository location | none | labels `pletka.io/{orgs,users,actors}/<slug>/projects/<ID>`, `ontology.pletka.io/<slug>`; `init-git` ignores configured hosts |
| No `mode`, no ids in requirements | none | `mode: parent`, `ontology_id`, `ontology_version_id` written |
| `require.ontologies` read on load | none | ontology links come from `project.yaml` only |
| `replace` | none | always `[]`, never read |
| Selectors: branch, commit | none | `draft` or a release version only; a database CHECK constraint forbids more |
| Resolve a selector through git | none | parents and ontologies come from the database (live or archive tables) |
| `pletka.sum` always written | partial | only with `init-git --self-contained`; not refreshed by later saves |
| `git:` in `pletka.sum` | partial | parsed, never written |
| Tree hash over content files | partial | hashes every file under the vendored directory |
| Sum verification | partial | recomputes vendored hashes on load and in verify-git; stops at first mismatch; no `sum-missing`/`sum-extra` |
| Module cache | none | — |
| `vendor/<module>@<selector>` layout | none | `vendor/projects/<module>[/<version>]` (draft and release of one module nest), `vendor/ontologies/<module>/<version>` |
| Vendored files copied byte for byte | none | rendered from the database |
| Vendoring is transitive and flat | done | `vendor_snapshot.go` `vendorDependenciesForProject` |
| Vendored projects skipped on load when present | done (by design) | `restore_vendored_projects.go`; existing projects are not compared |
| Ontology repository, one module per prefix | none | a repository exists, grouped by family, tags `<prefix>/v<version>` |
| `ontology.yaml` `require` | none | `imports: [string]` |
| One version per module in the closure | none | not checked; one existing violation, repaired when git resolution lands (D9) |
| `pletka mod` commands | none | `init-git --self-contained` then copy `vendor/` + `pletka.sum` by hand |

## Versions

| Rule | State | Today |
|---|---|---|
| Release = annotated tag `v<version>` | done | `release_materialize.go` |
| Tag on a commit of `main` | none | tags point at a separate `refs/heads/releases` history built from archive tables |
| Draft branch named `main` | partial | plain `git init`; name depends on git config |
| `releases/index.yaml` on `main` | none | on the releases history only |
| Release refused while a requirement uses a branch selector | partial | refused while a parent follows `draft` |
| Load a release from its tag | none | `load-git` restores drafts only |
| Release branches | none | — |

## Verification

See the **[not built]** markers in [`verification.md`](verification.md).
Known defects:

- The verify workflow must live under `.github/workflows/`; a copy at the
  repository root does not run.
- `adoption-receipt-missing` is reported but was missing from the
  documented finding codes.
- The versioned `pletka.mod` writer looks up a parent's archived row with
  the child's version number (`versioned_tree.go`
  `loadPletkaProjectRequirementsAtVersion`) and does not sort requirements.

## Schema conformance of existing trees

The schemas were run (2026-11-08) against every available materialized tree:
132,493 files across 130 trees (project, `pletka.mod`, receipts, entities,
overrides, vendored `ontology.yaml`). Everything validates except three
data defects, which the schemas correctly reject:

| Defect | Files |
|---|---|
| A model numbered `0` (`<P>M.0`), invalid under the semantic-id grammar, and the overrides and refs naming it | 29 |
| Collections whose `ontology_scope` is an empty element (all keys `""`) | 6 |
| A base override whose `category_id` is a database id instead of a semantic id (category adopted from a parent) | 2 |

Keys that v2 removes (`source_mode`, `source_version`, `mode`, database ids,
`artifact_digest`, receipt `source.version`, `imports`) are accepted and
marked `deprecated` in the schemas, so v1 trees validate unchanged.
