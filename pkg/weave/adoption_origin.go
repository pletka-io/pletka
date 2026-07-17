package weave

import (
	"context"
	"strings"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

func AdoptionOriginsForProject(ctx context.Context, store domain.WeaveStore, projectID, entityType string) (map[string]domain.Origin, error) {
	if store == nil || projectID == "" || entityType == "" {
		return map[string]domain.Origin{}, nil
	}

	opts := []domain.QueryOption{
		domain.WithProjectID(projectID),
		domain.WithFilter("entity_type", entityType),
	}
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		opts = append(opts, domain.WithVersion(version))
	}

	adoptions, err := store.Adoptions().List(ctx, opts...)
	if err != nil {
		return nil, err
	}

	out := make(map[string]domain.Origin, len(adoptions))
	for _, adoption := range adoptions {
		out[adoptionOriginKey(adoption.SourceProjectID, adoption.SourceEntityID)] = domain.AdoptedOrigin(adoption.SourceProjectID, adoption.SourceEntityID)
	}
	return out, nil
}

func ResolveAdoptionOrigin(adopted map[string]domain.Origin, currentProjectID, sourceProjectID, sourceEntityID string) domain.Origin {
	if len(adopted) > 0 {
		if origin, ok := adopted[adoptionOriginKey(sourceProjectID, sourceEntityID)]; ok {
			return origin
		}
	}
	return domain.OriginFromProject(currentProjectID, sourceProjectID)
}

func ResolveReuseOrigin(
	forked map[string]domain.Origin,
	adopted map[string]domain.Origin,
	currentProjectID, sourceProjectID, entityID string,
) domain.Origin {
	if sourceProjectID == "" || sourceProjectID == currentProjectID {
		if len(forked) > 0 {
			if origin, ok := forked[entityID]; ok {
				return origin
			}
		}
		return domain.OwnOrigin()
	}
	return ResolveAdoptionOrigin(adopted, currentProjectID, sourceProjectID, entityID)
}

func adoptionOriginKey(sourceProjectID, sourceEntityID string) string {
	return sourceProjectID + "|" + sourceEntityID
}

func CategoryAdoptionOriginsBySystemName(
	ctx context.Context,
	store domain.WeaveStore,
	projectID string,
	categories []*domain.Category,
) (map[string]domain.Origin, error) {
	if store == nil || projectID == "" || len(categories) == 0 {
		return map[string]domain.Origin{}, nil
	}

	interesting := make(map[string]struct{}, len(categories))
	for _, category := range categories {
		if category == nil || strings.TrimSpace(category.SystemName) == "" {
			continue
		}
		interesting[category.SystemName] = struct{}{}
	}
	if len(interesting) == 0 {
		return map[string]domain.Origin{}, nil
	}

	opts := []domain.QueryOption{
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
		domain.WithFilter("entity_type", "category"),
	}
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		opts = append(opts, domain.WithVersion(version))
	}

	adoptions, err := store.Adoptions().List(ctx, opts...)
	if err != nil {
		return nil, err
	}

	projectLabels := make(map[string]string)
	out := make(map[string]domain.Origin, len(adoptions))
	for _, adoption := range adoptions {
		sourceProjectID := strings.TrimSpace(adoption.SourceProjectID)
		sourceEntityID := strings.TrimSpace(adoption.SourceEntityID)
		if sourceProjectID == "" || sourceEntityID == "" {
			continue
		}
		sourceCategory, err := store.WeaveCategories().GetByIdentifier(ctx, sourceEntityID, sourceProjectID)
		if err != nil || sourceCategory == nil || strings.TrimSpace(sourceCategory.SystemName) == "" {
			continue
		}
		if _, ok := interesting[sourceCategory.SystemName]; !ok {
			continue
		}
		origin := adoption.Origin
		if origin.Kind == "" {
			origin = domain.AdoptedOrigin(sourceProjectID, sourceEntityID)
		}
		if origin.SourceProjectLabel == "" {
			if label := projectLabels[sourceProjectID]; label != "" {
				origin.SourceProjectLabel = label
			} else if project, err := store.Projects().GetByID(ctx, sourceProjectID); err == nil && project != nil {
				label = project.UIName.Get("en", project.ID)
				projectLabels[sourceProjectID] = label
				origin.SourceProjectLabel = label
			}
		}
		out[sourceCategory.SystemName] = origin
	}
	return out, nil
}
