package weave

import (
	"context"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestCreateVocabularyRejectsEmptyProjectID pins the guard that replaced the
// nullable column's protection. weave_vocabularies.project_id is NOT NULL and
// has no foreign key, so an empty owner is a row the database accepts and no
// project-scoped query can ever see. The store has to refuse it itself.
//
// The nil queries handle is deliberate: reaching the database at all would
// mean the guard did not fire.
func TestCreateVocabularyRejectsEmptyProjectID(t *testing.T) {
	store := &vocabularyStore{}

	err := store.CreateVocabulary(context.Background(), &domain.Vocabulary{
		Entity: domain.Entity{
			ID:        "01JCVOCABGHOST0000000000",
			ProjectID: "",
		},
		ConnectorType: "skosmos",
	})

	if err == nil {
		t.Fatal("CreateVocabulary accepted an empty project_id: that inserts a vocabulary owned by no project")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Fatalf("error = %q, want it to name project_id", err)
	}
}
