package gitmaterializer

import (
	"context"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// adoptionIDResolver maps one entity reference of an adoption row to another
// spelling of the same entity: the writer maps a row id to the semantic id,
// restore maps whatever the receipt says back to the row id. ok is false
// when the entity is unknown; the caller then keeps the id as written.
type adoptionIDResolver func(ctx context.Context, entityType, projectID, id string) (string, bool)

// rewriteAdoptionIDs passes the source entity (in its source project) and the
// context entity (in the adopting project) of every adoption through resolve.
func rewriteAdoptionIDs(ctx context.Context, adoptions []domain.Adoption, resolve adoptionIDResolver) []domain.Adoption {
	out := make([]domain.Adoption, 0, len(adoptions))
	for _, a := range adoptions {
		if id, ok := resolve(ctx, a.EntityType, a.SourceProjectID, a.SourceEntityID); ok {
			a.SourceEntityID = id
		}
		if id, ok := resolve(ctx, a.ContextEntityType, a.ProjectID, a.ContextEntityID); ok {
			a.ContextEntityID = id
		}
		out = append(out, a)
	}
	return out
}

// semanticAdoptionIDs resolves a row id to the entity's semantic id, so a
// receipt names entities the way the tree does (a UI-created entity keeps a
// ULID row id; the tree knows it by its semantic id only).
func semanticAdoptionIDs(q *sqlcgen.Queries) adoptionIDResolver {
	return func(ctx context.Context, entityType, _, id string) (string, bool) {
		var sem *string
		var err error
		switch entityType {
		case entityTypeField:
			var row sqlcgen.WeaveField
			row, err = q.WeaveGetFieldByID(ctx, id)
			sem = row.SemanticID
		case entityTypeCategory:
			var row sqlcgen.WeaveCategory
			row, err = q.WeaveGetCategoryByID(ctx, id)
			sem = row.SemanticID
		case entityTypeConceptList:
			var row sqlcgen.WeaveConceptList
			row, err = q.WeaveGetConceptListByIDOrSemanticID(ctx, id)
			sem = row.SemanticID
		default:
			// models and collections have no separate semantic id: the row id is it
			return "", false
		}
		if err != nil || sem == nil || strings.TrimSpace(*sem) == "" {
			return "", false
		}
		return strings.TrimSpace(*sem), true
	}
}

// rowAdoptionIDs resolves an identifier written in a receipt (row id,
// semantic id or system name) to the row id on this instance, within the
// project the receipt names.
func rowAdoptionIDs(q *sqlcgen.Queries) adoptionIDResolver {
	return func(ctx context.Context, entityType, projectID, id string) (string, bool) {
		id = strings.TrimSpace(id)
		if id == "" || projectID == "" {
			return "", false
		}
		switch entityType {
		case entityTypeField:
			row, err := q.WeaveGetFieldByIdentifier(ctx, sqlcgen.WeaveGetFieldByIdentifierParams{SemanticID: nullableString(id), ProjectID: projectID})
			if err != nil {
				return "", false
			}
			return row.ID, true
		case entityTypeCategory:
			row, err := q.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{SemanticID: nullableString(id), ProjectID: projectID})
			if err != nil {
				return "", false
			}
			return row.ID, true
		case entityTypeModel:
			row, err := q.WeaveGetModelByIdentifier(ctx, sqlcgen.WeaveGetModelByIdentifierParams{ProjectID: projectID, Identifier: id})
			if err != nil {
				return "", false
			}
			return row.ID, true
		case entityTypeCollection:
			row, err := q.WeaveGetCollectionByIdentifier(ctx, sqlcgen.WeaveGetCollectionByIdentifierParams{ProjectID: projectID, Identifier: id})
			if err != nil {
				return "", false
			}
			return row.ID, true
		case entityTypeConceptList:
			row, err := q.WeaveGetConceptListByIDOrSemanticID(ctx, id)
			if err != nil || row.ProjectID != projectID {
				return "", false
			}
			return row.ID, true
		}
		return "", false
	}
}
