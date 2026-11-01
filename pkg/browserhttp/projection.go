package browserhttp

import (
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

const originSourceKey = "source"

type inventoryRow struct {
	browserinput.InventoryRow
	OriginLabel i18n.LocalizedText `json:"origin_label"`
}

func projectRows(value browserinput.Inventory) []inventoryRow {
	rows := make([]inventoryRow, 0, len(value.Items))
	for _, r := range value.Items {
		if value.Version != "" || r.SourceVersion != "" {
			r.Status = "published"
		}
		rows = append(rows, inventoryRow{r, originListLabel(r.Origin, r.NotReferenced)})
	}
	return rows
}

func originListLabel(origin domain.Origin, notReferenced bool) i18n.LocalizedText {
	source := origin.SourceProjectID
	if origin.SourceEntityID != "" {
		if source != "" {
			source += " · "
		}
		source += origin.SourceEntityID
	}
	switch origin.Kind {
	case domain.OriginForked:
		if source == "" {
			return i18n.L("common.origin_state.adapted", "Adapted")
		}
		return i18n.LF("common.origin_state.adapted_from", "Adapted from {source}", map[string]string{originSourceKey: source})
	case domain.OriginAdopted:
		if notReferenced {
			if source == "" {
				return i18n.L("common.origin_state.adopted_unused", "Adopted · not yet referenced")
			}
			return i18n.LF("common.origin_state.adopted_unused_from", "Adopted from {source} · not yet referenced", map[string]string{originSourceKey: source})
		}
		if source == "" {
			return i18n.L("common.origin_state.adopted", "Adopted")
		}
		return i18n.LF("common.origin_state.adopted_from", "Adopted from {source}", map[string]string{originSourceKey: source})
	case domain.OriginAdoptedReference:
		if source == "" {
			return i18n.L("common.origin_state.adopted_reference", "Adopted (by reference)")
		}
		return i18n.LF("common.origin_state.adopted_reference_from", "Adopted by reference from {source}", map[string]string{originSourceKey: source})
	case domain.OriginInherited:
		if source == "" {
			return i18n.L("common.origin_state.inherited", "Inherited")
		}
		return i18n.LF("common.origin_state.inherited_from", "Inherited from {source}", map[string]string{originSourceKey: source})
	case domain.OriginOwn:
		return i18n.L("common.origin_state.own", "Own")
	default:
		return i18n.LocalizedText{}
	}
}
