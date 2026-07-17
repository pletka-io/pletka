package weave_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
)

// TestPostgresStore_ImplementsWeaveStore is a compile-time assertion
// that PostgresStore implements the full domain.WeaveStore interface including
// the ProjectOntologyVersions and NamespaceBindings surfaces added in Task 4.
func TestPostgresStore_ImplementsWeaveStore(t *testing.T) {
	var _ domain.WeaveStore = (*weave.PostgresStore)(nil)
}
