package auth

// AuthSnapshot is built per-request and attached to context. All read paths
// consult it via FromContext(ctx) and Can(cap, resource, entity).
type AuthSnapshot struct {
	// ActorID is "" when IsAnonymous is true.
	ActorID     string
	IsAnonymous bool
	// IsSuperAdmin short-circuits Can() to true when set. Populated from
	// weave_actors.role == "super_admin" during BuildSnapshot.
	IsSuperAdmin bool
	// Roles is keyed by "scopeType:scopeID", e.g. "project:LA", "org:org1".
	Roles map[string]string
	// OwnedProjectIDs is the set of project IDs where weave_projects.owner_id == ActorID.
	OwnedProjectIDs map[string]struct{}
}

// Resource identifies the thing a capability check is targeted at.
// Callers (handlers, schema builders) construct this from already-loaded data.
type Resource struct {
	// ScopeType is "project" or "org".
	ScopeType string
	// ID is the project ID or org/actor ID.
	ID string
	// OrgID is the project's owner, when the owner is an org-type actor.
	// Empty when owner is a person or when ScopeType != "project".
	OrgID string
	// Visibility is "public" or "private". Used by project and org resources.
	Visibility string
}

// EntityContext carries the specific entity being acted on when ownership
// may grant extra capabilities beyond role. Pass nil when not applicable.
type EntityContext struct {
	CreatedByID string
	Status      string // "draft" | "published" | "deprecated"
}

// EffectiveRole resolves the highest role the snapshot grants on r.
// Resolution order (highest wins):
//  1. direct project ownership (OwnedProjectIDs)
//  2. higher of inherited-from-org vs explicit project membership
//  3. public-read fallback
//
// Nil receiver is treated as anonymous: only public reads are granted.
func (s *AuthSnapshot) EffectiveRole(r Resource) string {
	if s == nil {
		if (r.ScopeType == "project" || r.ScopeType == "org") && r.Visibility == "public" {
			if r.ScopeType == "project" {
				return "viewer"
			}
			return "member"
		}
		return ""
	}
	// Super-admin bypass: treat as owner on every project/org.
	if s.IsSuperAdmin {
		return "owner"
	}
	if r.ScopeType == "project" {
		if _, own := s.OwnedProjectIDs[r.ID]; own {
			return "owner"
		}

		inherited := ""
		if r.OrgID != "" {
			switch s.Roles["org:"+r.OrgID] {
			case "owner", "admin":
				inherited = "owner"
			case "member":
				if r.Visibility == "public" {
					inherited = "viewer"
				}
			}
		}
		explicit := s.Roles["project:"+r.ID]

		if winner := higherProjectRole(inherited, explicit); winner != "" {
			return winner
		}
		if r.Visibility == "public" {
			return "viewer"
		}
		return ""
	}
	if r.ScopeType == "org" {
		if role := s.Roles["org:"+r.ID]; role != "" {
			return role
		}
		if r.Visibility == "public" {
			return "member"
		}
		return ""
	}
	return ""
}

// higherProjectRole returns whichever of a, b has the higher project-role
// rank (owner > maintainer > contributor > viewer). Returns "" when both
// are empty.
func higherProjectRole(a, b string) string {
	return pickHigher(a, b, projectRoleRank)
}

var projectRoleRank = map[string]int{
	"owner":       4,
	"maintainer":  3,
	"contributor": 2,
	"viewer":      1,
}

func pickHigher(a, b string, rank map[string]int) string {
	if rank[a] >= rank[b] {
		return a
	}
	return b
}

// IsProjectMember reports whether the snapshot represents a member of
// the project resource r — owner, explicit project role, or inherited
// org-membership. Public-project anonymous fallback (`viewer`) does
// NOT count as membership.
//
// Used by surfaces that should be visible to project participants only,
// not to drive-by readers — e.g. the verification CSV export, which
// dumps drafts + override detail. Does not replace Can(); use Can()
// for capability checks. This is a simple "are you on the team?"
// predicate.
//
// Nil receiver / anonymous → false.
func (s *AuthSnapshot) IsProjectMember(r Resource) bool {
	if s == nil || s.IsAnonymous {
		return false
	}
	if s.IsSuperAdmin {
		return true
	}
	if r.ScopeType != "project" {
		return false
	}
	if _, own := s.OwnedProjectIDs[r.ID]; own {
		return true
	}
	if s.Roles["project:"+r.ID] != "" {
		return true
	}
	if r.OrgID != "" && s.Roles["org:"+r.OrgID] != "" {
		return true
	}
	return false
}

// Can reports whether the snapshot grants capability cap on resource r,
// optionally considering the specific entity being acted on (for the
// ownership modifier that lets creators edit/delete their own drafts).
func (s *AuthSnapshot) Can(cap Capability, r Resource, entity *EntityContext) bool {
	// Super-admin bypass: always allowed. Avoids per-capability gate
	// maintenance as new capabilities are added.
	if s != nil && s.IsSuperAdmin {
		return true
	}
	role := s.EffectiveRole(r)
	if RoleForScope(r.ScopeType, role).Has(cap) {
		return true
	}

	if entity == nil || s == nil || s.ActorID == "" {
		return false
	}
	// Ownership modifier: creators can edit/delete their own drafts
	// regardless of role, as long as they can at least see the resource.
	if entity.CreatedByID != s.ActorID || entity.Status != "draft" {
		return false
	}
	// Minimum gate: must have at least read access to the resource.
	if !RoleForScope(r.ScopeType, role).Has(ProjectRead) {
		return false
	}
	return OwnerSelfCaps().Has(cap)
}
