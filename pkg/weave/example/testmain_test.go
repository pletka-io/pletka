//go:build integration

package example_test

import (
	"os"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }
