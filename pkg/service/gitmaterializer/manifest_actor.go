package gitmaterializer

import (
	"context"
	"strings"
)

type manifestActor struct {
	ActorID     string `json:"actor_id,omitempty"`
	Type        string `json:"type,omitempty"`
	Slug        string `json:"slug,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

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
