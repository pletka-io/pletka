package detailview

import (
	"context"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	weaveroutes "github.com/pletka-io/pletka/pkg/weave/routes"
)

func (h *Handler) loadDetail(ctx context.Context, scope auth.ReadScope, project, kind, id string) (projectedDetail, error) {
	if kind != detailModel && kind != detailCollection && kind != detailField && kind != detailConceptList {
		return projectedDetail{}, apierror.BadRequest("unsupported entity type")
	}
	if h.source == nil || !scope.Valid() {
		return projectedDetail{}, fmt.Errorf("detail read requires a bounded source and explicit scope")
	}
	resource := h.projectResource(ctx, project)
	if resource.ID != project || !auth.FromContext(ctx).Can(auth.ProjectRead, resource, nil) {
		return projectedDetail{}, apierror.NotFound("project not found")
	}
	input, err := h.source.Detail(ctx, scope, project, strings.ReplaceAll(kind, "-", "_"), id)
	if err != nil {
		return projectedDetail{}, err
	}
	value, err := projectDetail(input)
	if err != nil {
		return projectedDetail{}, err
	}
	if value.Consumer.ProjectID() != project || value.Consumer.Version() != scope.Version() || value.Response.Entity.ID != id || value.Response.Entity.Type != kind {
		return projectedDetail{}, fmt.Errorf("browser source returned a different selection")
	}
	return value, nil
}

// decorateDetail adds current actions to captured display content. Only editable
// working views can consult current usage for lifecycle actions; released
// responses never obtain content through the aggregate weave store.
func (h *Handler) decorateDetail(ctx context.Context, value projectedDetail) *Response {
	r := value.Response
	e := &r.Entity
	project, version := value.Consumer.ProjectID(), value.Consumer.Version()
	resource := h.projectResource(ctx, project)
	canEdit := value.Consumer.IsDraft() && h.hasProjectEdit(ctx, project)
	editable := canEdit && (e.Origin.Kind == domain.OriginOwn || e.Origin.Kind == domain.OriginForked)
	kind := e.Type
	plural := entitySegment(kind)
	base := fmt.Sprintf("/projects/%s/%s/%s", project, plural, e.ID)
	viewBase := fmt.Sprintf("%s/%s/entity-view/%s/%s", weaveroutes.ProjectBase, project, kind, e.ID)
	r.Release = releaseView(project, version, h.hasProjectEdit(ctx, project))
	r.Capabilities = ViewCapabilities{Editable: editable}
	c := &r.Capabilities
	if kind == detailConceptList {
		apiBase := "/api/v2" + base
		c.MetadataURL = capabilityURL(editable, apiBase+"/form-schema")
		c.DeleteURL = capabilityURL(editable, apiBase)
		c.ConceptList = &ConceptListCap{
			SearchEntriesURL: capabilityURL(editable, apiBase+"/source-entries/search"), AddEntryURL: capabilityURL(editable, apiBase+"/entries"), UpdateEntryURL: capabilityURL(editable, apiBase+"/entries/{id}"), RemoveEntryURL: capabilityURL(editable, apiBase+"/entries/{id}"), ReorderURL: capabilityURL(editable, apiBase+"/entries/reorder"), CreateTermURL: capabilityURL(editable, apiBase+"/terms"), SealURL: capabilityURL(editable, apiBase+"/sealed"), BroaderURLTemplate: capabilityURL(editable, apiBase+"/terms/{conceptID}/broader"), IsClosed: value.Closed, SourceName: value.ConceptSource, HasRemoteSource: value.RemoteSource,
			// The legacy exporter still loads hierarchy edges from live rows.
			SkosURL: capabilityURL(value.Consumer.IsDraft(), apiBase+"/skos"),
		}
		return &r
	}
	e.PublicationState = h.pubState(ctx, project, kind, e.ID, version)
	c.MetadataURL = capabilityURL(editable, fmt.Sprintf("/projects/%s/form-schema/%s?mode=edit&entity_id=%s", project, kind, e.ID))
	c.SearchFieldsURL = withVersion(fmt.Sprintf("/projects/%s/fields/search", project), version)
	c.Derivatives = h.derivativesFor(ctx, auth.FromContext(ctx), resource, plural, e.ID, version)
	c.ReuseURL = capabilityURL(value.Consumer.IsDraft(), viewBase+"/reuse")
	r.StatsURL = withVersion(viewBase+"/stats", version)
	if kind == detailModel || kind == detailCollection {
		c.CanAddFields, c.CanReorder = editable, editable
		c.SaveURL = capabilityURL(editable, base+"/overrides")
		c.ForkLabel = i18n.L("common.capabilities.adapt", "Adapt to edit")
		c.ForkPending = i18n.L("common.capabilities.adapt_pending", "Adapting…")
		if canEdit && e.Origin.Kind == domain.OriginAdopted {
			c.ForkURL = base + "/fork"
		}
	}
	if kind == detailModel {
		c.AdoptLabel = i18n.L("common.capabilities.adopt", "Adopt")
		if canEdit && e.Origin.Kind == domain.OriginInherited {
			c.AdoptURL = base + "/adopt"
		}
		c.ExamplesSchemaURL = capabilityURL(value.Consumer.IsDraft(), fmt.Sprintf("/projects/%s/entity-list-schema/example?entity_type=model&entity_id=%s", project, e.ID))
	}
	if editable {
		inUse := h.currentInUse(ctx, project, kind, e.ID)
		c.DeleteURL, c.DeprecateURL, c.ActivateURL = lifecycleCaps(editable, inUse, version, base, e.Deprecated)
	}
	return &r
}

func (h *Handler) currentInUse(ctx context.Context, project, kind, id string) bool {
	switch kind {
	case detailField:
		counts, err := h.weave.WeaveFields().CountUsage(ctx, id, project)
		return err != nil || counts.ModelsUsing+counts.CollectionsUsing > 0
	case detailCollection:
		usage, err := h.weave.Collections().ListUsage(ctx, id, project)
		if err != nil || len(usage) > 0 {
			return true
		}
	}
	usage, err := h.weave.Models().ListUsage(ctx, id)
	return err != nil || len(usage.Models)+len(usage.Collections) > 0
}
