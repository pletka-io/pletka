package gitmaterializer

import (
	"path"

	"github.com/pletka-io/pletka/pkg/domain"
)

// deletePathsFor returns the repo-relative paths to remove when an entity of
// entityType (identifier = its semantic id or ulid, matching how it was
// written) is deleted. Model/field/collection own a directory (entity file
// plus nested overrides); category owns a single file.
func deletePathsFor(entityType, identifier string) []string {
	if identifier == "" {
		// An empty identifier would compute e.g. "models/" -> path.Dir ->
		// wipe the whole models/ directory. No entity type ever has a valid
		// empty identifier, so treat it as nothing-to-delete.
		return nil
	}
	switch entityType {
	case entityTypeModel, entityTypeField, entityTypeCollection:
		// Remove the whole entity directory (e.g. models/GRPM.2), which holds
		// the entity file plus any overrides/ written beneath it.
		return []string{path.Dir(domain.FilePath(domain.PathSpec{EntityType: entityType, EntityID: identifier}))}
	case entityTypeCategory:
		return []string{domain.FilePath(domain.PathSpec{EntityType: entityType, EntityID: identifier})}
	default:
		return nil
	}
}
