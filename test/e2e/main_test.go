//go:build integration

// Package e2e is the black-box, HTTP-level regression net for the assembled
// Pletka app. Each test boots the real app.New HTTP surface over an isolated,
// fixture-hydrated database clone and drives a flow through the public routes
// only — no in-process handler calls, no store access except to read back the
// generated ULIDs the HTTP layer does not expose.
//
// It is deliberately a starter: one end-to-end flow (read + generator export)
// that proves the wiring holds together, and an expandable place to add more.
package e2e

import (
	"os"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestMain provisions the schema+fixture template and an isolated per-package
// clone via testdb.Setup, then points testdb.Pool at that clone for the
// duration of this package's DB tests. See internal/testdb for the
// container/template/clone lifecycle.
func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }
