package ontology

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

type draftFormBinding struct {
	field  formschema.FieldDef
	target any
}
type draftSourceEdits struct {
	removed []bool
	added   DraftSource
}

func bindDraftField(name, label, widget string, target any) draftFormBinding {
	field := formschema.FieldDef{Name: name, Label: i18n.L("ontology.editor."+name, label), Widget: widget, Value: target}
	if widget == formschema.WidgetTextarea {
		field.Rows = 12
	}
	return draftFormBinding{field: field, target: target}
}

// The same binding inventory builds schemas and decodes submissions. Unexposed
// descriptor/dependency inputs stay in the bound draft; absent keys cannot erase them.
func draftFormBindings(d *DraftDocument, edits *draftSourceEdits) []draftFormBinding {
	fields := []draftFormBinding{
		bindDraftField("name", "Name", formschema.WidgetText, &d.Ontology.Name),
		bindDraftField("prefix", "Prefix", formschema.WidgetText, &d.Ontology.Prefix),
		bindDraftField("namespace", "Namespace", formschema.WidgetText, &d.Ontology.Namespace),
		bindDraftField("description", "Description", formschema.WidgetMultilingualTextarea, &d.Ontology.Description),
		bindDraftField("homepage_url", "Homepage", formschema.WidgetText, &d.Ontology.HomepageURL),
		bindDraftField("source_url", "Source URL", formschema.WidgetText, &d.Ontology.SourceURL),
	}
	sources := append([]*DraftSource{&d.Primary}, make([]*DraftSource, len(d.Companions))...)
	for i := range d.Companions {
		sources[i+1] = &d.Companions[i]
	}
	edits.removed = make([]bool, len(d.Companions))
	for i, source := range sources {
		key := fmt.Sprintf("source_%d_", i)
		fields = append(fields, bindDraftField(key+"filename", "Filename", formschema.WidgetText, &source.Filename), bindDraftField(key+"content", "RDF/XML source", formschema.WidgetTextarea, &source.Content))
		if i > 0 {
			fields = append(fields, bindDraftField(key+"description", "Description", formschema.WidgetText, &source.Description), bindDraftField(key+"remove", "Remove this companion on save", formschema.WidgetCheckbox, &edits.removed[i-1]))
		}
	}
	return append(fields, bindDraftField("new_filename", "New companion filename (optional)", formschema.WidgetText, &edits.added.Filename), bindDraftField("new_content", "New companion RDF/XML", formschema.WidgetTextarea, &edits.added.Content), bindDraftField("new_description", "New companion description", formschema.WidgetText, &edits.added.Description))
}
func applyDraftForm(d DraftDocument, values map[string]json.RawMessage) (DraftDocument, error) {
	var edits draftSourceEdits
	bindings := draftFormBindings(&d, &edits)
	allowed := map[string]bool{editorRevisionKey: true}
	for _, b := range bindings {
		allowed[b.field.Name] = true
		raw, ok := values[b.field.Name]
		if !ok {
			return d, draftInvalid(b.field.Name, "field is missing; reload the editor")
		}
		if err := json.Unmarshal(raw, b.target); err != nil {
			return d, draftInvalid(b.field.Name, "invalid field value")
		}
	}
	for key := range values {
		if !allowed[key] {
			return d, draftInvalid(key, "unknown field; reload the editor")
		}
	}
	kept := make([]DraftSource, 0, len(d.Companions)+1)
	for i, source := range d.Companions {
		if !edits.removed[i] {
			kept = append(kept, source)
		}
	}
	if edits.added.Filename != "" {
		kept = append(kept, edits.added)
	} else if edits.added.Content != "" || edits.added.Description != "" {
		return d, draftInvalid("new_filename", "name the new companion file")
	}
	d.Companions = kept
	return d, nil
}
func draftFormIdentity(snapshot DraftSnapshot) string {
	names := make([]string, 0, 3+len(snapshot.Document.Companions))
	names = append(names, string(snapshot.OntologyID), string(snapshot.ID), snapshot.Document.Primary.Filename)
	for _, source := range snapshot.Document.Companions {
		names = append(names, source.Filename)
	}
	data, _ := json.Marshal(names)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func editorLanguages(lang string, languages []formschema.LanguageInfo) []formschema.LanguageInfo {
	if len(languages) > 0 {
		return languages
	}
	return []formschema.LanguageInfo{{Code: lang, Name: lang}}
}

// All editor forms share the complete renderer contract, including languages
// and cancellation. Adding another lifecycle action cannot omit these defaults.
func newEditorForm(kind, owner, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: kind, Mode: formschema.ModeCreate,
		UI: formschema.SchemaUI{
			CancelLabel: lt(lang, "Cancel"), PrimaryLang: lang,
			Languages:                  editorLanguages(lang, languages),
			SuccessRedirectURLTemplate: editorPage(owner),
		},
	}
}

func buildDraftForm(snapshot DraftSnapshot, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	var edits draftSourceEdits
	bindings := draftFormBindings(&snapshot.Document, &edits)
	sections := []formschema.Section{{ID: "metadata", Label: lt(lang, "Ontology metadata")}}
	for i, b := range bindings {
		if i == 6 || b.field.Name == "new_filename" {
			sections = append(sections, formschema.Section{ID: b.field.Name, Label: lt(lang, "Source files")})
		}
		if i > 6 && i < len(bindings)-3 && b.field.Widget == formschema.WidgetText && strings.HasSuffix(b.field.Name, "filename") {
			sections = append(sections, formschema.Section{ID: b.field.Name, Label: lt(lang, "Companion source")})
		}
		sections[len(sections)-1].Fields = append(sections[len(sections)-1].Fields, b.field)
	}
	schema := newEditorForm(editorKind, string(snapshot.OntologyID), lang, languages)
	schema.Sections = sections
	schema.Endpoint = &formschema.SchemaEndpoint{Method: http.MethodPost, URL: editorBase(string(snapshot.OntologyID)) + "/form"}
	schema.UI.SubmitLabel = lt(lang, "Create draft")
	schema.UI.SuccessMessage = lt(lang, "Draft created")
	if snapshot.ID != "" {
		ref, _ := NewDraftReference(snapshot.OntologyID, snapshot.ID)
		schema.Mode = formschema.ModeEdit
		schema.Endpoint = &formschema.SchemaEndpoint{Method: http.MethodPut, URL: editorBound(ref) + "/form"}
		schema.UI.SubmitLabel = lt(lang, "Save draft")
		schema.UI.SuccessMessage = lt(lang, "Draft saved")
		schema.Sections[0].Fields = append(schema.Sections[0].Fields, formschema.FieldDef{Name: editorRevisionKey, Widget: formschema.WidgetHidden, Value: snapshot.Revision})
		schema.Conflict = &formschema.FormConflictConfig{RevisionField: editorRevisionKey, Identity: draftFormIdentity(snapshot), Message: lt(lang, "This draft changed while you were editing. Your unsaved values are preserved. Compare with the latest draft before saving again.")}
	}
	return schema
}
func (h *EditorHost) blankDocument(r *http.Request) (DraftDocument, error) {
	o, err := h.Catalog.GetOntology(r.Context(), chi.URLParam(r, "ontologyID"))
	if err != nil {
		return DraftDocument{}, err
	}
	return blankOntologyDocument(o), nil
}
func blankOntologyDocument(o *domain.Ontology) DraftDocument {
	return DraftDocument{
		FormatVersion: 1,
		Ontology: DraftOntology{
			Prefix: o.Prefix, Namespace: o.Namespace, Name: o.Name, Description: o.Description,
			FamilyID: o.FamilyID, Type: o.OntologyType, ExtendsOntologyID: o.ExtendsOntologyID,
			HomepageURL: o.HomepageURL, SourceURL: o.SourceURL,
		},
		Primary: DraftSource{
			Filename: o.Prefix + ".rdf",
			Content:  `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:owl="http://www.w3.org/2002/07/owl#"></rdf:RDF>`,
		},
	}
}
func (h *EditorHost) startFormSchema(w http.ResponseWriter, r *http.Request) {
	owner := ontologybody.OntologyID(chi.URLParam(r, "ontologyID"))
	base := r.URL.Query().Get("from_release")
	if base != "" {
		ref, err := ontologyrelease.NewReference(owner, ontologybody.VersionID(base))
		if err != nil {
			editorError(w, r, err)
			return
		}
		err = h.Releases.Read(r.Context(), ref, func(ontologyrelease.Reader) error { return nil })
		if err != nil {
			editorError(w, r, err)
			return
		}
		lang := h.lang(r)
		schema := newEditorForm(editorKind, string(owner), lang, h.Languages)
		schema.Endpoint = &formschema.SchemaEndpoint{
			Method: http.MethodPost, URL: editorBase(string(owner)),
			Confirm: &formschema.ConfirmConfig{
				Title:        lt(lang, "Create draft"),
				Message:      lt(lang, "Copy this release into the ontology's editable draft? Existing draft work will never be replaced."),
				ConfirmLabel: lt(lang, "Create draft"),
			},
		}
		schema.Sections = []formschema.Section{{
			ID: "source", Label: lt(lang, "Start from this release"),
			Fields: []formschema.FieldDef{{Name: "from_release", Widget: formschema.WidgetHidden, Value: base}},
		}}
		schema.UI.SubmitLabel = lt(lang, "Create draft")
		writeJSON(w, http.StatusOK, schema)
		return
	}
	doc, err := h.blankDocument(r)
	if err != nil {
		editorError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, buildDraftForm(DraftSnapshot{OntologyID: owner, Document: doc}, h.lang(r), h.Languages))
}
func (h *EditorHost) startForm(w http.ResponseWriter, r *http.Request) {
	var values map[string]json.RawMessage
	if !editorDecode(w, r, &values) {
		return
	}
	doc, err := h.blankDocument(r)
	if err == nil {
		doc, err = applyDraftForm(doc, values)
	}
	if err != nil {
		editorError(w, r, err)
		return
	}
	out, err := h.Drafts.Start(r.Context(), ontologybody.OntologyID(chi.URLParam(r, "ontologyID")), doc)
	if err != nil {
		editorError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
func (h *EditorHost) editFormSchema(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.snapshot(r)
	if err != nil {
		editorError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, buildDraftForm(snapshot, h.lang(r), h.Languages))
}
func (h *EditorHost) saveForm(w http.ResponseWriter, r *http.Request) {
	var values map[string]json.RawMessage
	if !editorDecode(w, r, &values) {
		return
	}
	var revision int64
	if err := json.Unmarshal(values[editorRevisionKey], &revision); err != nil {
		editorError(w, r, draftInvalid(editorRevisionKey, "a positive revision is required"))
		return
	}
	ref, err := editorReference(r)
	if err != nil {
		editorError(w, r, err)
		return
	}
	var out DraftSnapshot
	err = h.Drafts.Edit(r.Context(), ref, revision, func(editor DraftEditor) error {
		snapshot, err := editor.Snapshot(r.Context())
		if err != nil {
			return err
		}
		doc, err := applyDraftForm(snapshot.Document, values)
		if err != nil {
			return err
		}
		out, err = editor.Save(r.Context(), doc)
		return err
	})
	if err != nil {
		editorError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (h *EditorHost) publishFormSchema(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.snapshot(r)
	if err != nil {
		editorError(w, r, err)
		return
	}
	ref, _ := NewDraftReference(snapshot.OntologyID, snapshot.ID)
	lang := h.lang(r)
	schema := newEditorForm("ontology-publication", string(snapshot.OntologyID), lang, h.Languages)
	schema.Endpoint = &formschema.SchemaEndpoint{
		Method: http.MethodPost, URL: editorBound(ref) + "/publish",
		Confirm: &formschema.ConfirmConfig{
			Title:        lt(lang, "Publish ontology release"),
			Message:      lt(lang, "Publish the saved draft as an immutable release? Future changes will require a new release."),
			ConfirmLabel: lt(lang, "Publish"),
		},
	}
	schema.Sections = []formschema.Section{{
		ID: "publication", Label: lt(lang, "Release"),
		Fields: []formschema.FieldDef{
			{Name: editorRevisionKey, Widget: formschema.WidgetHidden, Value: snapshot.Revision},
			{Name: "version", Label: lt(lang, "Version"), Widget: formschema.WidgetText, Required: true, Value: ""},
			{Name: "set_active", Label: lt(lang, "Make this the active release"), Widget: formschema.WidgetCheckbox, Value: false},
		},
	}}
	schema.UI.SubmitLabel = lt(lang, "Publish")
	schema.UI.SuccessMessage = lt(lang, "Release published")
	if snapshot.Publication != nil {
		schema.MakeReadOnly()
	}
	writeJSON(w, http.StatusOK, schema)
}
func startReleaseFormURL(owner, version string) string {
	return editorBase(owner) + "/start-form-schema?from_release=" + url.QueryEscape(version)
}
