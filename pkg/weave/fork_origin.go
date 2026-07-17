package weave

import (
	"context"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

func ForkOriginsForProject(ctx context.Context, store domain.WeaveStore, projectID, entityType string) (map[string]domain.Origin, error) {
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

	forks, err := store.Forks().List(ctx, opts...)
	if err != nil {
		return nil, err
	}

	out := make(map[string]domain.Origin, len(forks))
	for _, fork := range forks {
		out[fork.ForkEntityID] = domain.Origin{
			Kind:            domain.OriginForked,
			SourceProjectID: fork.SourceProjectID,
			SourceEntityID:  fork.SourceEntityID,
		}
	}
	return out, nil
}
