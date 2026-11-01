package projectpage

import (
	"fmt"
	"net/url"
	"time"

	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

func adoptionSchema(value browserinput.Adoptions) (ProjectAdoptionsTabSchema, error) {
	items := make([]ProjectAdoptionTabItem, 0, len(value.Items))
	for _, a := range value.Items {
		r := a.Receipt
		t, err := time.Parse(time.RFC3339Nano, string(r.AdoptedAt))
		if err != nil {
			return ProjectAdoptionsTabSchema{}, err
		}
		version := value.Version
		if version == "" {
			version = "draft"
		}
		items = append(items, ProjectAdoptionTabItem{EntityType: r.EntityType, SourceProjectID: r.SourceProjectID, SourceEntityID: r.SourceEntityID,
			CurrentURL: a.Root.CurrentURL, SourceURL: a.Root.SourceURL, AdoptedAt: t, CreatedBy: a.Actor, Origin: domain.AdoptedOrigin(r.SourceProjectID, r.SourceEntityID),
			Label: domain.Translations{"en": r.EntityType + " · " + r.SourceEntityID, "nl": r.EntityType + " · " + r.SourceEntityID}, Name: a.Root.UIName,
			ModelCount: len(a.Closure["model"]), ModelCountDirect: len(a.Direct["model"]), CollectionCount: len(a.Closure["collection"]), CollectionCountDirect: len(a.Direct["collection"]), FieldCount: len(a.Closure["field"]), FieldCountDirect: len(a.Direct["field"]),
			ClosureURLTemplate: fmt.Sprintf("/projects/%s/adoptions/%s/%s/closure?kind={kind}&version=%s", url.PathEscape(value.ProjectID), url.PathEscape(r.SourceProjectID), url.PathEscape(r.SourceEntityID), url.QueryEscape(version)),
		})
	}
	return ProjectAdoptionsTabSchema{Kind: "adoptions", ProjectID: value.ProjectID, CurrentVersion: value.Version, DraftURL: "/projects/" + value.ProjectID + "?version=draft", Items: items, EmptyMessage: i18n.L("project_page.adoptions_empty", "No adoptions yet."),
		Labels: ProjectAdoptionsLabels{
			Title:             i18n.L("project_page.adoptions.title", "Adoptions"),
			Description:       i18n.L("project_page.adoptions.description", "Explicit receipts you claimed via the Adopt button. Each card lists what that adoption transitively brings into the project — click a count to expand."),
			From:              i18n.L("project_page.adoptions.from", "from"),
			SourceLink:        i18n.L("project_page.adoptions.source_link", "source ↗"),
			AdoptedPrefix:     i18n.L("project_page.adoptions.adopted_prefix", "Adopted"),
			ByPrefix:          i18n.L("project_page.adoptions.by_prefix", "by"),
			Models:            i18n.L("common.models", "Models"),
			Collections:       i18n.L("common.collections", "Collections"),
			Fields:            i18n.L("common.fields", "Fields"),
			Loading:           i18n.L("common.loading", "Loading…"),
			LoadErrorPrefix:   i18n.L("project_page.adoptions.load_error_prefix", "Failed to load:"),
			SectionEmpty:      i18n.L("project_page.adoptions.section_empty", "Nothing in this section."),
			SourceColumnLabel: i18n.L("project_page.adoptions.source_column_label", "source"),
		},
	}, nil
}
