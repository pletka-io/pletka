//go:build integration

package vocabulary

// TestConceptNamespace exposes the failure contract to external database tests.
var TestConceptNamespace = (*Service).conceptNamespace
