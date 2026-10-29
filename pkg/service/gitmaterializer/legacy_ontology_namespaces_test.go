package gitmaterializer

import (
	"strings"
	"testing"
)

func TestLegacyTransportRejectsEffectiveNamespaces(t *testing.T) {
	for name, namespaces := range map[string]*projectManifestNamespaces{
		"empty map": {},
		"collapsed alias": {EffectiveBindings: []projectManifestNamespaceBinding{
			{Prefix: "crmarchaeo", Namespace: "https://example.org/crmarchaeo/"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := legacyTransportFixture(t)
			snapshot.Manifest.Namespaces = namespaces
			if err := validateOntologyRequirements(snapshot); err == nil || !strings.Contains(err.Error(), "namespaces") {
				t.Fatalf("schema 2 accepted an effective namespace map: %v", err)
			}
		})
	}
}
