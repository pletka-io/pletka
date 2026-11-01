package browserread

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Header returns only selected project display metadata for an HTML shell.
func (s *Source) Header(ctx context.Context, scope auth.ReadScope, project string) (browserinput.ProjectHeader, error) {
	var out browserinput.ProjectHeader
	if s == nil || s.sessions == nil {
		return out, fmt.Errorf("browser source is uninitialized")
	}
	err := s.sessions.Read(ctx, scope, project, func(session *readsession.Session) error {
		var err error
		out, err = session.ProjectHeader(ctx)
		return err
	})
	if err != nil {
		return browserinput.ProjectHeader{}, err
	}
	return out, nil
}

// Summary captures every project-page content facet in one read session.
func (s *Source) Summary(ctx context.Context, scope auth.ReadScope, project string) (browserinput.ProjectSummary, error) {
	var out browserinput.ProjectSummary
	if s == nil || s.sessions == nil {
		return out, fmt.Errorf("browser source is uninitialized")
	}
	err := s.sessions.Read(ctx, scope, project, func(session *readsession.Session) error {
		var err error
		out.Header, err = session.ProjectHeader(ctx)
		if err != nil {
			return err
		}
		out.Counts, err = session.ProjectCounts(ctx)
		if err != nil {
			return err
		}
		out.Credits, err = session.ProjectCredits(ctx)
		if err != nil {
			return err
		}
		out.Ontologies, err = projectOntologies(ctx, session)
		return err
	})
	if err != nil {
		return browserinput.ProjectSummary{}, err
	}
	return out, nil
}

func projectOntologies(ctx context.Context, root *readsession.Session) ([]browserinput.OntologySummary, error) {
	out := []browserinput.OntologySummary{}
	queue := []*readsession.Session{root}
	seen := map[string]bool{}
	sessions := map[string]*readsession.Session{}
	versions := map[string]string{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current.ProjectID()] {
			continue
		}
		seen[current.ProjectID()] = true
		sessions[current.ProjectID()] = current
		if len(seen) > 32 {
			return nil, fmt.Errorf("project summary exceeds 32 inherited projects")
		}
		own, err := current.OwnOntologySummaries(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range own {
			if digest, ok := versions[o.VersionID]; ok {
				if digest != o.Digest {
					return nil, fmt.Errorf("ontology %s has conflicting retained bodies", o.VersionID)
				}
				continue
			}
			versions[o.VersionID] = o.Digest
			if len(versions) > readsession.SummaryLimit {
				return nil, fmt.Errorf("project summary ontology limit exceeded")
			}
			out = append(out, o)
		}
		ids, err := summaryParents(ctx, current)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			next, err := current.ProjectDependency(ctx, id)
			if err != nil {
				return nil, err
			}
			queue = append(queue, next)
		}
	}
	if err := summaryUsage(ctx, root, sessions, out); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b browserinput.OntologySummary) int {
		if (a.SourceProjectID == root.ProjectID()) != (b.SourceProjectID == root.ProjectID()) {
			if a.SourceProjectID == root.ProjectID() {
				return -1
			}
			return 1
		}
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return strings.Compare(a.VersionID, b.VersionID)
	})
	return out, nil
}

func summaryParents(ctx context.Context, current *readsession.Session) ([]string, error) {
	header, err := current.ProjectHeader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := current.ContentRows()
	if err != nil {
		return nil, err
	}
	parents, err := rows.ContentInheritance(ctx, readsession.Page{Limit: 33})
	if err != nil {
		return nil, err
	}
	if len(parents) > 32 {
		return nil, fmt.Errorf("project summary parent limit exceeded")
	}
	slices.SortFunc(parents, func(a, b projectsnapshot.InheritanceRecord) int {
		if a.CanonicalOrder < b.CanonicalOrder {
			return -1
		}
		if a.CanonicalOrder > b.CanonicalOrder {
			return 1
		}
		return strings.Compare(a.ParentProjectID, b.ParentProjectID)
	})
	ids := make([]string, 0, len(parents)+1)
	for _, p := range parents {
		ids = append(ids, p.ParentProjectID)
	}
	if p := header.Record.ParentProjectID; p != nil && *p != "" && !slices.Contains(ids, *p) {
		ids = append(ids, *p)
	}
	return ids, nil
}

func summaryUsage(ctx context.Context, root *readsession.Session, sessions map[string]*readsession.Session, out []browserinput.OntologySummary) error {
	ids := make([]string, 0, len(out))
	for _, o := range out {
		ids = append(ids, o.VersionID)
	}
	usage := map[string]readsession.OntologyUsage{}
	if root.Scope().IsRelease() {
		var err error
		usage, err = root.OntologyUsages(ctx, ids)
		if err != nil {
			return err
		}
	} else {
		refs, err := root.DraftSummaryReferences(ctx)
		if err != nil {
			return err
		}
		groups := map[string][]string{}
		for _, o := range out {
			groups[o.SourceProjectID] = append(groups[o.SourceProjectID], o.VersionID)
		}
		projects := make([]string, 0, len(groups))
		for id := range groups {
			projects = append(projects, id)
		}
		slices.Sort(projects)
		for _, id := range projects {
			counts, err := sessions[id].MatchOntologyReferences(ctx, groups[id], refs)
			if err != nil {
				return err
			}
			for version, count := range counts {
				usage[version] = count
			}
		}
	}
	for i := range out {
		u, ok := usage[out[i].VersionID]
		if !ok {
			return fmt.Errorf("project summary usage is incomplete")
		}
		out[i].ClassesUsed = u.Classes
		out[i].PropertiesUsed = u.Properties
		out[i].UsageUnavailable = !u.Known
	}
	return nil
}
