// Package autocomplete provides path-builder suggestions and ontology-label
// lookups backed by the master ontology slice's Store. It replaces the
// older regex-heuristic autocomplete implementation and its in-memory cache
// layer.
//
// All type detection (class vs property vs literal) comes from Store
// lookups — the polymorphic weave_ontology_relations table + the
// classes / properties tables. Regex-based classifiers are gone.
package autocomplete

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	nsutil "github.com/pletka-io/pletka/pkg/namespace"
)

// Store is the read surface autocomplete needs. It's a sub-set of the
// pkg/weave/ontology.Store interface — declared here so the subpackage
// can be tested with a fake.
type Store interface {
	GetVersion(ctx context.Context, id string) (*domain.OntologyVersion, error)
	GetOntology(ctx context.Context, id string) (*domain.Ontology, error)

	ListClassesByVersion(ctx context.Context, versionID string) ([]*domain.OntologyClass, error)
	ListPropertiesByVersion(ctx context.Context, versionID string) ([]*domain.OntologyProperty, error)
	GetClassByQname(ctx context.Context, versionID, qname string) (*domain.OntologyClass, error)
	GetPropertyByQname(ctx context.Context, versionID, qname string) (*domain.OntologyProperty, error)
	SearchClasses(ctx context.Context, versionID, query string, limit int) ([]*domain.OntologyClass, error)
	SearchProperties(ctx context.Context, versionID, query string, limit int) ([]*domain.OntologyProperty, error)
	PropertiesForDomainQname(ctx context.Context, versionID, qname string) ([]*domain.OntologyProperty, error)
	// ListSubclassesByQname returns the direct subclasses of parentQname.
	// Caller walks breadth-first for transitive coverage.
	ListSubclassesByQname(ctx context.Context, versionID, parentQname string) ([]*domain.OntologyClass, error)

	ListRelationsForSourceByType(ctx context.Context, sourceID, sourceKind, relType string) ([]*domain.OntologyRelation, error)
}

// ProjectReader resolves a project's selected ontology-version IDs,
// optionally walking inherited links from parent projects.
type ProjectReader interface {
	ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error)
}

// DirectEngine composes Store + ProjectReader into the autocomplete surface.
type DirectEngine struct {
	store             Store
	projects          ProjectReader
	namespaceBindings NamespaceBindingsFunc
}

// NewDirect constructs a DirectEngine. Both deps are required.
func NewDirect(store Store, projects ProjectReader) *DirectEngine {
	return NewDirectWithNamespaceBindings(store, projects, nil)
}

// NewDirectWithNamespaceBindings constructs a DirectEngine with runtime namespace
// bindings for resolving full URI literal ranges.
func NewDirectWithNamespaceBindings(store Store, projects ProjectReader, namespaceBindings NamespaceBindingsFunc) *DirectEngine {
	return &DirectEngine{store: store, projects: projects, namespaceBindings: namespaceBindings}
}

// ----------------------------------------------------------------------------
// Public types
// ----------------------------------------------------------------------------

// Request is the input to GetSuggestions. CurrentPath is the path the
// user has built so far (qnames). Query is the in-progress text.
type Request struct {
	ProjectID               string   `json:"project_id"`
	VersionID               string   `json:"version_id,omitempty"` // optional pin to a specific version
	CurrentPath             []string `json:"current_path"`         // qnames
	Query                   string   `json:"query"`
	MaxResults              int      `json:"max_results"`
	IncludeInverse          bool     `json:"include_inverse"`
	IncludeParentProjects   bool     `json:"include_parent_projects"`
	IncludeRangeSuggestions bool     `json:"include_range_suggestions"`
	AllowManualComplete     bool     `json:"allow_manual_complete"`
	ScopeClass              string   `json:"scope_class,omitempty"` // root class qname for first-step suggestions

	// ScopeAdditionalClasses lists secondary scope class qnames for
	// multi-class entities (e.g. crmdig:D1 + crm:E36). When the current
	// path stands at the scope position, property suggestions are
	// unioned across primary scope and these classes' lineages so the
	// user can pick properties defined on either class.
	ScopeAdditionalClasses []string `json:"scope_additional_classes,omitempty"`

	// Source carries the per-request engine override value. Only "direct"
	// is meaningful; other values are ignored.
	// ponytail: two scalar fields beat threading an auth interface into the engine.
	Source string `json:"source,omitempty"`
	// SuperAdmin is set by the handler from request auth. When true and the
	// DispatchConfig allows per-request overrides, Source=="direct" forces
	// the DirectEngine for this request.
	SuperAdmin bool `json:"-"`
}

// Suggestion is one entry in the autocomplete result.
type Suggestion struct {
	URI       string              `json:"uri"`
	Qname     string              `json:"qname"`
	Prefix    string              `json:"prefix"`
	LocalName string              `json:"local_name"`
	Label     domain.Translations `json:"label,omitempty"`
	Comment   domain.Translations `json:"comment,omitempty"`
	Type      string              `json:"type"` // "class" | "property" | "literal"

	DomainClasses []string `json:"domain_classes,omitempty"`
	RangeClasses  []string `json:"range_classes,omitempty"`

	OntologyPrefix  string `json:"ontology_prefix,omitempty"`
	OntologyVersion string `json:"ontology_version,omitempty"`

	IsInverse bool    `json:"is_inverse,omitempty"`
	Relevance float64 `json:"relevance,omitempty"`

	// Datatype is set when Type == "literal" and carries the canonical
	// xsd:* / rdfs:Literal qname the path element should persist. Pulled
	// from the property's range relation when the engine emits a
	// synthetic literal suggestion (see suggestRangeForProperty).
	// Frontend's suggestionToElement copies it onto PathElement.Datatype.
	Datatype string `json:"datatype,omitempty"`
}

// OntologyLabelsResult is the response shape from OntologyLabels.
// Matches the legacy result so the frontend client.ts works unchanged.
type OntologyLabelsResult struct {
	ClassLabels    map[string]string `json:"class_labels"`
	PropertyLabels map[string]string `json:"property_labels"`
	Prefixes       map[string]string `json:"prefixes"`
	FullNames      map[string]string `json:"full_names"`
}

// ----------------------------------------------------------------------------
// GetSuggestions
// ----------------------------------------------------------------------------

// GetSuggestions returns autocomplete entries appropriate for the next
// path step. Logic is purely Store-driven:
//
//   - empty CurrentPath → suggest classes (filtered by ScopeClass if set)
//   - last element is a class → suggest properties whose domain includes
//     that class
//   - last element is a property → suggest classes from that property's
//     range
//
// Type detection is by Store lookup, NOT regex. If a step's qname is
// found in weave_ontology_classes it's a class; in
// weave_ontology_properties it's a property; otherwise unknown.
func (e *DirectEngine) GetSuggestions(ctx context.Context, req Request) ([]Suggestion, error) {
	if req.MaxResults <= 0 {
		req.MaxResults = 50
	}

	versionIDs, err := e.resolveVersions(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(versionIDs) == 0 {
		return nil, nil
	}

	// First step.
	if len(req.CurrentPath) == 0 {
		return e.suggestRootClasses(ctx, versionIDs, req)
	}

	// Subsequent steps. Look up the type of the last element.
	last := strings.TrimSpace(req.CurrentPath[len(req.CurrentPath)-1])
	if last == "" {
		return nil, nil
	}

	// A genuine not-found (nil, nil) in one version falls through to the
	// property lookup and the next version (cross-version resolution).
	// A real DB error short-circuits with that error rather than masking it:
	// a transient failure in one version is ambiguous, so we fail loud instead
	// of silently returning suggestions resolved from the remaining versions.
	for _, vID := range versionIDs {
		// Try class first (faster path-builder UX uses qname-form).
		c, err := e.store.GetClassByQname(ctx, vID, last)
		if err != nil {
			return nil, fmt.Errorf("get class %s: %w", last, err)
		}
		if c != nil {
			return e.suggestPropertiesForClass(ctx, versionIDs, c, req)
		}
		p, err := e.store.GetPropertyByQname(ctx, vID, last)
		if err != nil {
			return nil, fmt.Errorf("get property %s: %w", last, err)
		}
		if p != nil {
			return e.suggestRangeForProperty(ctx, versionIDs, p, req)
		}
	}

	// Unknown last element — return empty rather than fall-through to a
	// regex guess.
	return nil, nil
}

func (e *DirectEngine) suggestRootClasses(ctx context.Context, versionIDs []string, req Request) ([]Suggestion, error) {
	out := make([]Suggestion, 0)
	seen := make(map[string]struct{})

	for _, vID := range versionIDs {
		// Look up parent ontology for prefix/version metadata used in
		// the suggestion's OntologyInfo block.
		ontPrefix, versionString := e.metaFor(ctx, vID)

		var classes []*domain.OntologyClass
		var err error
		if strings.TrimSpace(req.Query) != "" {
			classes, err = e.store.SearchClasses(ctx, vID, req.Query, req.MaxResults)
		} else {
			classes, err = e.store.ListClassesByVersion(ctx, vID)
		}
		if err != nil {
			return nil, fmt.Errorf("list classes: %w", err)
		}

		for _, c := range classes {
			if _, dup := seen[c.Qname]; dup {
				continue
			}
			seen[c.Qname] = struct{}{}
			out = append(out, classToSuggestion(c, ontPrefix, versionString))
			if len(out) >= req.MaxResults {
				return out, nil
			}
		}
	}
	return out, nil
}

func (e *DirectEngine) suggestPropertiesForClass(ctx context.Context, versionIDs []string, class *domain.OntologyClass, req Request) ([]Suggestion, error) {
	out := make([]Suggestion, 0)
	seen := make(map[string]struct{})
	q := strings.TrimSpace(req.Query)

	// At the scope position (path length 1), union with additional scope
	// classes so multi-class entities (D1 + E36) get properties of both.
	additional := atScope(req)

	// Lineage is computed once across the full project-union of linked
	// versions — superclass edges that cross ontology boundaries (e.g.
	// aaao:ZE19 → crm:E13) only resolve when classLineageQnames sees all
	// versions, so per-version lineage would silently truncate the chain
	// and lose every ancestor property.
	domainQnames, err := e.classLineageQnames(ctx, versionIDs, class.Qname)
	if err != nil {
		return nil, fmt.Errorf("class lineage: %w", err)
	}
	for _, extra := range additional {
		extraQnames, err := e.classLineageQnames(ctx, versionIDs, extra)
		if err != nil {
			return nil, fmt.Errorf("additional class lineage %s: %w", extra, err)
		}
		domainQnames = appendUnique(domainQnames, extraQnames)
	}

	for _, vID := range versionIDs {
		ontPrefix, versionString := e.metaFor(ctx, vID)

		for _, domainQname := range domainQnames {
			props, err := e.store.PropertiesForDomainQname(ctx, vID, domainQname)
			if err != nil {
				return nil, fmt.Errorf("properties for domain: %w", err)
			}
			for _, p := range props {
				if _, dup := seen[p.Qname]; dup {
					continue
				}
				if q != "" && !matchesQuery(p.LocalName, p.Label, q) {
					continue
				}
				seen[p.Qname] = struct{}{}
				out = append(out, propertyToSuggestion(p, ontPrefix, versionString, false))
				if len(out) >= req.MaxResults {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

// classLineageQnames returns the qname chain from `qname` up to the
// roots, traversing subclass_of edges across **every linked ontology
// version**. Cross-version chains are real: AAAo's ZE19_Naming declares
// crm:E13_Attribute_Assignment as its parent via a subclass_of relation
// whose target is defined in a sibling CRM version. Walking only one
// version breaks the chain at that boundary, so the caller would miss
// every property whose domain lives further up in the cross-ontology
// hierarchy.
//
// A qname can also have DISTINCT rows in different linked versions (e.g.
// crm:E4_Period has a CRM 7.1.3 row declaring parents E2+E92, and a
// crmgeo 1.2.1 row declaring parent crmgeo:SP1_Phenomenal_Spacetime_Volume).
// Walking only the first row drops cross-ontology parents — the SP1 branch
// is missed, and with it every property whose domain is SP1 or its
// ancestors. The fix: for each queued qname, iterate over
// ALL linked versions and union their subclass_of edges.
//
// Dangling parents (no version has the class) still get recorded in the
// output so PropertiesForDomainQname can try them by qname, but the walk
// stops there.
func (e *DirectEngine) classLineageQnames(ctx context.Context, versionIDs []string, qname string) ([]string, error) {
	out := make([]string, 0, 8)
	seen := map[string]struct{}{}
	queue := []string{qname}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if _, dup := seen[cur]; dup {
			continue
		}
		seen[cur] = struct{}{}
		out = append(out, cur)

		// Gather subclass_of parents from EVERY linked version-row of cur.
		// A qname can have distinct rows per version (e.g. crm:E4_Period in
		// CRM vs crmgeo), each declaring its own subclass_of edges. Walking
		// only the first row drops cross-ontology parents.
		for _, vID := range versionIDs {
			c, err := e.store.GetClassByQname(ctx, vID, cur)
			if err != nil {
				return nil, fmt.Errorf("get class %s: %w", cur, err)
			}
			if c == nil {
				continue // not in this version — try the next
			}
			rels, err := e.store.ListRelationsForSourceByType(ctx, c.ID, "class", "subclass_of")
			if err != nil {
				return nil, fmt.Errorf("subclass relations for %s: %w", cur, err)
			}
			for _, rel := range rels {
				if rel == nil || rel.TargetQname == "" {
					continue
				}
				if _, dup := seen[rel.TargetQname]; !dup {
					queue = append(queue, rel.TargetQname)
				}
			}
		}
	}

	return out, nil
}

func (e *DirectEngine) suggestRangeForProperty(ctx context.Context, versionIDs []string, prop *domain.OntologyProperty, req Request) ([]Suggestion, error) {
	out := make([]Suggestion, 0)
	seen := make(map[string]struct{})
	q := strings.TrimSpace(req.Query)

	// Walk relations to find range qnames for this property.
	rels, err := e.store.ListRelationsForSourceByType(ctx, prop.ID, "property", "range")
	if err != nil {
		return nil, fmt.Errorf("range relations: %w", err)
	}

	isDatatypeProperty := prop.PropertyType == "DatatypeProperty"
	literals := qnameResolver{namespaceBindings: e.namespaceBindings}

	for _, r := range rels {
		// Look the qname up in any version we've got — first hit wins.
		var found *domain.OntologyClass
		var foundVersion string
		for _, vID := range versionIDs {
			c, err := e.store.GetClassByQname(ctx, vID, r.TargetQname)
			if err != nil {
				return nil, fmt.Errorf("get class %s: %w", r.TargetQname, err)
			}
			if c != nil {
				found = c
				foundVersion = vID
				break
			}
		}
		if found != nil {
			ontPrefix, versionString := e.metaFor(ctx, foundVersion)

			if _, dup := seen[found.Qname]; !dup {
				seen[found.Qname] = struct{}{}
				if q == "" || matchesQuery(found.LocalName, found.Label, q) {
					out = append(out, classToSuggestion(found, ontPrefix, versionString))
					if len(out) >= req.MaxResults {
						return out, nil
					}
				}
			}

			// Walk subclasses of the range class so curators can pick a
			// more specific node where appropriate. P1's range is E41,
			// but E33_E41_Linguistic_Appellation also fits (it's a
			// subclass of E41). Without this, the dropdown only ever
			// surfaces the direct range.
			subs, err := e.collectSubclasses(ctx, versionIDs, found.Qname, seen)
			if err != nil {
				return nil, fmt.Errorf("collect subclasses for %s: %w", found.Qname, err)
			}
			for _, sub := range subs {
				if q != "" && !matchesQuery(sub.LocalName, sub.Label, q) {
					continue
				}
				out = append(out, classToSuggestion(sub, ontPrefix, versionString))
				if len(out) >= req.MaxResults {
					return out, nil
				}
			}

			continue
		}

		// Range target is not in our class store. For DatatypeProperty,
		// that's expected — the target is a literal type (xsd:string,
		// rdfs:Literal, etc.). Synthesise a literal suggestion using
		// the canonical qname so the path can terminate cleanly. The
		// engine never needs a hardcoded literal-name list — the
		// ontology's PropertyType + range relation already say "this
		// terminates in a literal".
		if isDatatypeProperty {
			s, err := literals.literalSuggestion(ctx, r.TargetQname)
			if err != nil {
				return nil, err
			}
			if _, dup := seen[s.Qname]; dup {
				continue
			}
			if q != "" && !matchesQuery(s.LocalName, s.Label, q) {
				continue
			}
			seen[s.Qname] = struct{}{}
			out = append(out, s)
			if len(out) >= req.MaxResults {
				return out, nil
			}
		}
	}

	// DatatypeProperty with no explicit range relation (or all ranges
	// pointed at classes we don't have): still let the path terminate
	// by emitting a generic rdf:literal.
	if isDatatypeProperty && len(out) == 0 {
		out = append(out, literalSuggestionFromQname("rdf:literal"))
	}

	return out, nil
}

// collectSubclasses returns the transitive subclasses of parentQname
// across every linked ontology version, breadth-first. Walking the
// project-union (rather than a single version) is what lets a curator
// pick e.g. aaao:ZE19_Naming as a more-specific node for a property
// whose range crm:E13_Attribute_Assignment is defined in CRM.
//
// seen is updated as the walk proceeds so the caller does not
// double-emit when subclasses surface from multiple parent ranges. A
// subclass that is already in seen is skipped (its descendants too —
// they will have been queued via that other parent if reachable).
//
// The walk is bounded by the qname-level dedupe map; CRM-scale
// ontologies have a few hundred classes total so a full traversal
// across N linked versions stays cheap (one round-trip per version
// per BFS step).
func (e *DirectEngine) collectSubclasses(ctx context.Context, versionIDs []string, parentQname string, seen map[string]struct{}) ([]*domain.OntologyClass, error) {
	out := make([]*domain.OntologyClass, 0)
	queue := []string{parentQname}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, vID := range versionIDs {
			subs, err := e.store.ListSubclassesByQname(ctx, vID, current)
			if err != nil {
				return nil, err
			}
			for _, sub := range subs {
				if sub == nil {
					continue
				}
				if _, dup := seen[sub.Qname]; dup {
					continue
				}
				seen[sub.Qname] = struct{}{}
				out = append(out, sub)
				queue = append(queue, sub.Qname)
			}
		}
	}
	return out, nil
}

// NamespaceBinding is one runtime prefix-to-namespace binding used by
// autocomplete to resolve full URI relation targets without importing the
// parent ontology package.
type NamespaceBinding struct {
	Prefix    string
	Namespace string
}

// NamespaceBindingsFunc returns runtime namespace bindings ordered from most
// specific to least specific namespace.
type NamespaceBindingsFunc func(context.Context) ([]NamespaceBinding, error)

type qnameResolver struct {
	namespaceBindings NamespaceBindingsFunc
	manager           nsutil.Manager
}

func (r *qnameResolver) literalSuggestion(ctx context.Context, target string) (Suggestion, error) {
	qname, err := r.resolve(ctx, target)
	if err != nil {
		return Suggestion{}, err
	}
	return literalSuggestionFromQname(qname), nil
}

func (r *qnameResolver) resolve(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" || !isFullURI(target) {
		return target, nil
	}

	mgr, err := r.namespaceManager(ctx)
	if err != nil {
		return "", err
	}
	base, local := nsutil.SplitURI(target)
	if base == "" || local == "" {
		return "", fmt.Errorf("autocomplete: cannot split literal range URI %q", target)
	}
	ns, err := mgr.GetWithBase(base)
	if err != nil {
		return "", fmt.Errorf("autocomplete: missing namespace binding for literal range URI %q: %w", target, err)
	}
	return ns.Prefix + ":" + local, nil
}

func (r *qnameResolver) namespaceManager(ctx context.Context) (nsutil.Manager, error) {
	if r.manager != nil {
		return r.manager, nil
	}
	mgr := nsutil.NewDefaultManager()
	if r.namespaceBindings != nil {
		bindings, err := r.namespaceBindings(ctx)
		if err != nil {
			return nil, fmt.Errorf("autocomplete: load namespace bindings: %w", err)
		}
		for i, binding := range bindings {
			if binding.Prefix == "" || binding.Namespace == "" {
				continue
			}
			_, _ = mgr.Put(binding.Prefix, binding.Namespace, len(bindings)-i)
		}
	}
	r.manager = mgr
	return mgr, nil
}

func normalizeRelationQname(ctx context.Context, resolver *qnameResolver, target string) (string, error) {
	if resolver == nil {
		resolver = &qnameResolver{}
	}
	return resolver.resolve(ctx, target)
}

func isFullURI(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

// literalSuggestionFromQname builds a synthetic Suggestion of type "literal"
// from a canonical qname such as xsd:string or rdfs:Literal.
func literalSuggestionFromQname(qname string) Suggestion {
	prefix, local, ok := strings.Cut(qname, ":")
	if !ok {
		prefix = ""
		local = qname
	}
	return Suggestion{
		URI:       qname,
		Qname:     qname,
		Prefix:    prefix,
		LocalName: local,
		Type:      "literal",
		Datatype:  qname,
		Label:     domain.Translations{"en": "Literal value (" + qname + ")"},
		Comment:   domain.Translations{"en": "Path terminates in a literal value of type " + qname + "."},
	}
}

// ----------------------------------------------------------------------------
// OntologyLabels
// ----------------------------------------------------------------------------

// OntologyLabels assembles the label maps used by the frontend's
// schema-driven renderers (path display, breadcrumbs, …).
//
// Walks every class + property across the project's selected versions
// (own + inherited) and produces:
//
//   - ClassLabels[local_name]    → label in lang (or empty)
//   - PropertyLabels[local_name] → label in lang (or empty)
//   - Prefixes[short_code]       → prefix (e.g. "E33" → "crm")
//   - FullNames[short_code]      → full local name (e.g. "P1" → "P1_is_identified_by")
func (e *DirectEngine) OntologyLabels(ctx context.Context, projectID, lang string) (*OntologyLabelsResult, error) {
	if lang == "" {
		lang = "en"
	}
	out := &OntologyLabelsResult{
		ClassLabels:    map[string]string{},
		PropertyLabels: map[string]string{},
		Prefixes:       map[string]string{},
		FullNames:      map[string]string{},
	}

	resolved, err := e.projects.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		return nil, err
	}

	for _, r := range resolved {
		vID := r.Link.OntologyVersionID
		classes, err := e.store.ListClassesByVersion(ctx, vID)
		if err != nil {
			return nil, fmt.Errorf("list classes for labels: %w", err)
		}
		for _, c := range classes {
			out.ClassLabels[c.LocalName] = c.Label.Get(lang)
			short := shortCode(c.LocalName)
			if short != "" {
				out.Prefixes[short] = c.Prefix
				out.FullNames[short] = c.LocalName
			}
		}
		props, err := e.store.ListPropertiesByVersion(ctx, vID)
		if err != nil {
			return nil, fmt.Errorf("list properties for labels: %w", err)
		}
		for _, p := range props {
			out.PropertyLabels[p.LocalName] = p.Label.Get(lang)
			short := shortCode(p.LocalName)
			if short != "" {
				out.Prefixes[short] = p.Prefix
				out.FullNames[short] = p.LocalName
			}
		}
	}
	return out, nil
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func (e *DirectEngine) resolveVersions(ctx context.Context, req Request) ([]string, error) {
	if req.VersionID != "" {
		return []string{req.VersionID}, nil
	}
	if req.ProjectID == "" {
		return nil, nil
	}
	resolved, err := e.projects.ResolvedOntologyVersions(ctx, req.ProjectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(resolved))
	for _, r := range resolved {
		// Skip inherited entries when the caller didn't ask for them.
		if r.SourceProjectID != "" && !req.IncludeParentProjects {
			continue
		}
		out = append(out, r.Link.OntologyVersionID)
	}
	return out, nil
}

// metaFor resolves the ontology prefix + version string for a version ID,
// used to populate a suggestion's OntologyInfo block. A lookup failure
// degrades to blank Meta (cosmetic, non-load-bearing) but is logged rather
// than silently swallowed.
func (e *DirectEngine) metaFor(ctx context.Context, versionID string) (ontPrefix, versionString string) {
	v, err := e.store.GetVersion(ctx, versionID)
	if err != nil {
		slog.Warn("autocomplete: version lookup failed", "version_id", versionID, "err", err)
		return "", ""
	}
	if v == nil {
		return "", ""
	}
	versionString = v.VersionString
	o, err := e.store.GetOntology(ctx, v.OntologyID)
	if err != nil {
		slog.Warn("autocomplete: ontology lookup failed", "version_id", versionID, "ontology_id", v.OntologyID, "err", err)
		return "", versionString
	}
	if o != nil {
		ontPrefix = o.Prefix
	}
	return ontPrefix, versionString
}

func classToSuggestion(c *domain.OntologyClass, ontPrefix, versionString string) Suggestion {
	return Suggestion{
		URI:             c.URI,
		Qname:           c.Qname,
		Prefix:          c.Prefix,
		LocalName:       c.LocalName,
		Label:           c.Label,
		Comment:         c.Comment,
		Type:            "class",
		OntologyPrefix:  ontPrefix,
		OntologyVersion: versionString,
	}
}

func propertyToSuggestion(p *domain.OntologyProperty, ontPrefix, versionString string, isInverse bool) Suggestion {
	return Suggestion{
		URI:             p.URI,
		Qname:           p.Qname,
		Prefix:          p.Prefix,
		LocalName:       p.LocalName,
		Label:           p.Label,
		Comment:         p.Comment,
		Type:            "property",
		OntologyPrefix:  ontPrefix,
		OntologyVersion: versionString,
		IsInverse:       isInverse,
	}
}

// matchesQuery does a case-insensitive substring match against either
// the local name or any label translation.
func matchesQuery(localName string, labels domain.Translations, query string) bool {
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(localName), q) {
		return true
	}
	for _, v := range labels {
		if strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

// shortCode delegates to domain.DeriveClassCode. Kept as a local alias
// so call sites stay readable.
func shortCode(localName string) string { return domain.DeriveClassCode(localName) }

// atScope returns the additional scope class qnames to union when the
// current path stands at the scope position (one step in — i.e. the
// scope class itself). Returns nil otherwise so deeper path steps are
// not affected by multi-class scope.
func atScope(req Request) []string {
	if len(req.CurrentPath) != 1 || len(req.ScopeAdditionalClasses) == 0 {
		return nil
	}
	out := make([]string, 0, len(req.ScopeAdditionalClasses))
	for _, q := range req.ScopeAdditionalClasses {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		out = append(out, q)
	}
	return out
}

// appendUnique appends qnames from extra into base, skipping duplicates.
func appendUnique(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base))
	for _, q := range base {
		seen[q] = struct{}{}
	}
	for _, q := range extra {
		if _, ok := seen[q]; ok {
			continue
		}
		seen[q] = struct{}{}
		base = append(base, q)
	}
	return base
}

// SortSuggestions deterministically orders suggestions by Type then
// LocalName. Useful so callers can rely on a stable result order.
func SortSuggestions(s []Suggestion) {
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].Type != s[j].Type {
			return s[i].Type < s[j].Type
		}
		return s[i].LocalName < s[j].LocalName
	})
}
