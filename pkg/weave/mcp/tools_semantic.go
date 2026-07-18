package mcp

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/category"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/detailview"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

type reuseInput struct {
	ProjectID  string `json:"project_id" jsonschema:"project ID / prefix"`
	EntityType string `json:"entity_type" jsonschema:"one of: field, model, collection"`
	EntityID   string `json:"entity_id" jsonschema:"entity ULID"`
}

func entityReuse(ctx context.Context, h Host, in reuseInput) (*detailview.FieldReuseResponse, error) {
	if _, err := resolveProject(ctx, h, in.ProjectID); err != nil {
		return nil, err
	}
	resp, err := detailview.BuildReuse(ctx, h.Weave, in.ProjectID, strings.ToLower(in.EntityType), in.EntityID)
	if err != nil {
		return nil, fmt.Errorf("reuse for %s %s: %w", in.EntityType, in.EntityID, err)
	}
	return resp, nil
}

type autocompleteInput struct {
	ProjectID   string   `json:"project_id" jsonschema:"project ID / prefix"`
	CurrentPath []string `json:"current_path,omitempty" jsonschema:"qnames of the path built so far, e.g. [crm:E21_Person]"`
	Query       string   `json:"query,omitempty" jsonschema:"substring filter on suggestions"`
	ScopeClass  string   `json:"scope_class,omitempty" jsonschema:"root scope class qname when starting a path"`
	MaxResults  int      `json:"max_results,omitempty" jsonschema:"default 25"`
}

type autocompleteOutput struct {
	Suggestions []autocomplete.Suggestion `json:"suggestions"`
}

func ontologyAutocomplete(ctx context.Context, h Host, in autocompleteInput) (autocompleteOutput, error) {
	if _, err := resolveProject(ctx, h, in.ProjectID); err != nil {
		return autocompleteOutput{}, err
	}
	req := autocomplete.Request{
		ProjectID:               in.ProjectID,
		CurrentPath:             in.CurrentPath,
		Query:                   in.Query,
		ScopeClass:              in.ScopeClass,
		MaxResults:              in.MaxResults,
		IncludeParentProjects:   true,
		IncludeRangeSuggestions: true,
		Source:                  "mcp",
		SuperAdmin:              auth.FromContext(ctx).IsSuperAdmin,
	}
	if req.MaxResults <= 0 {
		req.MaxResults = 25
	}
	suggestions, err := h.Ontology.GetSuggestions(ctx, req)
	if err != nil {
		return autocompleteOutput{}, fmt.Errorf("autocomplete: %w", err)
	}
	return autocompleteOutput{Suggestions: suggestions}, nil
}

type formSchemaInput struct {
	ProjectID  string `json:"project_id" jsonschema:"project ID / prefix"`
	EntityType string `json:"entity_type" jsonschema:"one of: field, model, collection, category"`
	Lang       string `json:"lang,omitempty" jsonschema:"language code, default en"`
}

type formSchemaOutput struct {
	Schema *formschema.FormSchema `json:"schema"`
}

func getFormSchema(ctx context.Context, h Host, in formSchemaInput) (formSchemaOutput, error) {
	if _, err := resolveProject(ctx, h, in.ProjectID); err != nil {
		return formSchemaOutput{}, err
	}
	lang := in.Lang
	if lang == "" {
		lang = "en"
	}
	var schema *formschema.FormSchema
	switch strings.ToLower(in.EntityType) {
	case "field":
		schema = field.BuildFormSchema("create", nil, in.ProjectID, lang, h.Languages)
	case "model":
		schema = model.BuildFormSchema("create", nil, in.ProjectID, lang, h.Languages)
	case "collection":
		schema = collection.BuildFormSchema("create", nil, in.ProjectID, lang, h.Languages)
	case "category":
		schema = category.BuildCreateForm(in.ProjectID, lang, h.Languages)
	default:
		return formSchemaOutput{}, fmt.Errorf("unknown entity_type %q (field, model, collection, category)", in.EntityType)
	}
	return formSchemaOutput{Schema: schema}, nil
}

type vocabulariesInput struct {
	ProjectID string `json:"project_id" jsonschema:"project ID / prefix"`
}

type vocabulariesOutput struct {
	Vocabularies []vocabulary.VocabularyView  `json:"vocabularies"`
	ConceptLists []vocabulary.ConceptListView `json:"concept_lists"`
}

// vocabularyEntryRefSchema is a hand-authored, non-recursive override for
// domain.VocabularyEntryRef's output schema. The type is self-referential
// (BroaderPath []VocabularyEntryRef), which github.com/google/jsonschema-go's
// reflection-based inferencer rejects outright ("cycle detected for type
// ...") — it has no $ref-based mechanism for recursive Go types. The
// override flattens the leaf shape (everything but the recursive
// broader_path chain) for schema-advertisement purposes only; the actual
// JSON a tool call returns is unaffected; this only governs what's
// advertised as the tool's output schema.
var vocabularyEntryRefSchema = &jsonschema.Schema{
	Type: "object",
	Properties: map[string]*jsonschema.Schema{
		"id":            {Type: "string"},
		"vocabulary_id": {Type: "string"},
		"uri":           {Type: "string"},
		"label":         {Type: "object"},
		"scope_note":    {Type: "object"},
		"broader_uri":   {Type: "string"},
		"external_id":   {Type: "string"},
	},
}

// vocabulariesOutputSchema builds the list_vocabularies output schema,
// substituting vocabularyEntryRefSchema wherever domain.VocabularyEntryRef
// would otherwise be inferred (see its doc comment for why).
func vocabulariesOutputSchema() *jsonschema.Schema {
	s, err := jsonschema.For[vocabulariesOutput](&jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			reflect.TypeFor[domain.VocabularyEntryRef](): vocabularyEntryRefSchema,
		},
	})
	if err != nil {
		// Wiring-time fail-fast, sanctioned for Mount (see go-style.md) —
		// registerSemanticTools runs once at boot, before any request.
		panic(fmt.Sprintf("mcp: build list_vocabularies output schema: %v", err))
	}
	return s
}

func listVocabularies(ctx context.Context, h Host, in vocabulariesInput) (vocabulariesOutput, error) {
	if _, err := resolveProject(ctx, h, in.ProjectID); err != nil {
		return vocabulariesOutput{}, err
	}
	vocabs, err := h.Vocabulary.ListProjectVocabularies(ctx, in.ProjectID)
	if err != nil {
		return vocabulariesOutput{}, fmt.Errorf("list vocabularies: %w", err)
	}
	lists, err := h.Vocabulary.ListProjectConceptLists(ctx, in.ProjectID)
	if err != nil {
		return vocabulariesOutput{}, fmt.Errorf("list concept lists: %w", err)
	}
	return vocabulariesOutput{Vocabularies: vocabs, ConceptLists: lists}, nil
}

func registerSemanticTools(s *sdk.Server, h Host) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "entity_reuse",
		Description: "Where an entity is reused: included_in (membership in models/collections) and referenced_by (fields targeting it as expected value type).",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in reuseInput) (*sdk.CallToolResult, *detailview.FieldReuseResponse, error) {
		out, err := entityReuse(ctx, h, in)
		return nil, out, err
	})
	sdk.AddTool(s, &sdk.Tool{
		Name:        "ontology_autocomplete",
		Description: "Valid next steps for a CIDOC-CRM ontology path: only properties whose domain matches the current class lineage, and classes in a property's range. Same engine as the UI path builder.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in autocompleteInput) (*sdk.CallToolResult, autocompleteOutput, error) {
		out, err := ontologyAutocomplete(ctx, h, in)
		return nil, out, err
	})
	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_form_schema",
		Description: "The create-form schema for an entity type: every field, widget, validation, and option source an entity of this type accepts. Read this before proposing entity payloads.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in formSchemaInput) (*sdk.CallToolResult, formSchemaOutput, error) {
		out, err := getFormSchema(ctx, h, in)
		return nil, out, err
	})
	sdk.AddTool(s, &sdk.Tool{
		Name:         "list_vocabularies",
		Description:  "A project's vocabularies and concept lists (controlled term lists bindable to fields).",
		OutputSchema: vocabulariesOutputSchema(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in vocabulariesInput) (*sdk.CallToolResult, vocabulariesOutput, error) {
		out, err := listVocabularies(ctx, h, in)
		return nil, out, err
	})
}
