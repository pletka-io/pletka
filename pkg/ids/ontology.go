package ids

import (
	"github.com/google/uuid"
)

// OntologyNamespace is the fixed UUID5 namespace under which every
// ontology-related deterministic UUID is derived. Pinning a v1 key in
// the construction lets us bump to v2 in the future without confusing
// any UUIDs already emitted under v1.
//
// The literal below is the precomputed value of
//
//	uuid.NewSHA1(uuid.Nil, []byte("archessql/ontology/v1"))
//
// pasted here so the bytes never change even if the derivation
// expression is ever altered. Verified by TestOntologyNamespaceLiteral.
var OntologyNamespace = uuid.MustParse("cd11247e-0958-5b3f-bc4f-2d3f41473606")

// OntologyVersionID returns the deterministic UUID for one ontology
// version, keyed by (prefix, version) — e.g. ("crm", "7.1.3"). Same
// inputs produce the same UUID across projects + installations, so
// graph.ontologyid references resolve as long as the destination DB
// has run a matching `archessql ontology import`.
func OntologyVersionID(prefix, version string) string {
	return UUIDv5(OntologyNamespace, prefix+"/"+version)
}

// OntologyClassID returns the deterministic UUID for one ontology
// class within a specific (prefix, version), keyed by the class qname
// (e.g. "crm:E1_CRM_Entity").
func OntologyClassID(prefix, version, qname string) string {
	return UUIDv5(OntologyNamespace, prefix+"/"+version+"/class/"+qname)
}
