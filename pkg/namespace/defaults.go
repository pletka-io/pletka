package namespace

var standardNamespaces = []struct {
	prefix string
	uri    string
}{
	{"rdf", "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
	{"rdfs", "http://www.w3.org/2000/01/rdf-schema#"},
	{"owl", "http://www.w3.org/2002/07/owl#"},
	{"xsd", "http://www.w3.org/2001/XMLSchema#"},
	{"skos", "http://www.w3.org/2004/02/skos/core#"},
	{"dc", "http://purl.org/dc/elements/1.1/"},
	{"dcterms", "http://purl.org/dc/terms/"},
	{"foaf", "http://xmlns.com/foaf/0.1/"},
	{"schema", "http://schema.org/"},
}

// StandardNamespace is one built-in namespace binding required by common RDF
// tooling. These bindings are seed data; runtime renderers should read
// namespace bindings from their snapshot or store.
type StandardNamespace struct {
	Prefix string
	URI    string
}

func StandardNamespaces() []StandardNamespace {
	out := make([]StandardNamespace, 0, len(standardNamespaces))
	for _, ns := range standardNamespaces {
		out = append(out, StandardNamespace{
			Prefix: ns.prefix,
			URI:    ns.uri,
		})
	}
	return out
}

func NewDefaultManager() Manager {
	mgr := NewStore()
	for _, ns := range standardNamespaces {
		_, _ = mgr.Put(ns.prefix, ns.uri, 1)
	}
	return mgr
}
