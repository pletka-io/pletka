package vocabulary

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
	weaveadmin "github.com/pletka-io/pletka/pkg/weave/admin"
)

type Host struct {
	Service      *Service
	Projects     auth.ProjectReader
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

func (h Host) Validate() error {
	var missing []string
	if h.Service == nil {
		missing = append(missing, "Service")
	}
	if h.Projects == nil {
		missing = append(missing, "Projects")
	}
	if len(missing) > 0 {
		return fmt.Errorf("vocabulary host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Service, host.Logger, host.Languages, host.LangResolver)

	parent.Get("/api/v2/vocabularies", h.ListGlobalVocabularies)
	parent.Get("/api/v2/vocabularies/{vocabularyID}/entries/search", h.SearchVocabularyEntries)
	parent.Get("/api/v2/vocabulary-entries/resolve", h.ResolveEntry)
	parent.With(weaveadmin.RequireSuperAdmin).Get("/admin/vocabularies/entity-list-schema", h.AdminVocabularyEntityListSchema)
	parent.With(weaveadmin.RequireSuperAdmin).Get("/admin/vocabularies/data", h.AdminVocabularyData)

	parent.With(auth.RequireProjectRead(host.Projects)).Get("/api/v2/projects/{projectID}/vocabularies", h.ListProjectVocabularies)
	parent.With(auth.RequireProjectRead(host.Projects)).Get("/api/v2/projects/{projectID}/concept-lists", h.ListProjectConceptLists)
	parent.With(auth.RequireProjectEdit(host.Projects)).Get("/api/v2/projects/{projectID}/concept-lists/form-schema", h.ConceptListCreateFormSchema)
	parent.With(auth.RequireProjectEdit(host.Projects)).Post("/api/v2/projects/{projectID}/concept-lists", h.CreateProjectConceptList)
	parent.With(auth.RequireProjectRead(host.Projects)).Get("/api/v2/projects/{projectID}/concept-lists/filters/vocabularies", h.ConceptListVocabularyFilterOptions)
	parent.With(auth.RequireProjectRead(host.Projects)).Get("/api/v2/projects/{projectID}/concept-lists/{listID}", h.GetProjectConceptList)
	parent.With(auth.RequireProjectEdit(host.Projects)).Get("/api/v2/projects/{projectID}/concept-lists/{listID}/form-schema", h.ConceptListEditFormSchema)
	parent.With(auth.RequireProjectEdit(host.Projects)).Put("/api/v2/projects/{projectID}/concept-lists/{listID}", h.UpdateProjectConceptList)
	parent.With(auth.RequireProjectEdit(host.Projects)).Delete("/api/v2/projects/{projectID}/concept-lists/{listID}", h.DeleteProjectConceptList)
	parent.With(auth.RequireProjectEdit(host.Projects)).Get("/api/v2/projects/{projectID}/concept-lists/{listID}/source-entries/search", h.SearchConceptListSourceEntries)
	parent.With(auth.RequireProjectEdit(host.Projects)).Post("/api/v2/projects/{projectID}/concept-lists/{listID}/entries", h.AddProjectConceptListEntry)
	parent.With(auth.RequireProjectEdit(host.Projects)).Patch("/api/v2/projects/{projectID}/concept-lists/{listID}/entries/reorder", h.ReorderProjectConceptListEntries)
	parent.With(auth.RequireProjectEdit(host.Projects)).Patch("/api/v2/projects/{projectID}/concept-lists/{listID}/entries/{entryID}", h.UpdateProjectConceptListEntry)
	parent.With(auth.RequireProjectEdit(host.Projects)).Delete("/api/v2/projects/{projectID}/concept-lists/{listID}/entries/{entryID}", h.RemoveProjectConceptListEntry)
	parent.Get("/api/v2/concept-lists/{listID}/entries/search", h.SearchConceptListEntries)
}
