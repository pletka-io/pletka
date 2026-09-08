//go:build integration

package hub

import (
	"os"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestMain provisions the schema+fixture template and an isolated per-package
// clone via testdb.Setup, then points testdb.Pool at that clone for the
// duration of this package's DB tests. See internal/testdb for the
// container/template/clone lifecycle. Mirrors
// pkg/weave/settings/testmain_test.go — hub had no integration-tagged tests
// before this fix, so no TestMain existed yet.
func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }
