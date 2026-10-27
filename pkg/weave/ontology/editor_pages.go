package ontology

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

const (
	editorBrowseLabel     = "Ontologies"
	editorOverview        = "overview"
	editorRevisionKey     = "revision"
	editorKind            = "ontology-draft"
	editorBrowsePage      = "/admin/ontologies/page"
	editorCallout         = "callout"
	editorPrimaryAction   = "primary"
	editorMetadataList    = "metadata-list"
	editorEditAction      = "edit-draft"
	editorSecondaryAction = "secondary"
	editorSuccessTone     = "green"
	editorDeleteAction    = "delete"
)

func (p *AdminPages) draftPage(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "ontologyID")
	p.renderOntologySchemaShell(w, r, "Ontology draft", "Edit sources and metadata, then publish a release.", editorBase(owner)+"/page-schema", []weavetemplates.Breadcrumb{{Label: editorBrowseLabel, Href: editorBrowsePage}, {Label: "Draft"}})
}
func (h *EditorHost) pageSchema(w http.ResponseWriter, r *http.Request) {
	owner := ontologybody.OntologyID(chi.URLParam(r, "ontologyID"))
	lang := h.lang(r)
	snapshot, err := h.Drafts.Current(r.Context(), owner)
	if err != nil && apierror.FromError(err).Code != apierror.CodeNotFound {
		editorError(w, r, err)
		return
	}
	schema := &formschema.OntologyPageSchema{Kind: editorKind, Title: lt(lang, "Ontology draft"), Sections: []formschema.OntologyPageSection{}, UI: formschema.SchemaUI{PrimaryLang: lang, Languages: h.Languages}}
	if err != nil {
		// Discovery never copies active content: the user chooses a blank draft or
		// explicitly starts from a published version's page.
		o, err := h.Catalog.GetOntology(r.Context(), string(owner))
		if err != nil {
			editorError(w, r, err)
			return
		}
		schema.Title = lt(lang, o.Name+" draft")
		schema.Sections = append(schema.Sections, formschema.OntologyPageSection{ID: "start", Widget: editorCallout, Title: lt(lang, "No draft yet"), Description: lt(lang, "Create a blank draft, or open a published version and choose Start draft from this release.")})
		schema.Actions = []formschema.OntologyPageAction{{ID: "create-draft", Label: lt(lang, "Create blank draft"), FormSchemaURL: editorBase(string(owner)) + "/start-form-schema", Style: editorPrimaryAction}}
	} else {
		ref, _ := NewDraftReference(owner, snapshot.ID)
		schema.Title = lt(lang, snapshot.Document.Ontology.Name+" draft")
		schema.Subtitle = lt(lang, fmt.Sprintf("Revision %d", snapshot.Revision))
		schema.Sections = append(schema.Sections, formschema.OntologyPageSection{ID: editorOverview, Widget: editorMetadataList, Title: lt(lang, "Saved draft"), Metadata: []formschema.OntologyPageMetadata{{Label: lt(lang, "Prefix"), Value: snapshot.Document.Ontology.Prefix}, {Label: lt(lang, "Namespace"), Value: snapshot.Document.Ontology.Namespace}, {Label: lt(lang, "Primary source"), Value: snapshot.Document.Primary.Filename}, {Label: lt(lang, "Companion sources"), Value: strconv.Itoa(len(snapshot.Document.Companions))}}})
		schema.Actions = []formschema.OntologyPageAction{{ID: editorEditAction, Label: lt(lang, "Edit draft"), FormSchemaURL: editorBound(ref) + "/form-schema", Style: editorPrimaryAction}}
		if snapshot.Publication == nil {
			schema.Actions = append(schema.Actions, formschema.OntologyPageAction{ID: "publish-draft", Label: lt(lang, "Publish saved draft"), FormSchemaURL: editorBound(ref) + "/publish-form-schema", Style: editorSecondaryAction})
		} else {
			schema.Sections = append(schema.Sections, formschema.OntologyPageSection{ID: "publication", Widget: editorCallout, Title: lt(lang, "Published as "+snapshot.Publication.Version), Description: lt(lang, "This saved revision has been published. Edit and save the draft to prepare another release."), Tone: editorSuccessTone})
		}
	}
	writeJSON(w, http.StatusOK, schema)
}
func addOntologyEditorAction(schema *formschema.OntologyPageSchema, owner, lang string) {
	for i := range schema.Sections {
		for j := range schema.Sections[i].Cards {
			card := &schema.Sections[i].Cards[j]
			kept := make([]formschema.OntologyPageAction, 0, len(card.Actions))
			for _, action := range card.Actions {
				if action.ID != actionIDEdit && action.ID != editorDeleteAction {
					kept = append(kept, action)
				}
			}
			card.Actions = kept
		}
	}

	for i := range schema.Actions {
		if schema.Actions[i].ID == actionIDEdit {
			schema.Actions[i] = formschema.OntologyPageAction{ID: editorEditAction, Label: lt(lang, "Edit draft"), Href: editorPage(owner), Style: editorPrimaryAction}
			return
		}
	}
	schema.Actions = append(schema.Actions, formschema.OntologyPageAction{ID: editorEditAction, Label: lt(lang, "Edit draft"), Href: editorPage(owner), Style: editorPrimaryAction})
}
func (h *EditorHost) versionActions(ctx context.Context, schema *formschema.OntologyPageSchema, owner, version, lang string) error {
	// Released content never offers edit/delete actions. Mutable active selection
	// and the explicit external-import workflow remain independent.
	actions := make([]formschema.OntologyPageAction, 0, len(schema.Actions))
	for _, action := range schema.Actions {
		if action.ID != actionIDEdit && action.ID != editorDeleteAction {
			actions = append(actions, action)
		}
	}
	schema.Actions = actions
	_, err := h.Drafts.Current(ctx, ontologybody.OntologyID(owner))
	if err == nil {
		schema.Actions = append(schema.Actions, formschema.OntologyPageAction{ID: editorEditAction, Label: lt(lang, "Open draft"), Href: editorPage(owner)})
		return nil
	}
	if apierror.FromError(err).Code != apierror.CodeNotFound {
		return err
	}
	ref, err := ontologyrelease.NewReference(ontologybody.OntologyID(owner), ontologybody.VersionID(version))
	if err != nil {
		return err
	}
	err = h.Releases.Read(ctx, ref, func(ontologyrelease.Reader) error { return nil })
	if err != nil && apierror.FromError(err).Code == apierror.CodeNotInRelease {
		schema.Actions = append(schema.Actions, formschema.OntologyPageAction{ID: editorEditAction, Label: lt(lang, "Create blank draft"), Href: editorPage(owner)})
		return nil
	}
	if err != nil {
		return err
	}
	schema.Actions = append(schema.Actions, formschema.OntologyPageAction{ID: "start-draft", Label: lt(lang, "Start draft from this release"), FormSchemaURL: startReleaseFormURL(owner, version), Style: editorPrimaryAction})
	return nil
}
