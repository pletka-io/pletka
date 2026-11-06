package gitmaterializer

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
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
	eng, expand, ok := v.buildOfflineEngine(ctx, specs, available)
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
func (v *verifyContext) buildOfflineEngine(ctx context.Context, specs []*OntologySnapshot, available map[string]*OntologySnapshot) (*autocomplete.OfflineEngine, prefixExpander, bool) {
	bindings := v.namespaceBindings(available)
	parserBindings := make([]weaveontology.NamespaceBinding, 0, len(bindings))
	for _, b := range bindings {
		parserBindings = append(parserBindings, weaveontology.NamespaceBinding{Prefix: b.Prefix, Namespace: b.Namespace})
	}
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
		mv, err := weaveontology.LoadOfflineVersion(weaveontology.OfflineVersionSpec{
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

// namespaceBindings lists the tree's effective bindings first, then every
// vendored ontology's prefixes, sorted for a stable index.
func (v *verifyContext) namespaceBindings(available map[string]*OntologySnapshot) []autocomplete.NamespaceBinding {
	var out []autocomplete.NamespaceBinding
	if v.snap.Manifest.Namespaces != nil {
		for _, b := range v.snap.Manifest.Namespaces.EffectiveBindings {
			out = append(out, autocomplete.NamespaceBinding{Prefix: b.Prefix, Namespace: b.Namespace})
		}
	}
	keys := make([]string, 0, len(available))
	for k := range available {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := map[*OntologySnapshot]bool{}
	for _, k := range keys {
		snap := available[k]
		if seen[snap] || snap.Manifest.Ontology.Namespace == "" {
			continue
		}
		seen[snap] = true
		for _, p := range snap.Manifest.Ontology.Prefixes {
			out = append(out, autocomplete.NamespaceBinding{Prefix: p, Namespace: snap.Manifest.Ontology.Namespace})
		}
	}
	return out
}

// replayFieldFile decodes one own field and replays its path.
func (v *verifyContext) replayFieldFile(ctx context.Context, eng *autocomplete.OfflineEngine, expand prefixExpander, item SnapshotEntityFile, rep *VerifyReport) {
	if hasAdoptionMarker(item.Payload) {
		return
	}
	doc, err := decodeCanonicalYAML[canonicalFieldDoc](item.Payload)
	if err != nil || len(doc.OntologyScope) == 0 || len(doc.PathElements) == 0 {
		return // decode problems are structure findings; a field without a path has nothing to replay
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
