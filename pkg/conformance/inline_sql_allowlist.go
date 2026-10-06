package conformance

// InlineSQLAllowlist is the phase-out plan for SQL written as Go string
// literals instead of named queries in pkg/database/queries/*.sql.
//
// Statements is a CEILING, not a note. A file may hold the count recorded
// here and no more, so an allowlisted file cannot grow new inline SQL: the
// gate fails on the increment. Converting queries out of a file means
// lowering its number, and the diff shows the debt shrinking.
//
// Entries are temporary unless marked Permanent. The rule is the same as the
// read allowlist's: every entry names why, and the list shrinks because
// numbers go down, not because entries are quietly deleted.
//
// Owner decision, 2026-10-06: inline SQL and named queries are not to be
// mixed. A query in a .sql file is checked by sqlc against the migrations, so
// a column that no longer exists fails generation; the same query as a Go
// string is checked by nothing until it runs, and cannot be found by anyone
// grepping the queries directory. Mixing both makes the queries directory
// look complete when it is not.

const (
	// The SQL is in the right layer and the wrong form: a slice store, where
	// data access belongs, writing its statements inline instead of calling a
	// generated query. Converts when that store's queries move to a .sql file.
	storeLayerWrongForm = "store layer, inline form: move these to pkg/database/queries and regenerate"

	// Two violations at once: SQL in a service breaks the store-layer rule as
	// well as this one. These convert with the slice's store extraction, not
	// before -- moving the string into a .sql file while leaving the call in
	// the service would satisfy this gate and leave the worse problem.
	sqlInAService = "SQL in a service: extract a store first, then name the queries"

	// The pre-slice aggregate stores under pkg/weave/*.go. They predate the
	// slice layout and convert with whatever slice eventually claims them.
	aggregateStore = "legacy aggregate store: converts with the slice that claims these tables"

	// Materialize and restore paths, which are not slices and read across many
	// tables at once. They need named queries too; they just have no slice to
	// convert with, so they are their own piece of work.
	materializer = "git materializer: needs named queries, no owning slice to convert with"

	// Hand-written SQL outside any of the above shapes.
	otherHandWritten = "hand-written SQL outside a store: needs a home and a named query"

	// Session-level advisory locks. SELECT pg_advisory_lock($1) is a lock
	// primitive, not a query over tables: there is no result set to map and
	// nothing for sqlc to validate against the schema. Permanent.
	lockPrimitive = "advisory lock primitive, not a table query: nothing for sqlc to check"
)

// InlineSQLExemption records what a file is allowed to hold, and why.
type InlineSQLExemption struct {
	// Statements is the maximum number of SQL-bearing string literals the
	// file may contain. Exceeding it fails the gate.
	Statements int

	// Reason is one of the constants above.
	Reason string

	// Permanent marks an exemption that is correct forever, rather than debt.
	Permanent bool
}

// InlineSQLAllowlist maps a repo-relative Go file to what it is currently
// allowed to hold. Keys are paths because the ceiling is per file: a slice
// converts its queries file by file, and the number is what shrinks.
var InlineSQLAllowlist = map[string]InlineSQLExemption{
	"pkg/app/observability/collectors.go":                {Statements: 1, Reason: otherHandWritten, Permanent: false},
	"pkg/database/advisorylock/advisorylock.go":          {Statements: 4, Reason: lockPrimitive, Permanent: true},
	"pkg/formschema/ontology_probe.go":                   {Statements: 1, Reason: otherHandWritten, Permanent: false},
	"pkg/service/gitmaterializer/adoptions_manifest.go":  {Statements: 1, Reason: materializer, Permanent: false},
	"pkg/service/gitmaterializer/forks_manifest.go":      {Statements: 1, Reason: materializer, Permanent: false},
	"pkg/service/gitmaterializer/project_manifest.go":    {Statements: 1, Reason: materializer, Permanent: false},
	"pkg/service/gitmaterializer/release_materialize.go": {Statements: 4, Reason: materializer, Permanent: false},
	"pkg/service/gitmaterializer/releases_index.go":      {Statements: 1, Reason: materializer, Permanent: false},
	"pkg/service/gitmaterializer/restore_hydrator.go":    {Statements: 5, Reason: materializer, Permanent: false},
	"pkg/service/gitmaterializer/restore_overrides.go":   {Statements: 6, Reason: materializer, Permanent: false},
	"pkg/service/gitmaterializer/versioned_loaders.go":   {Statements: 12, Reason: materializer, Permanent: false},
	"pkg/service/gitmaterializer/versioned_tree.go":      {Statements: 5, Reason: materializer, Permanent: false},
	"pkg/weave/adoption_store.go":                        {Statements: 4, Reason: aggregateStore, Permanent: false},
	"pkg/weave/category/scoped_reader.go":                {Statements: 4, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/category/store_postgres.go":               {Statements: 4, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/collection/store_postgres.go":             {Statements: 9, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/errortracking/store.go":                   {Statements: 2, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/example/store_postgres.go":                {Statements: 10, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/field/store_postgres.go":                  {Statements: 13, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/fork_store.go":                            {Statements: 3, Reason: aggregateStore, Permanent: false},
	"pkg/weave/generators/sparql/renderer.go":            {Statements: 2, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/gitrestoreadmin/store.go":                 {Statements: 5, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/materializationadmin/store.go":            {Statements: 3, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/members/service.go":                       {Statements: 1, Reason: sqlInAService, Permanent: false},
	"pkg/weave/model/store_postgres.go":                  {Statements: 9, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/namespacebinding/store_postgres.go":       {Statements: 1, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/ontology/store_postgres.go":               {Statements: 5, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/organization/store_postgres.go":           {Statements: 2, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/orgmembers/service.go":                    {Statements: 1, Reason: sqlInAService, Permanent: false},
	"pkg/weave/pathaudit/pathaudit.go":                   {Statements: 1, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/project/add_ontology.go":                  {Statements: 5, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/project/repin.go":                         {Statements: 4, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/project/store_delete.go":                  {Statements: 20, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/project/store_postgres.go":                {Statements: 5, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/project_inheritance_store.go":             {Statements: 17, Reason: aggregateStore, Permanent: false},
	"pkg/weave/project_ontology_version_store.go":        {Statements: 1, Reason: aggregateStore, Permanent: false},
	"pkg/weave/projectontologyversion/formschema.go":     {Statements: 1, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/projectontologyversion/store_postgres.go": {Statements: 8, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/publication/publication.go":               {Statements: 6, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/release/reconstruct.go":                   {Statements: 17, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/release/service.go":                       {Statements: 27, Reason: sqlInAService, Permanent: false},
	"pkg/weave/release/service_archive.go":               {Statements: 3, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/release/store_postgres.go":                {Statements: 2, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/resolve.go":                               {Statements: 9, Reason: aggregateStore, Permanent: false},
	"pkg/weave/router/test_helpers.go":                   {Statements: 1, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/search/path.go":                           {Statements: 6, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/search/selector_search.go":                {Statements: 11, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/search/selector_targets.go":               {Statements: 4, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/store.go":                                 {Statements: 1, Reason: storeLayerWrongForm, Permanent: false},
	"pkg/weave/version/handler.go":                       {Statements: 1, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/vocabconnector/aat/queries.go":            {Statements: 2, Reason: otherHandWritten, Permanent: false},
	"pkg/weave/vocabulary/service.go":                    {Statements: 17, Reason: sqlInAService, Permanent: false},
	"pkg/weave/weave_project_store.go":                   {Statements: 13, Reason: aggregateStore, Permanent: false},
}
