//go:build integration

package autocomplete_test

import (
	"os"
	"testing"
)

// TestMain skips this package's integration lane entirely unless
// TEST_DATABASE_URL points at a seeded database. Every integration-tagged test
// here is a real-data smoke built on real cross-version CRM/AAT/crmgeo ontology
// data that the synthetic fixtures deliberately do not reproduce, so the
// package cannot run testdb.Setup (a fixture clone would make the smokes fail).
// Exiting 0 before the pool is acquired keeps the fixture lane honestly green
// and never trips the REQUIRE_DB unreachable-DB gate. The package's untagged
// unit tests are unaffected — they run in the normal `go test ./...` lane.
//
// When TEST_DATABASE_URL is set, the smokes run against that seeded DB.
func TestMain(m *testing.M) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
