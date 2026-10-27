package gitmaterializer

import (
	"context"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/releasedescriptor"
)

type manifestActor = releasedescriptor.Actor

func (m *Materializer) loadManifestActor(ctx context.Context, actorID *string) *manifestActor {
	if actorID == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*actorID)
	if trimmed == "" {
		return nil
	}
	row, err := m.queries.WeaveGetActorByID(ctx, trimmed)
	if err != nil {
		return &manifestActor{ActorID: trimmed}
	}
	return &manifestActor{
		ActorID:     row.ID,
		Type:        row.Type,
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
	}
}
