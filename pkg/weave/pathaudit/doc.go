// Package pathaudit audits stored weave_fields.path_elements entries
// against a project's linked ontology versions: every non-literal element's
// stored type is checked against the source of truth — the
// weave_ontology_classes and weave_ontology_properties tables — so type
// corruption (a class stored as a property, or vice versa) and
// unresolvable qnames surface as errors.
//
// Service holds a *pgxpool.Pool directly rather than going through a slice
// Store. This is a deliberate, narrow exception to the store-boundary rule
// in .claude/rules/database-patterns.md: the audit is one raw, multi-CTE
// SQL sweep across three tables, lifted verbatim from the original
// `pletka weave verify-paths` CLI implementation
// (cmd/weave_verify_paths.go) to keep both the CLI and any future callers
// (e.g. an MCP tool) byte-for-byte equivalent with that original behavior.
// It joins the small, closed list of pool-holding exceptions already
// sanctioned there (health, organization, release) rather than opening a
// new pattern for ordinary slices to copy.
package pathaudit
