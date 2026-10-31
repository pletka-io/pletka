package gitmaterializer

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

func TestDraftAuditFailsClosed(t *testing.T) {
	cs := sqlcgen.WeaveChangeSet{ID: 1, ProjectID: "project"}
	valid := sqlcgen.WeaveChangeLog{ChangeSetID: 1, ProjectID: "project", EntityType: "field", EntityID: "f", Operation: "update"}
	for kind := range legacyDraftAuditKinds {
		for _, op := range []string{"create", "update", "delete"} {
			row := valid
			row.EntityType, row.Operation = kind, op
			if err := validateDraftAudit(cs, []sqlcgen.WeaveChangeLog{row}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, edit := range map[string]func(*sqlcgen.WeaveChangeLog){
		"unknown kind":      func(row *sqlcgen.WeaveChangeLog) { row.EntityType = "example" },
		"unknown operation": func(row *sqlcgen.WeaveChangeLog) { row.Operation = "merge" },
		"other project":     func(row *sqlcgen.WeaveChangeLog) { row.ProjectID = "other" },
		"other change set":  func(row *sqlcgen.WeaveChangeLog) { row.ChangeSetID++ },
		"missing entity":    func(row *sqlcgen.WeaveChangeLog) { row.EntityID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			row := valid
			edit(&row)
			if err := validateDraftAudit(cs, []sqlcgen.WeaveChangeLog{valid, row}); err == nil {
				t.Fatal("invalid entry would drain silently")
			}
		})
	}
}
