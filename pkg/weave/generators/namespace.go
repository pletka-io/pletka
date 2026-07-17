package generators

import (
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	nsutil "github.com/pletka-io/pletka/pkg/namespace"
)

// NamespaceSet is the prefix map carried by every generator snapshot.
type NamespaceSet struct {
	ProjectPrefix string             `json:"project_prefix,omitempty"`
	ProjectURI    string             `json:"project_uri,omitempty"`
	Bindings      []NamespaceBinding `json:"bindings"`
	ByPrefix      map[string]string  `json:"by_prefix"`
	Project       *NamespaceBinding  `json:"project,omitempty"`
}

// NamespaceBinding is a renderer-neutral namespace binding.
type NamespaceBinding struct {
	Prefix    string `json:"prefix"`
	Namespace string `json:"namespace"`
	Weight    int64  `json:"weight,omitempty"`
	Source    string `json:"source,omitempty"`
}

// Manager builds a weighted namespace manager from the snapshot namespace
// bindings. ByPrefix is folded in as a compatibility source for older/manual
// snapshots; explicit bindings and the project binding carry higher weight.
func (s NamespaceSet) Manager() nsutil.Manager {
	mgr := nsutil.NewStore()
	for prefix, uri := range s.ByPrefix {
		prefix = strings.TrimSpace(prefix)
		uri = strings.TrimSpace(uri)
		if prefix == "" || uri == "" {
			continue
		}
		_, _ = mgr.Put(prefix, uri, 1)
	}
	for _, binding := range s.Bindings {
		prefix := strings.TrimSpace(binding.Prefix)
		uri := strings.TrimSpace(binding.Namespace)
		if prefix == "" || uri == "" {
			continue
		}
		weight := int(binding.Weight)
		if weight == 0 {
			weight = 10
		}
		_, _ = mgr.Put(prefix, uri, weight)
	}
	if s.Project != nil {
		prefix := strings.TrimSpace(s.Project.Prefix)
		uri := strings.TrimSpace(s.Project.Namespace)
		if prefix != "" && uri != "" {
			existing, err := mgr.GetWithPrefix(prefix)
			if err != nil || existing == nil || existing.URI == uri {
				weight := int(s.Project.Weight)
				if weight == 0 {
					weight = 1000
				}
				_, _ = mgr.Put(prefix, uri, weight)
			}
		}
	}
	return mgr
}

// ResolvePrefix returns the highest-weight namespace URI for prefix.
func (s NamespaceSet) ResolvePrefix(prefix string) (string, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", false
	}
	ns, err := s.Manager().GetWithPrefix(prefix)
	if err != nil || ns == nil || ns.URI == "" {
		return "", false
	}
	return ns.URI, true
}

// Prefixes returns the resolved prefix map using the same weighted semantics
// as ResolvePrefix.
func (s NamespaceSet) Prefixes() map[string]string {
	return s.Manager().Prefixes()
}

// BuildNamespaceSet merges project namespace metadata and visible namespace
// bindings into a deterministic prefix set.
func BuildNamespaceSet(project domain.Project, bindings []*domain.NamespaceBinding) NamespaceSet {
	byPrefix := make(map[string]NamespaceBinding)
	for _, b := range bindings {
		if b == nil || strings.TrimSpace(b.Prefix) == "" || strings.TrimSpace(b.Namespace) == "" {
			continue
		}
		prefix := strings.TrimSpace(b.Prefix)
		ns := strings.TrimSpace(b.Namespace)
		existing, ok := byPrefix[prefix]
		if !ok || b.Weight > existing.Weight {
			byPrefix[prefix] = NamespaceBinding{
				Prefix:    prefix,
				Namespace: ns,
				Weight:    b.Weight,
				Source:    b.Source,
			}
		}
	}

	projectPrefix := strings.ToLower(strings.TrimSpace(project.ID))
	projectURI := projectNamespaceURI(project)
	var projectBinding *NamespaceBinding
	if projectPrefix != "" && projectURI != "" {
		nb := NamespaceBinding{
			Prefix:    projectPrefix,
			Namespace: projectURI,
			Weight:    1000,
			Source:    "project",
		}
		if existing, ok := byPrefix[projectPrefix]; !ok || existing.Namespace == projectURI {
			byPrefix[projectPrefix] = nb
		}
		projectBinding = &nb
	}

	out := make([]NamespaceBinding, 0, len(byPrefix))
	for _, b := range byPrefix {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Prefix < out[j].Prefix
	})

	lookup := make(map[string]string, len(out))
	for _, b := range out {
		lookup[b.Prefix] = b.Namespace
	}

	return NamespaceSet{
		ProjectPrefix: projectPrefix,
		ProjectURI:    projectURI,
		Bindings:      out,
		ByPrefix:      lookup,
		Project:       projectBinding,
	}
}

func projectNamespaceURI(project domain.Project) string {
	ns := strings.TrimSpace(project.Namespace)
	if ns == "" && project.ID != "" {
		ns = "/ns/" + project.ID + "/"
	}
	if ns == "" {
		return ""
	}
	if !strings.HasSuffix(ns, "/") && !strings.HasSuffix(ns, "#") {
		ns += "/"
	}
	return ns
}
