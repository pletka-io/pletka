// Package auth defines the identity, authorization snapshot, and capability
// map that the schema builders consult when deciding what actions to expose
// to a user.
package auth

// Capability is a string-typed action name, e.g. "field.edit".
type Capability string

// Project-scope capabilities.
const (
	ProjectRead          Capability = "project.read"
	ProjectEdit          Capability = "project.edit"
	ProjectDelete        Capability = "project.delete"
	ProjectManageMembers Capability = "project.manage_members"

	FieldRead    Capability = "field.read"
	FieldCreate  Capability = "field.create"
	FieldEdit    Capability = "field.edit"
	FieldPublish Capability = "field.publish"
	FieldDelete  Capability = "field.delete"

	ModelRead    Capability = "model.read"
	ModelCreate  Capability = "model.create"
	ModelEdit    Capability = "model.edit"
	ModelPublish Capability = "model.publish"
	ModelDelete  Capability = "model.delete"

	CollectionRead    Capability = "collection.read"
	CollectionCreate  Capability = "collection.create"
	CollectionEdit    Capability = "collection.edit"
	CollectionPublish Capability = "collection.publish"
	CollectionDelete  Capability = "collection.delete"

	CommentCreate Capability = "comment.create"
)

// Org-scope capabilities.
const (
	OrgRead          Capability = "org.read"
	OrgEdit          Capability = "org.edit"
	OrgManageMembers Capability = "org.manage_members"
	OrgProjectCreate Capability = "org.project_create"
	OrgDelete        Capability = "org.delete"
)

// Fine-grained UI-feature capabilities. These gate optional surfaces
// the schema builders emit (or omit) per-caller — e.g. whether the
// derivatives "Graph" sub-tab appears on a model/collection/field
// detail view. The frontend never checks roles directly; the server
// emits URLs only for capabilities the caller has, and the UI hides
// affordances whose URL is absent. New gated surfaces should declare
// a capability here, map it to the roles that should grant it
// (superadmin always passes via the IsSuperAdmin bypass in Can()),
// and let derivativesFor / similar emit-or-omit URLs based on
// snap.Can(cap, resource, nil).
const (
	// DerivativeExportGraphRead controls visibility of the
	// /gen/{kind}/{id}/exportgraph endpoint and its corresponding
	// "Graph" sub-tab in DiagramTab. Today granted to superadmin only —
	// the raw export-graph view exposes more internal modelling state
	// than is helpful to general curators. Loosen by adding to
	// projectRoleCapabilities sets below.
	DerivativeExportGraphRead Capability = "derivative.exportgraph.read"

	// The advanced derivatives below are company-private while under
	// development — granted to superadmin only (via the IsSuperAdmin bypass).
	// Per-format so each can be opened independently later; loosen by adding
	// to the projectRoleCapabilities sets below. Enforcement flows through
	// CanDerivative, consulted at BOTH the HTTP choke point
	// (visualization.canDerivativeFormat) and the detailview URL emit, so no
	// exit point leaks.
	DerivativeSHACLRead     Capability = "derivative.shacl.read"
	DerivativeSPARQLRead    Capability = "derivative.sparql.read"
	DerivativeArchesRead    Capability = "derivative.arches.read"
	DerivativeSnapshotRead  Capability = "derivative.snapshot.read"
	DerivativeASCIITreeRead Capability = "derivative.asciitree.read"
)

// openDerivatives are the generator formats any project reader may fetch —
// the visual + standard-RDF curator outputs. Every other format is
// super_admin-only (fail closed) unless it has an escalation capability in
// derivativeCaps. Keyed by the generators.Format string value to keep this
// policy in auth without an auth->generators import cycle. To open a format,
// add it here.
var openDerivatives = map[string]bool{
	"mermaid":   true,
	"cytoscape": true,
	"turtle":    true,
	"jsonld":    true,
	"x3ml":      true,
	"x3ml-b":    true,
}

// derivativeCaps maps gated formats to the capability that opens them beyond
// super_admin. A format here is granted to super_admin (IsSuperAdmin bypass)
// plus any role holding the capability. Escalate by granting the cap in a
// projectRoleCapabilities set above. Formats in neither map fall through to
// super_admin-only.
var derivativeCaps = map[string]Capability{
	"exportgraph": DerivativeExportGraphRead,
	"shacl":       DerivativeSHACLRead,
	"sparql":      DerivativeSPARQLRead,
	"arches":      DerivativeArchesRead,
	"snapshot":    DerivativeSnapshotRead,
	"ascii-tree":  DerivativeASCIITreeRead,
}

// CanDerivative reports whether the caller may fetch or be shown the given
// generator format on resource r. Fail closed: a format in neither
// openDerivatives nor derivativeCaps requires super_admin, so a newly added
// generator is gated by default until explicitly opened. This is the single
// policy both the visualization HTTP handlers and the detailview URL emit
// consult, so a format cannot be open in one exit point and gated in another.
func (s *AuthSnapshot) CanDerivative(format string, r Resource) bool {
	if openDerivatives[format] {
		return true
	}
	if cap, ok := derivativeCaps[format]; ok {
		return s.Can(cap, r, nil)
	}
	return s.IsSuperAdmin
}

// CapSet is an efficient set of capabilities. Zero-byte value type.
type CapSet map[Capability]struct{}

// Has reports whether c is in the set.
func (s CapSet) Has(c Capability) bool {
	_, ok := s[c]
	return ok
}

func newSet(caps ...Capability) CapSet {
	s := make(CapSet, len(caps))
	for _, c := range caps {
		s[c] = struct{}{}
	}
	return s
}

var emptySet = newSet()

var projectRoleCapabilities = map[string]CapSet{
	"owner": newSet(
		ProjectRead, ProjectEdit, ProjectDelete, ProjectManageMembers,
		FieldRead, FieldCreate, FieldEdit, FieldPublish, FieldDelete,
		ModelRead, ModelCreate, ModelEdit, ModelPublish, ModelDelete,
		CollectionRead, CollectionCreate, CollectionEdit, CollectionPublish, CollectionDelete,
		CommentCreate,
	),
	"maintainer": newSet(
		ProjectRead, ProjectEdit,
		FieldRead, FieldCreate, FieldEdit, FieldPublish, FieldDelete,
		ModelRead, ModelCreate, ModelEdit, ModelPublish, ModelDelete,
		CollectionRead, CollectionCreate, CollectionEdit, CollectionPublish, CollectionDelete,
		CommentCreate,
	),
	"contributor": newSet(
		ProjectRead,
		FieldRead, FieldCreate,
		ModelRead, ModelCreate,
		CollectionRead, CollectionCreate,
		CommentCreate,
	),
	"viewer": newSet(
		ProjectRead, FieldRead, ModelRead, CollectionRead,
	),
}

var orgRoleCapabilities = map[string]CapSet{
	"owner":  newSet(OrgRead, OrgEdit, OrgManageMembers, OrgProjectCreate, OrgDelete),
	"admin":  newSet(OrgRead, OrgEdit, OrgManageMembers, OrgProjectCreate),
	"member": newSet(OrgRead),
}

var ownerSelfCaps = newSet(
	FieldEdit, FieldDelete,
	ModelEdit, ModelDelete,
	CollectionEdit, CollectionDelete,
)

// RoleForScope returns the capability set granted by role in scopeType.
// Returns an empty set for unknown role/scope combinations.
func RoleForScope(scopeType, role string) CapSet {
	switch scopeType {
	case "project":
		if s, ok := projectRoleCapabilities[role]; ok {
			return s
		}
	case "org":
		if s, ok := orgRoleCapabilities[role]; ok {
			return s
		}
	}
	return emptySet
}

// OwnerSelfCaps returns the capabilities an entity's creator holds on their
// own DRAFT entities, consulted by the ownership modifier in Can().
func OwnerSelfCaps() CapSet { return ownerSelfCaps }

// AllProjectCapabilities returns every project-scoped capability. Used by
// tests and by documentation generators.
func AllProjectCapabilities() []Capability {
	return []Capability{
		ProjectRead, ProjectEdit, ProjectDelete, ProjectManageMembers,
		FieldRead, FieldCreate, FieldEdit, FieldPublish, FieldDelete,
		ModelRead, ModelCreate, ModelEdit, ModelPublish, ModelDelete,
		CollectionRead, CollectionCreate, CollectionEdit, CollectionPublish, CollectionDelete,
		CommentCreate,
	}
}
