//go:build integration

package ontology_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// TestStore_EmptyNullableFKPointerNormalisesToNull is the regression guard for
// the family-editor "can't restore order" bug: the form SelectWidget's
// make-root / unassign option submits "" for a nullable FK, which JSON-decodes
// into a *string pointing at "". The store must normalise that to SQL NULL, not
// write '' and trip the foreign-key constraint (which surfaced as a bare 500).
func TestStore_EmptyNullableFKPointerNormalisesToNull(t *testing.T) {
	ctx := context.Background()
	store := weaveontology.NewPostgresStore(testPool(t))

	// A root family to parent under, then move away from.
	rootID := ids.GenerateULID()
	if _, err := store.CreateFamily(ctx, weaveontology.CreateFamilyInput{
		ID:          rootID,
		Slug:        "f1-root-" + rootID,
		Name:        "F1 Root",
		Description: domain.Translations{"en": "f1 root"},
	}); err != nil {
		t.Fatalf("create root family: %v", err)
	}

	// A child family created under that root.
	childID := ids.GenerateULID()
	if _, err := store.CreateFamily(ctx, weaveontology.CreateFamilyInput{
		ID:             childID,
		Slug:           "f1-child-" + childID,
		Name:           "F1 Child",
		Description:    domain.Translations{"en": "f1 child"},
		ParentFamilyID: dbutil.Ptr(rootID),
	}); err != nil {
		t.Fatalf("create child family: %v", err)
	}

	// The bug: make the child a root by submitting "" for parent_family_id.
	// Pre-fix this wrote parent_family_id = '' -> FK violation -> 500.
	if _, err := store.UpdateFamily(ctx, childID, weaveontology.UpdateFamilyInput{
		Slug:           "f1-child-" + childID,
		Name:           "F1 Child",
		Description:    domain.Translations{"en": "f1 child"},
		ParentFamilyID: dbutil.Ptr(""),
	}); err != nil {
		t.Fatalf("make child root (empty parent): %v", err)
	}
	gotFam, err := store.GetFamily(ctx, childID)
	if err != nil {
		t.Fatalf("get child family: %v", err)
	}
	if gotFam.ParentFamilyID != nil {
		t.Fatalf("parent_family_id = %q; want NULL after make-root", *gotFam.ParentFamilyID)
	}

	// Creating a family directly with a "" parent (the widget's default option)
	// is also a valid root.
	root2ID := ids.GenerateULID()
	if _, err := store.CreateFamily(ctx, weaveontology.CreateFamilyInput{
		ID:             root2ID,
		Slug:           "f1-root2-" + root2ID,
		Name:           "F1 Root2",
		Description:    domain.Translations{"en": "f1 root2"},
		ParentFamilyID: dbutil.Ptr(""),
	}); err != nil {
		t.Fatalf("create root family with empty parent: %v", err)
	}

	// Ontology: created inside a family, then unassigned via "" family_id and
	// "" extends_ontology_id.
	ontID := ids.GenerateULID()
	prefix := "f1t" + ontID[:6]
	ns := "https://example.test/f1/" + ontID + "/"
	if _, err := store.CreateOntology(ctx, weaveontology.CreateOntologyInput{
		ID:           ontID,
		Prefix:       prefix,
		Namespace:    ns,
		Name:         "F1 Test Ontology",
		Description:  domain.Translations{"en": "f1 ontology"},
		FamilyID:     dbutil.Ptr(rootID),
		OntologyType: domain.OntologyTypeBase,
	}); err != nil {
		t.Fatalf("create ontology: %v", err)
	}
	if _, err := store.UpdateOntology(ctx, ontID, weaveontology.UpdateOntologyInput{
		Prefix:            prefix,
		Namespace:         ns,
		Name:              "F1 Test Ontology",
		Description:       domain.Translations{"en": "f1 ontology"},
		FamilyID:          dbutil.Ptr(""),
		OntologyType:      domain.OntologyTypeBase,
		ExtendsOntologyID: dbutil.Ptr(""),
	}); err != nil {
		t.Fatalf("unassign ontology family/extends (empty): %v", err)
	}
	gotOnt, err := store.GetOntology(ctx, ontID)
	if err != nil {
		t.Fatalf("get ontology: %v", err)
	}
	if gotOnt.FamilyID != nil {
		t.Fatalf("family_id = %q; want NULL after unassign", *gotOnt.FamilyID)
	}
	if gotOnt.ExtendsOntologyID != nil {
		t.Fatalf("extends_ontology_id = %q; want NULL after unassign", *gotOnt.ExtendsOntologyID)
	}
}
