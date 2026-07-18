package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// reportCheckOrphans, reportCheckPathCollisions, reportCheckNameCollisions
// name the three project_report checks. checks is empty (all three run) or a
// subset of these names.
const (
	reportCheckOrphans        = "orphans"
	reportCheckPathCollisions = "path_collisions"
	reportCheckNameCollisions = "name_collisions"
)

type projectReportInput struct {
	ProjectID string   `json:"project_id" jsonschema:"project ID / prefix"`
	Checks    []string `json:"checks,omitempty" jsonschema:"subset of: orphans, path_collisions, name_collisions; empty = all"`
}

// reportFieldRef is a field row's compact identity plus its current owners,
// as carried in a project_report finding.
type reportFieldRef struct {
	SemanticID  string   `json:"semantic_id"`
	SystemName  string   `json:"system_name"`
	Models      []string `json:"models,omitempty"`
	Collections []string `json:"collections,omitempty"`
}

// collisionGroup is a set of >=2 fields sharing the same ontology path or
// system name.
type collisionGroup struct {
	Value     string           `json:"value"` // the shared path or name
	Fields    []reportFieldRef `json:"fields"`
	SameModel bool             `json:"same_model"`
}

type projectReportOutput struct {
	Orphans        []reportFieldRef `json:"orphans,omitempty"`
	PathCollisions []collisionGroup `json:"path_collisions,omitempty"`
	NameCollisions []collisionGroup `json:"name_collisions,omitempty"`
	FieldsScanned  int64            `json:"fields_scanned"`
	Truncated      bool             `json:"truncated,omitempty"` // scan capped at fallbackScanLimit
}

// reportChecks validates in.Checks against the three known checks and
// returns the requested set; an empty input requests all three.
func reportChecks(checks []string) (map[string]bool, error) {
	valid := map[string]bool{
		reportCheckOrphans:        true,
		reportCheckPathCollisions: true,
		reportCheckNameCollisions: true,
	}
	if len(checks) == 0 {
		out := make(map[string]bool, len(valid))
		for k := range valid {
			out[k] = true
		}
		return out, nil
	}
	out := make(map[string]bool, len(checks))
	for _, c := range checks {
		c = strings.ToLower(strings.TrimSpace(c))
		if !valid[c] {
			return nil, fmt.Errorf("unknown check %q (orphans, path_collisions, name_collisions)", c)
		}
		out[c] = true
	}
	return out, nil
}

// reportFieldRefFor builds a reportFieldRef for f from its usage refs.
func reportFieldRefFor(f *domain.Field, usage domain.FieldUsageList) reportFieldRef {
	return reportFieldRef{
		SemanticID:  f.SemanticID,
		SystemName:  f.SystemName,
		Models:      semanticIDs(usage.Models),
		Collections: semanticIDs(usage.Collections),
	}
}

// reportOrphans returns fields with no model/collection owners at all,
// ordered by SemanticID ascending.
func reportOrphans(rows []*domain.Field, refs map[string]domain.FieldUsageList) []reportFieldRef {
	var out []reportFieldRef
	for _, f := range rows {
		usage := refs[f.ID]
		if len(usage.Models) == 0 && len(usage.Collections) == 0 {
			out = append(out, reportFieldRefFor(f, usage))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SemanticID < out[j].SemanticID })
	return out
}

// reportGroups groups rows by keyFn(f), dropping empty keys and groups of
// size 1, and computes SameModel per group: true when any owning model's
// semantic ID appears on >=2 distinct fields within the group (ownership may
// be cross-project — semantic IDs are compared as-is). Groups are ordered by
// member count descending, then Value ascending; fields within a group by
// SemanticID ascending.
func reportGroups(rows []*domain.Field, refs map[string]domain.FieldUsageList, keyFn func(*domain.Field) string) []collisionGroup {
	groups := map[string][]*domain.Field{}
	var order []string
	for _, f := range rows {
		key := keyFn(f)
		if key == "" {
			continue
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], f)
	}

	var out []collisionGroup
	for _, value := range order {
		members := groups[value]
		if len(members) <= 1 {
			continue
		}

		modelFieldCount := map[string]int{}
		for _, f := range members {
			seen := map[string]bool{}
			for _, m := range refs[f.ID].Models {
				if seen[m.SemanticID] {
					continue
				}
				seen[m.SemanticID] = true
				modelFieldCount[m.SemanticID]++
			}
		}
		sameModel := false
		for _, count := range modelFieldCount {
			if count >= 2 {
				sameModel = true
				break
			}
		}

		fieldRefs := make([]reportFieldRef, 0, len(members))
		for _, f := range members {
			fieldRefs = append(fieldRefs, reportFieldRefFor(f, refs[f.ID]))
		}
		sort.Slice(fieldRefs, func(i, j int) bool { return fieldRefs[i].SemanticID < fieldRefs[j].SemanticID })

		out = append(out, collisionGroup{Value: value, Fields: fieldRefs, SameModel: sameModel})
	}

	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Fields) != len(out[j].Fields) {
			return len(out[i].Fields) > len(out[j].Fields)
		}
		return out[i].Value < out[j].Value
	})
	return out
}

func projectReport(ctx context.Context, h Host, in projectReportInput) (projectReportOutput, error) {
	ctx, _, err := resolveProject(ctx, h, in.ProjectID)
	if err != nil {
		return projectReportOutput{}, err
	}
	requested, err := reportChecks(in.Checks)
	if err != nil {
		return projectReportOutput{}, err
	}

	// ponytail: semantic-id fallback scans up to fallbackScanLimit entities
	// in memory; a store-level lint query replaces this if any project
	// exceeds the limit.
	rows, total, err := h.Fields.List(ctx, in.ProjectID, domain.WithLimit(fallbackScanLimit))
	if err != nil {
		return projectReportOutput{}, fmt.Errorf("list fields: %w", err)
	}
	ids := make([]string, len(rows))
	for i, f := range rows {
		ids[i] = f.ID
	}
	refs, err := h.Fields.BatchUsageRefs(ctx, in.ProjectID, ids)
	if err != nil {
		return projectReportOutput{}, fmt.Errorf("field ownership: %w", err)
	}

	out := projectReportOutput{
		FieldsScanned: int64(len(rows)),
		Truncated:     total > int64(len(rows)),
	}
	if requested[reportCheckOrphans] {
		out.Orphans = reportOrphans(rows, refs)
	}
	if requested[reportCheckPathCollisions] {
		out.PathCollisions = reportGroups(rows, refs, func(f *domain.Field) string { return f.OntologyPath() })
	}
	if requested[reportCheckNameCollisions] {
		out.NameCollisions = reportGroups(rows, refs, func(f *domain.Field) string { return f.SystemName })
	}
	return out, nil
}

func registerReportTools(s *sdk.Server, h Host) {
	sdk.AddTool(s, &sdk.Tool{
		Name: "project_report",
		Description: "Lint a project's fields for three issues, selectable via checks (default: all three). " +
			"orphans: fields placed on no model or collection. path_collisions: >=2 fields sharing the same " +
			"ontology path (fields with no path are excluded). name_collisions: >=2 fields sharing the same " +
			"system name (empty names excluded). Each collision group carries same_model: true when one owning " +
			"model (by semantic ID, regardless of project) accounts for >=2 of the group's fields — those " +
			"collapse into identical triples on export and are the ones worth fixing; cross-model sharing of the " +
			"same path/name is legitimate anchoring, not a defect. Scans up to 10000 fields (fallbackScanLimit); " +
			"truncated=true and fields_scanned report when the project exceeds that cap. The report scans fields of all statuses, including drafts and deprecated fields.",
	}, instrumented(h, "project_report", func(ctx context.Context, req *sdk.CallToolRequest, in projectReportInput) (*sdk.CallToolResult, projectReportOutput, error) {
		out, err := projectReport(ctx, h, in)
		return nil, out, err
	}))
}
