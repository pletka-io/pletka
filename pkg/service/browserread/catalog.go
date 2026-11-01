package browserread

import (
	"context"
	"fmt"
	"net/url"

	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsession"
)

const (
	catalogConceptList = "concept_list"
	catalogExample     = "example"
)

// catalogInventory lists owned concept lists/examples. Labels resolve before
// filtering and paging; only compact reference/label data crosses that boundary.
func catalogInventory(ctx context.Context, root *readsession.Session, kind string, q browserinput.InventoryQuery) (browserinput.Inventory, error) {
	var out browserinput.Inventory
	if _, err := root.ProjectHeader(ctx); err != nil {
		return out, err
	}
	members, err := root.Membership(ctx)
	if err != nil {
		return out, err
	}
	refs, err := root.CatalogReferences(ctx, kind)
	if err != nil {
		return out, err
	}
	owners := map[inventoryLocal]string{}
	for _, m := range members {
		owners[inventoryLocal{m.Kind, m.ID}] = m.Source.ProjectID
	}
	type group struct{ project, kind string }
	groups := map[group][]string{}
	for _, ref := range refs {
		owner := owners[inventoryLocal{ref.Kind, ref.ID}]
		if owner == "" {
			owner = root.ProjectID()
		}
		key := group{owner, ref.Kind}
		groups[key] = append(groups[key], ref.ID)
	}
	if len(groups) > 128 {
		return out, fmt.Errorf("catalog source budget exceeded")
	}
	labels := map[string]readsession.CatalogLabel{}
	for key, ids := range groups {
		bound := root
		if key.project != root.ProjectID() {
			bound, err = root.ProjectDependency(ctx, key.project)
			if err != nil {
				return out, err
			}
			if _, err = bound.ProjectHeader(ctx); err != nil {
				return out, err
			}
		}
		values, err := bound.CatalogLabels(ctx, key.kind, ids)
		if err != nil {
			return out, err
		}
		for id, label := range values {
			labels[key.kind+":"+id] = label
		}
	}
	out, err = root.CatalogPage(ctx, kind, labels, q)
	if err != nil {
		return browserinput.Inventory{}, err
	}
	version := root.Scope().Version()
	for i := range out.Items {
		r := &out.Items[i]
		r.Owned = true
		path := "concept-lists"
		if kind == catalogExample {
			path = "examples"
			r.Title = r.UIName
			r.UIName = nil
			switch r.EntityType {
			case "model":
				r.EntityTypeLabel = "Model"
			case "collection":
				r.EntityTypeLabel = "Collection"
			}
		}
		r.CurrentURL = "/projects/" + url.PathEscape(root.ProjectID()) + "/" + path + "/" + url.PathEscape(r.ID)
		if version != "" {
			r.CurrentURL += "?version=" + url.QueryEscape(version)
		}
	}
	return out, nil
}
