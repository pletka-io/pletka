package ontology

import (
	"sort"
	"strings"

	nsutil "github.com/pletka-io/pletka/pkg/namespace"
	parsedontology "github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

// ImportNamespacePlan is the manifest-independent namespace input used by
// import tooling before RDF parsing starts.
type ImportNamespacePlan struct {
	Shared     []NamespaceBinding
	Ontologies []ImportNamespaceOntology
}

// ImportNamespaceOntology is the namespace-bearing part of an ontology import
// manifest, independent of the CLI's YAML shape.
type ImportNamespaceOntology struct {
	Prefix    string
	Namespace string
	Versions  []NamespaceBinding
	Aliases   []NamespaceBinding
}

// BuildImportNamespaceBindings extracts the prefix/base-URI bindings that an
// import manifest declares before individual RDF files are parsed.
func BuildImportNamespaceBindings(plan ImportNamespacePlan) []NamespaceBinding {
	seen := map[string]string{} // namespace -> prefix; first declaration wins.
	add := func(prefix, namespace string) {
		prefix = strings.TrimSpace(prefix)
		namespace = strings.TrimSpace(namespace)
		if prefix == "" || namespace == "" {
			return
		}
		if _, ok := seen[namespace]; !ok {
			seen[namespace] = prefix
		}
	}
	for _, binding := range plan.Shared {
		add(binding.Prefix, binding.Namespace)
	}
	for _, ont := range plan.Ontologies {
		add(ont.Prefix, ont.Namespace)
		for _, binding := range ont.Versions {
			add(binding.Prefix, binding.Namespace)
		}
		for _, binding := range ont.Aliases {
			add(binding.Prefix, binding.Namespace)
		}
	}

	bindings := make([]NamespaceBinding, 0, len(seen))
	for namespace, prefix := range seen {
		bindings = append(bindings, NamespaceBinding{Prefix: prefix, Namespace: namespace})
	}
	return MergeNamespaceBindings(bindings)
}

// MergeNamespaceBindings returns unique namespace bindings ordered for longest
// namespace-prefix URI matching. Earlier entries win when multiple prefixes
// point to the same namespace.
func MergeNamespaceBindings(sets ...[]NamespaceBinding) []NamespaceBinding {
	seen := map[string]string{}
	weights := map[string]int64{}
	for _, set := range sets {
		for _, binding := range set {
			prefix := strings.TrimSpace(binding.Prefix)
			namespace := strings.TrimSpace(binding.Namespace)
			if prefix == "" || namespace == "" {
				continue
			}
			if _, ok := seen[namespace]; !ok {
				seen[namespace] = prefix
				weights[namespace] = binding.Weight
			}
		}
	}
	bindings := make([]NamespaceBinding, 0, len(seen))
	for namespace, prefix := range seen {
		bindings = append(bindings, NamespaceBinding{
			Prefix:    prefix,
			Namespace: namespace,
			Weight:    weights[namespace],
		})
	}
	sort.Slice(bindings, func(i, j int) bool {
		if len(bindings[i].Namespace) != len(bindings[j].Namespace) {
			return len(bindings[i].Namespace) > len(bindings[j].Namespace)
		}
		if bindings[i].Weight != bindings[j].Weight {
			return bindings[i].Weight > bindings[j].Weight
		}
		return bindings[i].Prefix < bindings[j].Prefix
	})
	return bindings
}

// NamespaceBindingsFromDeclarations converts RDF-declared namespaces into
// non-persisted import bindings used for qname resolution while importing.
func NamespaceBindingsFromDeclarations(declared []parsedontology.NamespaceDeclaration) []NamespaceBinding {
	bindings := make([]NamespaceBinding, 0, len(declared))
	for _, ns := range declared {
		prefix := strings.TrimSpace(ns.Prefix)
		namespace := strings.TrimSpace(ns.Namespace)
		if prefix == "" || namespace == "" {
			continue
		}
		bindings = append(bindings, NamespaceBinding{Prefix: prefix, Namespace: namespace})
	}
	return MergeNamespaceBindings(bindings)
}

// StandardNamespaceBindings returns the built-in RDF namespace bindings that
// should be present before ontology imports run.
func StandardNamespaceBindings() []NamespaceBinding {
	namespaces := nsutil.StandardNamespaces()
	bindings := make([]NamespaceBinding, 0, len(namespaces))
	for _, ns := range namespaces {
		bindings = append(bindings, NamespaceBinding{
			Prefix:    ns.Prefix,
			Namespace: ns.URI,
			Weight:    100,
			Source:    "system",
		})
	}
	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].Prefix < bindings[j].Prefix
	})
	return bindings
}

// BuildImportNamespaceManager creates a namespace resolver from canonical
// namespace bindings.
func BuildImportNamespaceManager(bindings []NamespaceBinding) nsutil.Manager {
	mgr := nsutil.NewStore()
	for _, binding := range bindings {
		prefix := strings.TrimSpace(binding.Prefix)
		namespace := strings.TrimSpace(binding.Namespace)
		if prefix == "" || namespace == "" {
			continue
		}
		_, _ = mgr.Put(prefix, namespace, 100)
	}
	return mgr
}

// NewImportRDFParser creates an RDF parser wired to import namespace bindings.
func NewImportRDFParser(bindings []NamespaceBinding, opts ...parsedontology.ParserOption) *parsedontology.RDFParser {
	parserOpts := make([]parsedontology.ParserOption, 0, len(opts)+1)
	parserOpts = append(parserOpts, parsedontology.WithNamespaceManager(BuildImportNamespaceManager(bindings)))
	parserOpts = append(parserOpts, opts...)
	return parsedontology.NewRDFParser(parserOpts...)
}
