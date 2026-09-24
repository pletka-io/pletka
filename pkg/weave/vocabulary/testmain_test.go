//go:build integration

package vocabulary_test

import (
	"os"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestMain provisions the migrated schema template + an isolated per-package
// clone via testdb.Setup; testdb.Pool then hands tests that clone.
func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }
