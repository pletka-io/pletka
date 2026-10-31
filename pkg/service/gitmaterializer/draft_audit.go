package gitmaterializer

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// This is the reviewed legacy projection vocabulary, not schema3 patch coverage.
// Conformance compares it with actual ChangeLogEntry producers. A new kind must
// get an explicit writer before it can drain; unknown is never an audit-only noop.
var legacyDraftAuditKinds = map[string]bool{
	entityTypeField:            true,
	entityTypeModel:            true,
	entityTypeCollection:       true,
	entityTypeCategory:         true,
	"override":                 true,
	"namespace_binding":        true,
	"project-ontology-version": true,
}

func validateDraftAudit(cs sqlcgen.WeaveChangeSet, entries []sqlcgen.WeaveChangeLog) error {
	for _, entry := range entries {
		if !legacyDraftAuditKinds[entry.EntityType] {
			return fmt.Errorf("unsupported draft audit kind %q in change set %d; explicit materializer support is required", entry.EntityType, cs.ID)
		}
		if entry.ChangeSetID != cs.ID || entry.ProjectID != cs.ProjectID || entry.EntityID == "" {
			return fmt.Errorf("draft audit entry %d has invalid change-set, project or entity ownership", entry.ID)
		}
		switch entry.Operation {
		case "create", "update", "delete":
		default:
			return fmt.Errorf("unsupported draft audit operation %q in change set %d", entry.Operation, cs.ID)
		}
	}
	return nil
}
