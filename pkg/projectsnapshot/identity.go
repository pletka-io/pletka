// Package projectsnapshot contains detached, lossless project content and its
// structural source graph. It has no read, policy, serializer or restore capability.
package projectsnapshot

import (
	"fmt"
	"strings"
)

// Identity is an explicit draft or concrete release selection. Zero is invalid.
type Identity struct {
	project string
	version string
	draft   bool
}

// Draft identifies the root working copy. Foreign sources must use Release.
func Draft(project string) (Identity, error) {
	if strings.TrimSpace(project) == "" {
		return Identity{}, fmt.Errorf("snapshot requires a project identity")
	}
	return Identity{project: project, draft: true}, nil
}

// Release requires both project and version; empty never means draft.
func Release(project, version string) (Identity, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(version) == "" {
		return Identity{}, fmt.Errorf("snapshot requires a concrete project release")
	}
	return Identity{project: project, version: version}, nil
}

// ProjectID returns the owning project, not an ontology owner or version label.
func (i Identity) ProjectID() string { return i.project }

// Version returns the concrete release version, or empty for an explicit draft.
func (i Identity) Version() string { return i.version }

// IsDraft reports an explicit working-copy selection.
func (i Identity) IsDraft() bool { return i.draft }

func (i Identity) valid() bool { return i.project != "" && (i.draft != (i.version != "")) }

// EntityKey includes source project and selection, so equal local IDs in
// different projects or releases cannot collide.
type EntityKey struct {
	Source Identity
	Kind   string
	ID     string
}

// Membership records a direct foreign entity and its selected source release.
// It grants neither access nor permission to select a different source version.
type Membership struct {
	Kind   string
	ID     string
	Source Identity
}
