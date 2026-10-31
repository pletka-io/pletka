package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// A delayed draft writer may consume retained receipts/checkpoints. It must not
// recover missing context through a hot-table assembler, legacy closure, pool
// query, or whole-tree rewrite under the pending user's operation.
func TestCapturedDraftDeliveryCannotReadCurrentContent(t *testing.T) {
	queries := map[string]bool{
		"WeaveDraftEnrolled": true, "WeaveDraftBaseline": true, "WeaveDraftMutation": true,
		"WeaveLastDraftDelivery": true, "WeavePrepareDraftDelivery": true, "WeaveAcknowledgeDraftDelivery": true,
		"WeavePendingDraftProjects": true, "WeaveDraftCommittedRevision": true,
	}
	helpers := map[string]bool{
		"captureService": true, "draftOperation": true, "prepareDraft": true, "deliverDraft": true,
		"capturedDraft": true, "withCapturedRepo": true, "drainCapturedDraft": true, "rebuildPath": true,
	}
	for _, name := range []string{"draft_delivery.go", "draft_git.go", "draft_workflow.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(repoRoot(t), "pkg/service/gitmaterializer", name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if recv, ok := sel.X.(*ast.SelectorExpr); ok {
				if recv.Sel.Name == "queries" && !queries[sel.Sel.Name] {
					t.Errorf("%s reads an unreviewed query %s", name, sel.Sel.Name)
				}
				if recv.Sel.Name == "pool" {
					t.Errorf("%s escapes receipt boundary through pool.%s", name, sel.Sel.Name)
				}
			}
			if recv, ok := sel.X.(*ast.Ident); ok && recv.Name == "m" && !helpers[sel.Sel.Name] {
				t.Errorf("%s calls unreviewed materializer helper %s", name, sel.Sel.Name)
			}
			return true
		})
	}
}
