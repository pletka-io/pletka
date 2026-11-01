package csvexport

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

const (
	exportFields              = "fields"
	exportModels              = "models"
	exportCollections         = "collections"
	exportCategories          = "categories"
	exportModelOverrides      = "model-field-overrides"
	exportCollectionOverrides = "collection-field-overrides"
	exportBaseOverrides       = "base-field-overrides"
	exportOntologies          = "ontologies"
)

// validExportTypes lists all supported CSV export types.
var validExportTypes = map[string]bool{
	exportFields:              true,
	exportModels:              true,
	exportCollections:         true,
	exportCategories:          true,
	exportModelOverrides:      true,
	exportCollectionOverrides: true,
	exportBaseOverrides:       true,
	exportOntologies:          true,
}

// ExportTypes returns the list of valid export type names.
func ExportTypes() []string {
	return []string{
		exportFields, exportModels, exportCollections, exportCategories,
		exportModelOverrides, exportCollectionOverrides,
		exportBaseOverrides, exportOntologies,
	}
}

// Export is a detached verification projection. It contains owned rows and
// retained foreign reference metadata, never a loader or an effective merged view.
// It is not the lossless project restore format.
type Export struct {
	identity    projectsnapshot.Identity
	prefix      string
	systemName  string
	name        domain.Translations
	fields      []*domain.Field
	models      []*domain.Model
	collections []*domain.Collection
	categories  []*domain.Category
	baseMeta    map[string]FieldOverrideMeta
	baseRefs    map[string]string
	overrides   map[string][]OverrideRow
	ontologies  []OntologyRow
}

// Build projects all eight exports from one validated complete selection. Authored
// paths and raw overrides remain evidence, including unresolved legacy paths.
func Build(value projectsnapshot.CompleteProject) (*Export, error) {
	root, err := value.Root()
	if err != nil {
		return nil, err
	}
	content, err := value.Content(root)
	if err != nil {
		return nil, err
	}
	p := content.Projects[0]
	e := &Export{identity: root, prefix: exportPrefix(derefString(p.SystemName), root.ProjectID()), systemName: derefString(p.SystemName)}
	if err := decode(p.UIName, &e.name); err != nil {
		return nil, err
	}
	if err := e.entities(content); err != nil {
		return nil, err
	}
	if err := e.projectOverrides(value, content); err != nil {
		return nil, err
	}
	e.ontologies, err = ontologyRows(value, root, content.OntologyLinks)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// ProjectName comes from the selected project, not the permission lookup.
func (e *Export) ProjectName(lang string) string {
	return e.name.Get(lang, e.systemName, e.identity.ProjectID())
}

// CSV returns one complete file without reading any state.
func (e *Export) CSV(kind string) ([]byte, error) {
	if e == nil || e.identity.ProjectID() == "" {
		return nil, fmt.Errorf("project CSV is uninitialized")
	}
	var out bytes.Buffer
	var err error
	switch kind {
	case exportFields:
		err = WriteFields(&out, e.fields, e.baseRefs, e.baseMeta)
	case exportModels:
		err = WriteModels(&out, e.models)
	case exportCollections:
		err = WriteCollections(&out, e.collections)
	case exportCategories:
		err = WriteCategories(&out, e.categories)
	case exportModelOverrides:
		err = WriteModelFieldOverrides(&out, e.overrides["model"])
	case exportCollectionOverrides:
		err = WriteCollectionFieldOverrides(&out, e.overrides["collection"])
	case exportBaseOverrides:
		err = WriteBaseFieldOverrides(&out, e.overrides[""])
	case exportOntologies:
		err = WriteOntologies(&out, e.ontologies)
	default:
		return nil, fmt.Errorf("unknown export type: %s", kind)
	}
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func exportPrefix(name, id string) string {
	if name == "" {
		name = id
	}
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	name = strings.Trim(name, "._-")
	if name == "" {
		return "project"
	}
	return name
}
