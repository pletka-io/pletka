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
	out   []Finding
}

func (c *checker) add(code string, sev Severity, collection, field, format string, args ...any) {
	c.out = append(c.out, Finding{Code: code, Severity: sev, Model: c.model, Collection: collection, Field: field, Message: fmt.Sprintf(format, args...)})
}

// Model checks one model snapshot.
func Model(snap *generators.Snapshot) []Finding {
	if snap == nil {
		return nil
	}
	c := &checker{}
	if snap.Model != nil {
		c.model = snap.Model.SemanticID
	}

	root := newBranchTree()
	placed := map[string]bool{}
	for _, f := range snap.Fields {
		root.add(fieldPath(f), f.Field)
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
	for _, id := range slices.Sorted(maps.Keys(snap.Referenced)) {
		rc := snap.Referenced[id]
		collection := ""
		if rc.Collection != nil {
			collection = rc.Collection.SemanticID
		}
		members := newBranchTree()
		for _, f := range rc.Fields {
			if f.IsHidden {
				continue
			}
			c.fieldChecks(f, f.PathElements, collection)
			members.add(f.PathElements, f)
		}
		c.sameClassTwins(members, collection, collection)
	}
	c.sameClassTwins(root, c.model, "")
	return c.out
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
	switch typ {
	case "model", "reference model", "reference":
		if len(f.ResourceModels) == 0 {
			c.add("C9", Warning, collection, f.SemanticID, "%s is a Model field without target models", f.SemanticID)
		}
	case "collection", "collection model":
		if len(f.CollectionModels) == 0 {
			c.add("C9", Warning, collection, f.SemanticID, "%s is a Collection field without target collections", f.SemanticID)
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
