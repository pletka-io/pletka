//go:build integration

package ontologyrelease_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/namespace"
)

func TestPublicationRetainsSameWeightAliases(t *testing.T) {
	defaults(t)
	f := candidate(t, "publication_aliases")
	const prefix = "publication_alias_registry"
	// Multiple URI variants per prefix are the persisted alias model. They
	// need not form an effective one-URI-per-prefix map to identify a term.
	_, err := f.pool.Exec(t.Context(), `INSERT INTO weave_namespace_bindings
		(id,created_at,updated_at,prefix,namespace,weight,source) VALUES
		('publication-alias-old',now(),now(),$1,$2,40,'version-derived'),
		('publication-alias-new',now(),now(),$1,'https://new.example/publication_aliases/',40,'version-derived')`,
		prefix, "https://example.org/"+f.input.Classes[0].Prefix+"/")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		if _, err := f.pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE prefix=$1`, prefix); err != nil {
			t.Error(err)
		}
	})
	f.input.Relations = []domain.OntologyRelation{{SourceID: f.input.Classes[0].ID,
		SourceKind: "class", RelType: "subclass_of", TargetQname: prefix + ":A"}}
	f.publish(t)
	doc, err := f.artifact(t).Decode()
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Bindings []namespace.Binding `json:"namespace_bindings"`
	}
	if err := json.Unmarshal([]byte(doc.BodyJSON), &body); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, binding := range body.Bindings {
		if binding.Prefix == prefix && binding.Weight == 40 {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("wanted both tied aliases in the sealed body, got %d", count)
	}
	for _, requirement := range doc.Requirements {
		if requirement.Kind == "reference" && requirement.Name == prefix+":A" && requirement.Disposition == "local" {
			return
		}
	}
	t.Fatalf("local reference was not captured: %+v", doc.Requirements)
}
