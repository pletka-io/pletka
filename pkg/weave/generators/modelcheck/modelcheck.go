// Package modelcheck reports modeling problems in a model as generator
// snapshots see it: counts that contradict each other, value types that do
// not fit where the path ends, links without targets, and branches or
// models that nothing tells apart. Exports such as SHACL depend on these
// being right; the checks let modelers see them without exporting.
//
// Codes follow the SHACL generator design's data checks:
//
//	C3  error    max below effective min (field, or a collection placement)
//	C4  error    value type does not fit the path's end
//	C5  error    two models share a class and no fixed value tells them apart
//	C6  error    two branches under one property share a class and no
//	             required fixed value tells them apart
//	C7  error    a collection's fields share no entry property
//	C8  error    a collection_model ref does not resolve
//	C9  warning  a Model or Collection field names no targets
//	C10 warning  a Concept field names no concept list
//	C13 note     a model is identified by its class alone
//
// Not checked: Required against Min (one rule since they are kept in sync),
// deprecated targets and empty concept lists (snapshots carry neither),
// duplicate paths (the path_collisions report), and node ids (snapshots
// assign them).
//
// Snapshots should be built with Options.IncludeReferenced, so collections
// that stubs name are checked and C8 can see what did not resolve.
package modelcheck

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// Severity grades a finding.
type Severity string

// Severities, from the design's check table.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Note    Severity = "note"
)

// Finding is one modeling problem, keyed by the design's check code.
type Finding struct {
	Code       string   `json:"code"`
	Severity   Severity `json:"severity"`
	Model      string   `json:"model,omitempty"`
	Collection string   `json:"collection,omitempty"`
	Field      string   `json:"field,omitempty"`
	Message    string   `json:"message"`
}

type checker struct {
	model string
	scope string // Model of findings being added: the model, or "" inside a referenced collection
	ns    generators.NamespaceSet
	out   []Finding
	root  *branchTree // the model's own paths, for its identity
}

// add records a finding once: a collection placed on the model and also
// named by a stub is checked from both sides. Findings inside a referenced
// collection carry no model, so a project reports them once.
func (c *checker) add(code string, sev Severity, collection, field, format string, args ...any) {
	f := Finding{Code: code, Severity: sev, Model: c.scope, Collection: collection, Field: field, Message: fmt.Sprintf(format, args...)}
	for _, o := range c.out {
		o.Model = f.Model
		if o == f {
			return
		}
	}
	c.out = append(c.out, f)
}

// norm makes set values comparable: a CURIE with a bound prefix becomes
// its IRI.
func (c *checker) norm(v string) string {
	if prefix, local, ok := strings.Cut(v, ":"); ok && !strings.HasPrefix(local, "//") {
		if base, bound := c.ns.ResolvePrefix(prefix); bound {
			return base + local
		}
	}
	return v
}

// stubValues are the fixed values a Collection stub's one collection gives
// its branch: required set values one step below the collection's anchor.
func (c *checker) stubValues(f domain.ResolvedField, snap *generators.Snapshot) map[string][]string {
	if !isStub(f) || len(f.CollectionModels) != 1 {
		return nil
	}
	rc, ok := snap.Referenced[f.CollectionModels[0].ID]
	if !ok || rc.Missing != "" {
		return nil
	}
	out := map[string][]string{}
	for _, m := range rc.Fields {
		v := strings.TrimSpace(m.SetValue)
		if m.IsHidden || v == "" || effMin(m.IsRequired, m.MinOccurs) < 1 {
			continue
		}
		tail := stripPrefix(m.PathElements, rc.SharedPathPrefix)
		if len(tail) != 2 || tail[0].Type != "property" {
			continue
		}
		p := tail[0].PrefixedName()
		if v = c.norm(v); !slices.Contains(out[p], v) {
			out[p] = append(out[p], v)
		}
	}
	return out
}

// Model checks one model snapshot.
func Model(snap *generators.Snapshot) []Finding {
	if snap == nil {
		return nil
	}
	return check(snap).out
}

func check(snap *generators.Snapshot) *checker {
	c := &checker{ns: snap.Namespaces}
	if snap.Model != nil {
		c.model = snap.Model.SemanticID
	}
	c.scope = c.model
	for _, d := range snap.Report.Errors {
		c.add("snapshot", Error, "", "", "%s: %s (the export refuses this model)", d.Code, d.Message)
	}

	root := newBranchTree(c.norm)
	placed := map[string]bool{}
	for _, f := range snap.Fields {
		root.add(fieldPath(f), f.Field, c.stubValues(f.Field, snap))
		c.fieldChecks(f.Field, fieldPath(f), f.CollectionSemanticID)
		if f.CollectionID != "" && !placed[f.CollectionID] {
			placed[f.CollectionID] = true
			if f.CollectionMaxOccurs != nil && *f.CollectionMaxOccurs < effMin(f.CollectionIsRequired, f.CollectionMinOccurs) {
				c.add("C3", Error, f.CollectionSemanticID, "", "%s is placed with max %d below its min %d",
					f.CollectionSemanticID, *f.CollectionMaxOccurs, effMin(f.CollectionIsRequired, f.CollectionMinOccurs))
			}
		}
	}
	c.entryChecks(snap.Tree)
	c.refChecks(snap)
	c.sameClassTwins(root, c.model, "")
	c.scope = ""
	for _, id := range slices.Sorted(maps.Keys(snap.Referenced)) {
		rc := snap.Referenced[id]
		collection := ""
		if rc.Collection != nil {
			collection = rc.Collection.SemanticID
		}
		members := newBranchTree(c.norm)
		for _, f := range rc.Fields {
			if f.IsHidden {
				continue
			}
			c.fieldChecks(f, f.PathElements, collection)
			c.unresolvedRefs(snap, f, collection)
			if !hiddenByGraft(snap.Grafts, f.SemanticID) {
				members.add(f.PathElements, f, c.stubValues(f, snap))
			}
		}
		c.sameClassTwins(members, collection, collection)
	}
	c.scope = c.model
	c.root = root
	if snap.Model != nil && len(root.values[""]) == 0 {
		c.add("C13", Note, "", "", "%s is identified by its class alone", c.model)
	}
	return c
}

// Project checks every model snapshot, then reports C5: models sharing a
// class that no required fixed value tells apart.
func Project(snaps []*generators.Snapshot) []Finding {
	type model struct {
		id     string
		values map[string][]string
	}
	var out []Finding
	byClass := map[string][]model{}
	var classes []string
	for _, snap := range snaps {
		if snap == nil {
			continue
		}
		c := check(snap)
		for _, f := range c.out {
			if !slices.Contains(out, f) { // collection findings repeat per model
				out = append(out, f)
			}
		}
		if snap.Model == nil {
			continue
		}
		cls := classKey(snap.Model.OntologyScope)
		if _, seen := byClass[cls]; !seen {
			classes = append(classes, cls)
		}
		byClass[cls] = append(byClass[cls], model{c.model, c.root.values[""]})
	}
	for _, cls := range classes {
		ms := byClass[cls]
		var clash []string
		for i := range ms {
			for j := range ms {
				if i != j && !valuesApart(ms[i].values, ms[j].values) {
					clash = append(clash, ms[i].id)
					break
				}
			}
		}
		if len(clash) > 1 {
			out = append(out, Finding{Code: "C5", Severity: Error, Model: clash[0],
				Message: fmt.Sprintf("%s share class %s and no required fixed value tells them apart", strings.Join(clash, ", "), cls)})
		}
	}
	return out
}

// classKey names a scope by its class and any additional types, sorted.
func classKey(scope domain.PathElement) string {
	extra := make([]string, 0, len(scope.AdditionalTypes))
	for _, t := range scope.AdditionalTypes {
		extra = append(extra, t.PrefixedName())
	}
	slices.Sort(extra)
	primary := scope.URI
	if scope.LocalName != "" {
		primary = scope.Prefix + ":" + scope.LocalName
	}
	return strings.Join(append([]string{primary}, extra...), ", ")
}

func isStub(f domain.ResolvedField) bool {
	switch valueType(f) {
	case "collection", "collection model":
		return true
	}
	return false
}

// stripPrefix drops the graft prefix a referenced collection's members share.
func stripPrefix(path, prefix []domain.PathElement) []domain.PathElement {
	if len(prefix) == 0 || len(path) <= len(prefix) {
		return path
	}
	for i := range prefix {
		if !path[i].SameStep(prefix[i]) {
			return path
		}
	}
	return path[len(prefix):]
}

// fieldChecks runs C3, C4, C9 and C10 on one field.
func (c *checker) fieldChecks(f domain.ResolvedField, path []domain.PathElement, collection string) {
	if min := effMin(f.IsRequired, f.MinOccurs); f.MaxOccurs != nil && *f.MaxOccurs < min {
		c.add("C3", Error, collection, f.SemanticID, "%s has max %d below its min %d", f.SemanticID, *f.MaxOccurs, min)
	}
	typ := valueType(f)
	if len(path) > 0 {
		end := path[len(path)-1]
		switch {
		case literalType(typ) && classEnd(end):
			c.add("C4", Error, collection, f.SemanticID, "%s is %s but its path ends at class %s", f.SemanticID, f.ExpectedValueType, end.PrefixedName())
		case iriType(typ) && !classEnd(end):
			c.add("C4", Error, collection, f.SemanticID, "%s is %s but its path does not end at a class (%s)", f.SemanticID, f.ExpectedValueType, end.PrefixedName())
		}
	}
	if isStub(f) && len(f.CollectionModels) == 0 {
		c.add("C9", Warning, collection, f.SemanticID, "%s is a Collection field without target collections", f.SemanticID)
	}
	switch typ {
	case "model", "reference model", "reference":
		if len(f.ResourceModels) == 0 {
			c.add("C9", Warning, collection, f.SemanticID, "%s is a Model field without target models", f.SemanticID)
		}
	case "concept", "domain value":
		if len(f.ConceptLists) == 0 {
			c.add("C10", Warning, collection, f.SemanticID, "%s is a Concept field without a concept list", f.SemanticID)
		}
	}
}

// fieldPath is a model field's path without its scope element.
func fieldPath(f generators.FieldNode) []domain.PathElement {
	out := make([]domain.PathElement, 0, len(f.Path))
	for _, pn := range f.Path {
		if pn.Role != generators.PathRoleScope {
			out = append(out, pn.Element)
		}
	}
	return out
}

// effMin is the stricter of Min and Required.
func effMin(required bool, min int) int {
	if required && min < 1 {
		return 1
	}
	return min
}

func valueType(f domain.ResolvedField) string {
	return strings.ToLower(strings.TrimSpace(f.ExpectedValueType))
}

func literalType(t string) bool {
	switch t {
	case "string", "date", "integer", "number", "decimal", "float", "double", "real", "numeric", "geojson":
		return true
	}
	return false
}

func iriType(t string) bool {
	switch t {
	case "concept", "domain value", "model", "reference model", "reference", "collection", "collection model":
		return true
	}
	return false
}

// classEnd reports a path ending at a class that is not an XSD datatype
// (stored paths type some datatypes as classes).
func classEnd(e domain.PathElement) bool {
	return e.Type == "class" && !isDatatype(e)
}

func isDatatype(e domain.PathElement) bool {
	return e.Prefix == "xsd" || strings.HasPrefix(e.URI, "http://www.w3.org/2001/XMLSchema#") ||
		e.PrefixedName() == "rdf:langString"
}

// refIDs splits refs into ids and display names (semantic id, else id).
func refIDs(refs []domain.EntityRef) (ids, names []string) {
	for _, r := range refs {
		ids = append(ids, r.ID)
		name := r.SemanticID
		if name == "" {
			name = r.ID
		}
		names = append(names, name)
	}
	return ids, names
}

// hiddenByGraft reports a member a graft override hides at some address;
// it is left out of its collection's C6 analysis.
func hiddenByGraft(grafts map[string]domain.GraftOverride, semanticID string) bool {
	for addr, g := range grafts {
		if g.IsHidden && strings.HasSuffix(addr, "/"+semanticID) {
			return true
		}
	}
	return false
}
