package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// fallbackScanLimit bounds the in-memory semantic-id scan for fallback entity lookups.
// Projects exceeding this limit should implement a store-level semantic_id index.
const fallbackScanLimit = 10000

type listEntitiesInput struct {
	ProjectID  string `json:"project_id" jsonschema:"project ID / prefix"`
	EntityType string `json:"entity_type" jsonschema:"one of: field, model, collection, category"`
	Status     string `json:"status,omitempty" jsonschema:"filter: draft or published"`
	Query      string `json:"query,omitempty" jsonschema:"substring search on names"`
	Limit      int    `json:"limit,omitempty" jsonschema:"max results, default 100"`
	Offset     int    `json:"offset,omitempty"`
}

// entitySummary is the compact row shape shared by all four entity types.
type entitySummary struct {
	ID                string              `json:"id"`
	SemanticID        string              `json:"semantic_id"`
	SystemName        string              `json:"system_name"`
	UIName            domain.Translations `json:"ui_name"`
	Status            string              `json:"status"`
	Deprecated        bool                `json:"deprecated,omitempty"`
	OntologyPath      string              `json:"ontology_path,omitempty"`
	ExpectedValueType string              `json:"expected_value_type,omitempty"`
	// BaseClass is the model/collection's scope class (OntologyScope.PrefixedName()), only set when OntologyScope is non-zero.
	BaseClass string `json:"base_class,omitempty"`
	// PathElements carries a field's full ontology path verbatim (OntologyPath above is the derived string form).
	PathElements []domain.PathElement `json:"path_elements,omitempty"`
}

type listEntitiesOutput struct {
	Entities []entitySummary `json:"entities"`
	// TotalCount is the total count of rows matching the search and status filter, independent of paging.
	TotalCount int64 `json:"total_count" jsonschema:"total rows matching the search and status filter"`
}

type getEntityInput struct {
	ProjectID  string `json:"project_id" jsonschema:"project ID / prefix"`
	EntityType string `json:"entity_type" jsonschema:"one of: field, model, collection, category"`
	ID         string `json:"id" jsonschema:"ULID, semantic ID (e.g. LAF.12), or system name"`
}

type getEntityOutput struct {
	EntityType string `json:"entity_type"`
	Entity     any    `json:"entity"`
}

// getEntityOutputSchema builds the get_entity output schema, overriding
// Entity (declared `any` because it holds a field/model/collection/category
// depending on entity_type) with an explicit permissive object schema.
// jsonschema.For infers an unrestricted/empty schema for an `any` field
// (jsonschema-go treats reflect.Interface as "no constraint" and emits
// Schema{}), which some MCP clients' schema validators reject outright as
// an invalid property schema. The override only changes what's advertised
// as the tool's output schema — the actual JSON a call returns is
// unaffected, since Entity is still serialized as whatever concrete value
// getEntity assigned to it.
var getEntityEntitySchema = &jsonschema.Schema{
	Type:                 "object",
	AdditionalProperties: &jsonschema.Schema{},
}

// getEntityOutputSchema builds the get_entity tool's OutputSchema,
// substituting getEntityEntitySchema for the inferred `any` schema on
// Entity (see its doc comment for why).
func getEntityOutputSchema() *jsonschema.Schema {
	s, err := jsonschema.For[getEntityOutput](nil)
	if err != nil {
		// Wiring-time fail-fast, sanctioned for Mount (see go-style.md) —
		// registerEntityTools runs once at boot, before any request.
		panic(fmt.Sprintf("mcp: build get_entity output schema: %v", err))
	}
	if s.Properties == nil {
		panic("mcp: get_entity output schema has no properties")
	}
	s.Properties["entity"] = getEntityEntitySchema
	return s
}

func listOpts(in listEntitiesInput) []domain.QueryOption {
	var opts []domain.QueryOption
	if in.Query != "" {
		opts = append(opts, domain.WithSearch(in.Query))
	}
	if in.Status != "" {
		opts = append(opts, domain.WithFilter("status", strings.ToLower(in.Status)))
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	opts = append(opts, domain.WithLimit(limit), domain.WithOffset(in.Offset))
	return opts
}

// keepStatus reports whether status matches the requested filter; an empty
// want keeps everything. status/want are plain strings — callers convert
// domain.Status (a defined string type) at the call site.
func keepStatus(status, want string) bool { return want == "" || status == want }

func listEntities(ctx context.Context, h Host, in listEntitiesInput) (listEntitiesOutput, error) {
	if _, err := resolveProject(ctx, h, in.ProjectID); err != nil {
		return listEntitiesOutput{}, err
	}
	out := listEntitiesOutput{Entities: []entitySummary{}}
	switch strings.ToLower(in.EntityType) {
	case "field":
		rows, total, err := h.Fields.List(ctx, in.ProjectID, listOpts(in)...)
		if err != nil {
			return out, fmt.Errorf("list fields: %w", err)
		}
		out.TotalCount = total
		for _, f := range rows {
			out.Entities = append(out.Entities, entitySummary{
				ID: f.ID, SemanticID: f.SemanticID, SystemName: f.SystemName,
				UIName: f.UIName, Status: string(f.Status), Deprecated: f.Deprecated,
				OntologyPath: f.OntologyPath(), ExpectedValueType: f.ExpectedValueType,
				PathElements: f.PathElements,
			})
		}
	case "model":
		rows, total, err := h.Models.List(ctx, in.ProjectID, listOpts(in)...)
		if err != nil {
			return out, fmt.Errorf("list models: %w", err)
		}
		out.TotalCount = total
		for _, m := range rows {
			s := entitySummary{
				ID: m.ID, SemanticID: m.SemanticID, SystemName: m.SystemName,
				UIName: m.UIName, Status: string(m.Status), Deprecated: m.Deprecated,
			}
			if m.OntologyScope.LocalName != "" {
				s.BaseClass = m.OntologyScope.PrefixedName()
			}
			out.Entities = append(out.Entities, s)
		}
	case "collection":
		rows, total, err := h.Collections.List(ctx, in.ProjectID, listOpts(in)...)
		if err != nil {
			return out, fmt.Errorf("list collections: %w", err)
		}
		out.TotalCount = total
		for _, c := range rows {
			s := entitySummary{
				ID: c.ID, SemanticID: c.SemanticID, SystemName: c.SystemName,
				UIName: c.UIName, Status: string(c.Status), Deprecated: c.Deprecated,
			}
			if c.OntologyScope.LocalName != "" {
				s.BaseClass = c.OntologyScope.PrefixedName()
			}
			out.Entities = append(out.Entities, s)
		}
	case "category":
		// The category store ignores paging/filters entirely — the tool
		// fetches everything, filters by status, then pages in memory.
		rows, err := h.Categories.List(ctx, in.ProjectID, listOpts(in)...)
		if err != nil {
			return out, fmt.Errorf("list categories: %w", err)
		}
		var filtered []*domain.Category
		for _, c := range rows {
			if !keepStatus(string(c.Status), in.Status) {
				continue
			}
			filtered = append(filtered, c)
		}
		out.TotalCount = int64(len(filtered))
		limit := in.Limit
		if limit <= 0 {
			limit = 100
		}
		start := in.Offset
		if start < 0 {
			start = 0
		}
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		for _, c := range filtered[start:end] {
			out.Entities = append(out.Entities, entitySummary{
				ID: c.ID, SemanticID: c.SemanticID, SystemName: c.SystemName,
				UIName: c.UIName, Status: string(c.Status), Deprecated: c.Deprecated,
			})
		}
	default:
		return out, fmt.Errorf("unknown entity_type %q (field, model, collection, category)", in.EntityType)
	}
	return out, nil
}

func getEntity(ctx context.Context, h Host, in getEntityInput) (getEntityOutput, error) {
	if _, err := resolveProject(ctx, h, in.ProjectID); err != nil {
		return getEntityOutput{}, err
	}
	out := getEntityOutput{EntityType: strings.ToLower(in.EntityType)}
	switch out.EntityType {
	case "field":
		f, err := h.Fields.GetByIdentifier(ctx, in.ProjectID, in.ID)
		if err != nil {
			// Not-found messages deliberately omit the underlying store error: unreadable, missing, and erroring lookups stay indistinguishable to the client (same shape as resolveProject).
			return out, fmt.Errorf("field %q not found in %s", in.ID, in.ProjectID)
		}
		out.Entity = f
	case "model":
		m, err := h.Models.Get(ctx, in.ProjectID, in.ID)
		if err == nil {
			out.Entity = m
			return out, nil
		}
		// ponytail: semantic-id fallback scans up to fallbackScanLimit entities
		// in memory; a store-level semantic_id lookup replaces this if any
		// project exceeds the limit.
		rows, _, err := h.Models.List(ctx, in.ProjectID, domain.WithLimit(fallbackScanLimit))
		if err != nil {
			return out, fmt.Errorf("list models: %w", err)
		}
		for _, m := range rows {
			if m.SemanticID == in.ID || m.SystemName == in.ID {
				out.Entity = m
				return out, nil
			}
		}
		return out, fmt.Errorf("model %q not found in %s", in.ID, in.ProjectID)
	case "collection":
		c, err := h.Collections.Get(ctx, in.ProjectID, in.ID)
		if err == nil {
			out.Entity = c
			return out, nil
		}
		rows, _, err := h.Collections.List(ctx, in.ProjectID, domain.WithLimit(fallbackScanLimit))
		if err != nil {
			return out, fmt.Errorf("list collections: %w", err)
		}
		for _, c := range rows {
			if c.SemanticID == in.ID || c.SystemName == in.ID {
				out.Entity = c
				return out, nil
			}
		}
		return out, fmt.Errorf("collection %q not found in %s", in.ID, in.ProjectID)
	case "category":
		c, err := h.Categories.Get(ctx, in.ProjectID, in.ID)
		if err == nil {
			out.Entity = c
			return out, nil
		}
		rows, err := h.Categories.List(ctx, in.ProjectID, domain.WithLimit(fallbackScanLimit))
		if err != nil {
			return out, fmt.Errorf("list categories: %w", err)
		}
		for _, c := range rows {
			if c.SemanticID == in.ID || c.SystemName == in.ID {
				out.Entity = c
				return out, nil
			}
		}
		return out, fmt.Errorf("category %q not found in %s", in.ID, in.ProjectID)
	default:
		return out, fmt.Errorf("unknown entity_type %q (field, model, collection, category)", in.EntityType)
	}
	return out, nil
}

func registerEntityTools(s *sdk.Server, h Host) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_entities",
		Description: "List fields, models, collections, or categories in a project. Supports substring search (query), status filter, and paging. The status filter is applied before paging for every entity type; total_count is the total number of rows matching the search and status filter, independent of limit/offset.",
	}, instrumented(h, "list_entities", func(ctx context.Context, req *sdk.CallToolRequest, in listEntitiesInput) (*sdk.CallToolResult, listEntitiesOutput, error) {
		out, err := listEntities(ctx, h, in)
		return nil, out, err
	}))
	sdk.AddTool(s, &sdk.Tool{
		Name:         "get_entity",
		Description:  "Get one entity's full detail, including ontology path elements and overrides where applicable. Accepts ULID, semantic ID, or system name.",
		OutputSchema: getEntityOutputSchema(),
	}, instrumented(h, "get_entity", func(ctx context.Context, req *sdk.CallToolRequest, in getEntityInput) (*sdk.CallToolResult, getEntityOutput, error) {
		out, err := getEntity(ctx, h, in)
		return nil, out, err
	}))
}
