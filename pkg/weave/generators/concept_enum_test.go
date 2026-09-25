package generators

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeEnumReader struct {
	uris     []string
	gotLists []string
}

func (f *fakeEnumReader) ConceptListMemberURIs(_ context.Context, _ string, listIDs []string) ([]string, error) {
	f.gotLists = append(f.gotLists, listIDs...)
	return f.uris, nil
}

// TestAttachConceptEnums proves only sealed bound lists get a value enum, and
// the reader is asked only for the sealed list ids.
func TestAttachConceptEnums(t *testing.T) {
	reader := &fakeEnumReader{uris: []string{"u1", "u2"}}
	s := &Service{}
	s.SetConceptEnumReader(reader)

	snap := &Snapshot{
		Project: domain.Project{Entity: domain.Entity{ID: "P"}},
		Fields: []FieldNode{
			{Field: domain.ResolvedField{ConceptLists: []domain.EntityRef{{ID: "CLsealed", IsClosed: true}}}},
			{Field: domain.ResolvedField{ConceptLists: []domain.EntityRef{{ID: "CLopen", IsClosed: false}}}},
			{Field: domain.ResolvedField{}},
		},
	}
	s.attachConceptEnums(context.Background(), snap)

	if got := snap.Fields[0].ConceptEnum; len(got) != 2 {
		t.Fatalf("sealed-bound field should get enum, got %v", got)
	}
	if got := snap.Fields[1].ConceptEnum; got != nil {
		t.Fatalf("open-bound field should get no enum, got %v", got)
	}
	if got := snap.Fields[2].ConceptEnum; got != nil {
		t.Fatalf("unbound field should get no enum, got %v", got)
	}
	if len(reader.gotLists) != 1 || reader.gotLists[0] != "CLsealed" {
		t.Fatalf("reader should be asked only for sealed lists, got %v", reader.gotLists)
	}
}

// TestAttachConceptEnums_NoReader is a no-op when no reader is wired.
func TestAttachConceptEnums_NoReader(t *testing.T) {
	s := &Service{}
	snap := &Snapshot{Fields: []FieldNode{{Field: domain.ResolvedField{ConceptLists: []domain.EntityRef{{ID: "CL", IsClosed: true}}}}}}
	s.attachConceptEnums(context.Background(), snap)
	if snap.Fields[0].ConceptEnum != nil {
		t.Fatalf("no reader → no enum")
	}
}
