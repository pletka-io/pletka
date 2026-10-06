package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

// Inline SQL is SQL written as a Go string literal instead of as a named
// query in pkg/database/queries/*.sql.
//
// Why it is worth a gate rather than a preference: a query in a .sql file is
// checked by sqlc against the migrations, so a column that no longer exists
// fails generation. The same query as a Go string is checked by nothing until
// it runs. It also cannot be found: asked "where is this column read?", a
// reader greps the queries directory and silently misses every string
// literal. Mixing both means neither answer is trustworthy -- the queries
// directory looks complete and is not.
//
// Generated code is exempt because it IS the output of that pipeline.

// sqlVerb matches a literal that contains a SQL statement. Anchored on verbs
// that only appear in SQL, so prose mentioning "select" is not a hit.
var sqlVerb = regexp.MustCompile(`(?is)\b(SELECT\s+|INSERT\s+INTO\s+|UPDATE\s+\w+\s+SET\b|DELETE\s+FROM\s+|CREATE\s+(TABLE|INDEX)\s+|ALTER\s+TABLE\s+|TRUNCATE\s+)`)

// generatedOrExempt are paths whose SQL is not hand-written, or not a query.
func generatedOrExempt(rel string) bool {
	switch {
	case strings.HasPrefix(rel, "pkg/database/sqlcgen/"):
		// sqlc output: this is the pipeline's product, not a bypass of it.
		return true
	case strings.HasPrefix(rel, "pkg/database/migrations/"):
		// goose migrations are DDL by definition and are not queries.
		return true
	case strings.HasSuffix(rel, "_test.go"):
		// A test fixture's INSERT is seeding, not production data access.
		// Tests are still read by people, but a wrong fixture fails the test
		// it belongs to rather than shipping.
		return true
	}
	return false
}

// countInlineSQL returns how many string literals in the file carry a SQL
// statement. It walks the AST, so SQL quoted in a comment does not count --
// the boundary check learned that lesson the hard way.
func countInlineSQL(path string) (int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return 0, err
	}
	n := 0
	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if sqlVerb.MatchString(lit.Value) {
			n++
		}
		return true
	})
	return n, nil
}
