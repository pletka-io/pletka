package ontologyevidence

import (
	"encoding/json"
	"testing"
)

// Path elements write the generic literal datatype as "rdf:literal", a house
// convention with no such RDF term; the standards term is rdfs:Literal, which
// the path builder substitutes. The derived datatype reference follows the
// same alias; the stored element is untouched.
func TestElementReferencesAliasGenericLiteralDatatype(t *testing.T) {
	for _, spelling := range []string{"rdf:literal", "rdf:Literal"} {
		e := Element{EntityKind: "field", EntityID: "f", Location: "/path_elements/3", Original: json.RawMessage(`{"type":"literal","uri":"rdfs:Literal","prefix":"rdfs","local_name":"Literal","datatype":"` + spelling + `"}`)}
		refs, err := elementReferences(e)
		if err != nil {
			t.Fatal(err)
		}
		var datatype *Assessment
		for i := range refs {
			if refs[i].Location == "/path_elements/3/datatype" {
				datatype = &refs[i]
			}
		}
		if datatype == nil || datatype.Reference.URI != "rdfs:Literal" || datatype.Reference.Kind != "class" {
			t.Fatalf("%s: datatype reference = %+v", spelling, datatype)
		}
	}
	// Other datatypes are passed through unchanged.
	e := Element{EntityKind: "field", EntityID: "f", Location: "/path_elements/0", Original: json.RawMessage(`{"type":"literal","uri":"xsd:date","datatype":"xsd:date"}`)}
	refs, err := elementReferences(e)
	if err != nil {
		t.Fatal(err)
	}
	if got := refs[len(refs)-1].Reference.URI; got != "xsd:date" {
		t.Fatalf("xsd:date rewritten to %q", got)
	}
}
