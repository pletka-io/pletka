//go:build integration

package versionaudit_test

import (
	"os"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestMain provisions an isolated per-package database clone; see
// internal/testdb.
func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }
