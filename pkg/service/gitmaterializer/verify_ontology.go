package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
	"github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

// versionRef names one linked ontology version by prefix+version or by id.
type versionRef struct {
	Prefix    string
	Version   string
	VersionID string
}

func (r versionRef) String() string {
	if r.Prefix != "" {
		return r.Prefix + " " + r.Version
	}
	return "version id " + r.VersionID
}

// terminalMismatchKinds are expected value types a literal terminal cannot
// carry: the value is another entity, not a literal.
var terminalMismatchKinds = map[string]bool{"Model": true, "Collection": true, "Concept": true}

// checkOntology builds the autocomplete index from the vendored (or
// --ontologies) ontologies and replays every own field path through it.
func (v *verifyContext) checkOntology(ctx context.Context, opts VerifyOptions, rep *VerifyReport) {
	available := v.availableOntologies(opts)
	required := v.requiredVersions()
	ontologiesLine := yamlKeyLine(v.readRel("project.yaml"), "ontologies")
	if len(required) == 0 {
		v.add("project.yaml", ontologiesLine, "ontology-none", "no linked ontology versions (ontologies.linked_versions); path checks skipped")
		return
	}
	missing := 0
	var specs []*OntologySnapshot
	seen := map[*OntologySnapshot]bool{}
	for _, ref := range required {
		snap := available[ref.key()]
		if snap == nil {
			missing++
			v.add("project.yaml", ontologiesLine, "ontology-missing", "linked ontology %s is not vendored", ref)
			continue
		}
		if !seen[snap] {
			seen[snap] = true
			specs = append(specs, snap)
		}
	}
	if missing > 0 {
		v.add("project.yaml", ontologiesLine, "ontology-skipped", "path checks skipped: %d linked ontologies not vendored; run init-git --self-contained or pass --ontologies", missing)
		return
	}
	if defaults := highestWithPrefix(available, domain.DefaultsOntologyPrefix); defaults != nil && !seen[defaults] {
		specs = append(specs, defaults)
	}
	eng, expand, ok := v.buildOfflineEngine(ctx, specs)
	if !ok {
		return
	}
	for _, item := range v.snap.Entities.Fields {
		v.replayFieldFile(ctx, eng, expand, item, rep)
	}
}

func (r versionRef) key() string {
	if r.Prefix != "" {
		return r.Prefix + "@" + r.Version
	}
	return "id:" + r.VersionID
}

// requiredVersions lists the linked versions of the tree and of every loaded
// parent tree (the builder includes inherited ontologies).
func (v *verifyContext) requiredVersions() []versionRef {
	var out []versionRef
	add := func(ps *ProjectSnapshot) {
		if ps.Manifest.Ontologies == nil {
			return
		}
		for _, l := range ps.Manifest.Ontologies.LinkedVersions {
			out = append(out, versionRef{Prefix: l.Prefix, Version: l.Version, VersionID: l.OntologyVersionID})
		}
	}
	add(v.snap)
	for _, ps := range v.parents {
		add(ps)
	}
	return out
}

// availableOntologies indexes the vendored ontology snapshots (or those under
// opts.OntologiesDir) by prefix@version and by id:<version_id>.
func (v *verifyContext) availableOntologies(opts VerifyOptions) map[string]*OntologySnapshot {
	out := map[string]*OntologySnapshot{}
	index := func(snap *OntologySnapshot) {
		for _, p := range snap.Manifest.Ontology.Prefixes {
			out[p+"@"+snap.Manifest.Ontology.Version] = snap
		}
		if id := snap.Manifest.Ontology.VersionID; id != "" {
			out["id:"+id] = snap
		}
	}
	if opts.OntologiesDir == "" {
		for _, vo := range v.snap.Vendor.Ontologies {
			if vo.Snapshot != nil {
				v.vendored = append(v.vendored, vo)
				index(vo.Snapshot)
			}
		}
		return out
	}
	roots, err := findManifestRoots(opts.OntologiesDir, "ontology.yaml")
	if err != nil {
		v.add("project.yaml", 0, "ontology-load", "--ontologies %s: %v", opts.OntologiesDir, err)
		return out
	}
	for _, r := range roots {
		snap, err := loadOntologySnapshot(r.root)
		if err != nil {
			v.add("project.yaml", 0, "ontology-load", "--ontologies %s: %v", r.root, err)
			continue
		}
		v.vendored = append(v.vendored, VendoredOntologySnapshot{Module: r.module, Version: snap.Manifest.Ontology.Version, Snapshot: snap})
		index(snap)
	}
	return out
}

// highestWithPrefix returns the available snapshot with the given prefix and
// the lexically highest version, or nil.
func highestWithPrefix(available map[string]*OntologySnapshot, prefix string) *OntologySnapshot {
	var best *OntologySnapshot
	for key, snap := range available {
		if !strings.HasPrefix(key, prefix+"@") {
			continue
		}
		if best == nil || best.Manifest.Ontology.Version < snap.Manifest.Ontology.Version {
			best = snap
		}
	}
	return best
}

// buildOfflineEngine loads every snapshot into the in-memory store and builds
// the index with the tree's effective bindings plus every vendored namespace.
func (v *verifyContext) buildOfflineEngine(ctx context.Context, specs []*OntologySnapshot) (*autocomplete.OfflineEngine, prefixExpander, bool) {
	// The parser sees the same bindings the database's vendored import gets:
	// every vendored ontology's prefix bound to its namespace and aliases.
	// The tree's effective bindings stay out of the parser: a file's own
	// xmlns prefix (geo for CRMgeo) must not clash with a global one.
	pooled := poolVendoredOntologyBindings(nil, v.vendored)
	parserBindings := v.parserBindings()
	bindings := v.namespaceBindings(pooled)
	versions := make([]autocomplete.MemoryVersion, 0, len(specs))
	for _, snap := range specs {
		ont := snap.Manifest.Ontology
		manifestPath := relOrAbs(v.root, snap.RootDir) + "/ontology.yaml"
		if snap.Manifest.Sources == nil || snap.Manifest.Sources.Primary == "" {
			v.add(manifestPath, 0, "ontology-load", "no primary source file")
			return nil, nil, false
		}
		prefix := ont.Module
		if len(ont.Prefixes) > 0 {
			prefix = ont.Prefixes[0]
		}
		id := ont.OntologyID
		if id == "" {
			id = ont.Module
		}
		var companions []string
		for _, f := range snap.Manifest.Sources.Files {
			if f != snap.Manifest.Sources.Primary {
				companions = append(companions, f)
			}
		}
		mv, err := loadOfflineVersionRetrying(weaveontology.OfflineVersionSpec{
			Ontology:   &domain.Ontology{ID: id, Prefix: prefix, Namespace: ont.Namespace},
			VersionID:  prefix + "@" + ont.Version,
			Version:    ont.Version,
			BaseDir:    snap.RootDir,
			File:       snap.Manifest.Sources.Primary,
			Companions: companions,
			Bindings:   parserBindings,
		})
		if err != nil {
			v.add(manifestPath, 0, "ontology-load", "%v", err)
			return nil, nil, false
		}
		versions = append(versions, mv)
	}
	eng, err := autocomplete.NewOfflineEngine(ctx, autocomplete.NewMemoryStore(versions...), bindings, slog.Default())
	if err != nil {
		v.add("project.yaml", 0, "ontology-load", "%v", err)
		return nil, nil, false
	}
	byPrefix := map[string]string{}
	for _, b := range bindings {
		if _, dup := byPrefix[b.Prefix]; !dup {
			byPrefix[b.Prefix] = b.Namespace
		}
	}
	expand := snapshotPrefixExpanderWith(v.snap.Manifest.Namespaces, func(prefix string) (string, bool) {
		ns, ok := byPrefix[prefix]
		return ns, ok
	})
	return eng, expand, true
}

// globalBindingWeight is the weight of the tree's effective bindings in the
// parser set: below every vendored namespace, above a file's own declaration.
const globalBindingWeight = 100

// loadOfflineVersionRetrying parses the file; when its own xmlns prefix
// clashes with a pooled binding (geo for CRMgeo in one file, geo for
// geosparql elsewhere), the pooled bindings of that prefix that disagree
// with the declaration are dropped for this file and the parse retried:
// inside the file its declaration is authoritative, and the namespace it
// names keeps its own vendored prefix.
func loadOfflineVersionRetrying(spec weaveontology.OfflineVersionSpec) (autocomplete.MemoryVersion, error) {
	for attempt := 0; attempt < 8; attempt++ {
		mv, err := weaveontology.LoadOfflineVersion(spec)
		var conflict *rdf.PrefixConflictError
		if err == nil || !errors.As(err, &conflict) {
			return mv, err
		}
		kept := spec.Bindings[:0:0]
		dropped := false
		for _, b := range spec.Bindings {
			if b.Prefix == conflict.Prefix && b.Namespace != conflict.Declared {
				dropped = true
				continue
			}
			kept = append(kept, b)
		}
		if !dropped {
			return mv, err
		}
		spec.Bindings = kept
	}
	return weaveontology.LoadOfflineVersion(spec)
}

// parserBindings binds every vendored ontology's prefix to its namespace
// (weight 300) and to its aliases (weight 200, above a file's own declaration): the prefix resolves to the
// main namespace, while a file written against an alias still parses.
func (v *verifyContext) parserBindings() []weaveontology.NamespaceBinding {
	var out []weaveontology.NamespaceBinding
	// Vendored bindings come first: the merge keeps the first prefix seen
	// per namespace, and a tree's global bindings may name the same
	// namespace under another prefix (schemaorg beside sdo).
	for _, dep := range v.vendored {
		if dep.Snapshot == nil {
			continue
		}
		root := dep.Snapshot.Manifest.Ontology
		prefix := strings.TrimSpace(root.Slug)
		if len(root.Prefixes) > 0 && strings.TrimSpace(root.Prefixes[0]) != "" {
			prefix = strings.TrimSpace(root.Prefixes[0])
		}
		if prefix == "" || strings.TrimSpace(root.Namespace) == "" {
			continue
		}
		out = append(out, weaveontology.NamespaceBinding{Prefix: prefix, Namespace: strings.TrimSpace(root.Namespace), Weight: 300})
		for _, alias := range root.NamespaceAliases {
			if strings.TrimSpace(alias) != "" {
				out = append(out, weaveontology.NamespaceBinding{Prefix: prefix, Namespace: strings.TrimSpace(alias), Weight: 200})
			}
		}
	}
	// The tree's effective bindings stand in for the instance's global
	// bindings (weight 100, below everything vendored): a file written
	// against a namespace no vendored manifest lists still resolves.
	if v.snap.Manifest.Namespaces != nil {
		for _, b := range v.snap.Manifest.Namespaces.EffectiveBindings {
			if strings.TrimSpace(b.Prefix) != "" && strings.TrimSpace(b.Namespace) != "" {
				out = append(out, weaveontology.NamespaceBinding{Prefix: strings.TrimSpace(b.Prefix), Namespace: strings.TrimSpace(b.Namespace), Weight: globalBindingWeight})
			}
		}
	}
	return out
}

// namespaceBindings lists the tree's effective bindings first, then the
// pooled vendored bindings, for the index's qname resolution.
func (v *verifyContext) namespaceBindings(pooled []VendoredOntologyExternalBinding) []autocomplete.NamespaceBinding {
	out := make([]autocomplete.NamespaceBinding, 0, len(pooled))
	if v.snap.Manifest.Namespaces != nil {
		for _, b := range v.snap.Manifest.Namespaces.EffectiveBindings {
			out = append(out, autocomplete.NamespaceBinding{Prefix: b.Prefix, Namespace: b.Namespace})
		}
	}
	for _, b := range pooled {
		out = append(out, autocomplete.NamespaceBinding{Prefix: b.Prefix, Namespace: b.Namespace})
	}
	return out
}

// replayFieldFile decodes one own field and replays its path.
func (v *verifyContext) replayFieldFile(ctx context.Context, eng *autocomplete.OfflineEngine, expand prefixExpander, item SnapshotEntityFile, rep *VerifyReport) {
	if hasAdoptionMarker(item.Payload) {
		return
	}
	doc, err := decodeCanonicalYAML[canonicalFieldDoc](item.Payload)
	if err != nil {
		return // decode problems are structure findings
	}
	idLine := yamlKeyLine(item.Payload, "semantic_id")
	if len(doc.OntologyScope) == 0 {
		v.add(item.Path, idLine, "scope-missing", "field has no ontology_scope")
	}
	if len(doc.PathElements) == 0 {
		v.add(item.Path, idLine, "path-missing", "field has no path_elements")
	}
	if len(doc.OntologyScope) == 0 || len(doc.PathElements) == 0 {
		return
	}
	scopeLine := yamlKeyLine(item.Payload, "ontology_scope")
	pathLine := yamlKeyLine(item.Payload, "path_elements")
	scope, err := decodePathElementWith(doc.OntologyScope, "class", 0, expand)
	if err != nil {
		v.add(item.Path, scopeLine, pathErrorCode(err), "%v", err)
		return
	}
	elems, err := decodePathElementsWith(doc.PathElements, expand)
	if err != nil {
		v.add(item.Path, pathLine, pathErrorCode(err), "%v", err)
		return
	}
	if len(elems)%2 == 1 {
		v.add(item.Path, pathLine, "path-incomplete", "path ends on a property; the builder needs a class or literal after %s", elemQname(elems[len(elems)-1]))
	}
	if last := elems[len(elems)-1]; isLiteralPathElement(last) && terminalMismatchKinds[doc.ExpectedValueType] {
		v.add(item.Path, pathLine, "terminal-mismatch", "terminal %s is a literal but expected_value_type is %s", elemQname(last), doc.ExpectedValueType)
	}
	qnames := make([]string, 0, len(elems))
	uris := make([]string, 0, len(elems))
	for _, e := range elems {
		qnames = append(qnames, elemQname(e))
		uris = append(uris, e.URI)
	}
	misses, steps, err := autocomplete.ReplayPath(ctx, eng.Suggest, v.snap.Manifest.Project.ID, elemQname(scope), scope.URI, qnames, uris)
	if err != nil {
		v.add(item.Path, pathLine, "ontology-load", "%v", err)
		return
	}
	rep.Fields++
	rep.Steps += steps
	for _, m := range misses {
		line, where := pathLine, fmt.Sprintf("step %d", m.Position)
		if m.Position < 0 {
			line, where = scopeLine, "scope"
		}
		offered := m.Offered
		if len(offered) > 8 {
			offered = append(append([]string{}, offered[:8]...), "…")
		}
		v.add(item.Path, line, "not-offered", "%s: %s is not offered by the builder here (offered: %s)", where, m.Want, strings.Join(offered, ", "))
	}
}

func pathErrorCode(err error) string {
	if strings.Contains(err.Error(), "unbound prefix") {
		return "unbound-prefix"
	}
	return "decode"
}

func elemQname(pe domain.PathElement) string {
	return pe.Prefix + ":" + pe.LocalName
}

// relOrAbs renders p relative to root when it lies under it.
func relOrAbs(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return filepath.ToSlash(rel)
}
