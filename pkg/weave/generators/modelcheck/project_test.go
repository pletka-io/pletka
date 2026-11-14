package modelcheck

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func model(sem, class string, types ...string) *generators.Snapshot {
	snap := &generators.Snapshot{Model: &domain.Model{Entity: domain.Entity{SemanticID: sem}, OntologyScope: el("class", class)}}
	for _, v := range types {
		snap.Fields = append(snap.Fields, chained(sem+".t", "Concept", 1, v, p2, e55))
	}
	return snap
}

func byCode(fs []Finding, code string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func TestC13ClassOnlyModel(t *testing.T) {
	if got := byCode(Model(model("PM.1", "crm:E7_Activity")), "C13"); len(got) != 1 || got[0].Severity != Note {
		t.Fatalf("%+v", got)
	}
	if got := byCode(Model(model("PM.1", "crm:E22_Human-Made_Object", "aat:1")), "C13"); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestC5SharedClassWithoutDiscriminator(t *testing.T) {
	fs := Project([]*generators.Snapshot{model("PM.1", "crm:E7_Activity"), model("PM.2", "crm:E7_Activity"), model("PM.3", "crm:E21_Person")})
	got := byCode(fs, "C5")
	if len(got) != 1 || got[0].Model != "PM.1" || got[0].Severity != Error {
		t.Fatalf("%+v", got)
	}
}

func TestC5ApartByType(t *testing.T) {
	fs := Project([]*generators.Snapshot{model("PM.1", "crm:E22_Human-Made_Object", "aat:1"), model("PM.2", "crm:E22_Human-Made_Object", "aat:2")})
	if got := byCode(fs, "C5"); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestProjectRunsModelChecks(t *testing.T) {
	bad := model("PM.1", "crm:E7_Activity")
	bad.Fields = append(bad.Fields, fn("PF.1", "String", 2, intp(1), el("property", "rdfs:label"), el("literal", "xsd:string")))
	if got := byCode(Project([]*generators.Snapshot{bad}), "C3"); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}

// a collection placed on several models gives the same finding for each;
// the project reports it once, naming the models
func TestProjectFoldsIdenticalCollectionFindings(t *testing.T) {
	placed := func(model string) *generators.Snapshot {
		snap := &generators.Snapshot{Model: &domain.Model{Entity: domain.Entity{SemanticID: model}, OntologyScope: el("class", "crm:E7_Activity")}}
		f := chained("PF.9", "Concept", 0, "", p2, e55)
		f.CollectionID, f.CollectionSemanticID = "c", "PC.1"
		snap.Fields = []generators.FieldNode{f}
		return snap
	}
	got := byCode(Project([]*generators.Snapshot{placed("PM.1"), placed("PM.2"), placed("PM.3")}), "C10")
	if len(got) != 1 || got[0].Model != "" || len(got[0].Models) != 3 || got[0].Models[0] != "PM.1" {
		t.Fatalf("%+v", got)
	}
	// a finding in only one model keeps its model
	got = byCode(Project([]*generators.Snapshot{placed("PM.1")}), "C10")
	if len(got) != 1 || got[0].Model != "PM.1" || got[0].Models != nil {
		t.Fatalf("%+v", got)
	}
}
