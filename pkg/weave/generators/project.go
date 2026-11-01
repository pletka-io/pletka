package generators

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// BindProject finishes one complete read and returns a service that can only
// select from that retained value. Bulk exports and identifier lookup share it.
func (s *Service) BindProject(ctx context.Context, scope auth.ReadScope, project string) (*Service, projectsnapshot.CompleteProject, error) {
	if s == nil || s.source == nil || !scope.Valid() {
		return nil, projectsnapshot.CompleteProject{}, fmt.Errorf("generator project requires explicit scope and source")
	}
	value, err := s.source.Project(ctx, scope, project)
	if err != nil {
		return nil, projectsnapshot.CompleteProject{}, err
	}
	frozen := frozenSource{value: value}
	if _, err := frozen.Project(ctx, scope, project); err != nil {
		return nil, projectsnapshot.CompleteProject{}, err
	}
	return NewService(frozen, s.registry), value, nil
}

type frozenSource struct {
	value projectsnapshot.CompleteProject
}

func (s frozenSource) Project(ctx context.Context, scope auth.ReadScope, project string) (projectsnapshot.CompleteProject, error) {
	root, err := s.value.Root()
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	if err := ctx.Err(); err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	if !scope.Valid() || root.ProjectID() != project || root.Version() != scope.Version() || root.IsDraft() == scope.IsRelease() {
		return projectsnapshot.CompleteProject{}, fmt.Errorf("selection is outside bound generator project")
	}
	return s.value, nil
}
func (s frozenSource) Entity(ctx context.Context, scope auth.ReadScope, project, kind, id string) (projectsnapshot.EntityFragment, error) {
	value, err := s.Project(ctx, scope, project)
	if err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	return value.Entity(kind, id)
}

// EntityOptions lists only owned and explicitly visible retained entities. It is
// pure and provides the same identifier inventory to CLI and bulk consumers.
func EntityOptions(value projectsnapshot.CompleteProject, kind string) ([]domain.EntityOption, error) {
	root, err := value.Root()
	if err != nil {
		return nil, err
	}
	input, err := value.Input(root)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	switch kind {
	case string(EntityModel):
		for _, r := range input.Content.Models {
			ids = append(ids, r.ID)
		}
	case string(EntityCollection):
		for _, r := range input.Content.Collections {
			ids = append(ids, r.ID)
		}
	case string(EntityField):
		for _, r := range input.Content.Fields {
			ids = append(ids, r.ID)
		}
	default:
		return nil, fmt.Errorf("unsupported generator entity kind")
	}
	for _, m := range input.Membership {
		if m.Kind == kind {
			ids = append(ids, m.ID)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	out := make([]domain.EntityOption, 0, len(ids))
	for _, id := range ids {
		fragment, err := value.Entity(kind, id)
		if err != nil {
			return nil, err
		}
		key, err := fragment.Root()
		if err != nil {
			return nil, err
		}
		c, err := fragment.Content(key.Source)
		if err != nil {
			return nil, err
		}
		item, err := retainedEntityOption(c, kind, id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// ResolveIdentifier resolves ID, semantic ID or system name within retained data.
// Ambiguous aliases fail instead of depending on query/iteration order.
func ResolveIdentifier(value projectsnapshot.CompleteProject, kind, identifier string) (string, error) {
	if strings.TrimSpace(identifier) == "" {
		return "", fmt.Errorf("generator identifier is required")
	}
	items, err := EntityOptions(value, kind)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.ID == identifier {
			return item.ID, nil
		}
	}
	found := ""
	for _, item := range items {
		if item.SemanticID == identifier || strings.EqualFold(item.SystemName, identifier) {
			if found != "" && found != item.ID {
				return "", fmt.Errorf("ambiguous generator identifier %q", identifier)
			}
			found = item.ID
		}
	}
	if found == "" {
		return "", fmt.Errorf("generator %s %q is not in selected project", kind, identifier)
	}
	return found, nil
}

func retainedEntityOption(c projectsnapshot.Content, kind, id string) (domain.EntityOption, error) {
	item := domain.EntityOption{ID: id, SemanticID: id}
	var label []byte
	switch kind {
	case string(EntityModel):
		for _, r := range c.Models {
			if r.ID == id {
				item.SystemName = projectionValue(r.SystemName)
				label = r.UIName
			}
		}
	case string(EntityCollection):
		for _, r := range c.Collections {
			if r.ID == id {
				item.SystemName = projectionValue(r.SystemName)
				label = r.UIName
			}
		}
	case string(EntityField):
		for _, r := range c.Fields {
			if r.ID == id {
				item.SemanticID = projectionValue(r.SemanticID)
				item.SystemName = projectionValue(r.SystemName)
				label = r.UIName
			}
		}
	}
	if err := decodeProjection(label, &item.UIName); err != nil {
		return domain.EntityOption{}, err
	}

	return item, nil
}
