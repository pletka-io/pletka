package detailview

import (
	"net/url"
	"sort"

	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

func originDetailLabel(origin pkgdomain.Origin) i18n.LocalizedText {
	source := origin.SourceProjectID
	if origin.SourceEntityID != "" {
		if source != "" {
			source += " · "
		}
		source += origin.SourceEntityID
	}
	switch origin.Kind {
	case pkgdomain.OriginForked:
		if source == "" {
			return i18n.L("common.origin_state.adapted", "Adapted")
		}
		return i18n.LF("common.origin_state.adapted_from", "Adapted from {source}", map[string]string{originSourceKey: source})
	case pkgdomain.OriginAdopted:
		if source == "" {
			return i18n.L("common.origin_state.adopted", "Adopted")
		}
		return i18n.LF("common.origin_state.adopted_from", "Adopted from {source}", map[string]string{originSourceKey: source})
	case pkgdomain.OriginAdoptedReference:
		if source == "" {
			return i18n.L("common.origin_state.adopted_reference", "Adopted (by reference)")
		}
		return i18n.LF("common.origin_state.adopted_reference_from", "Adopted by reference from {source}", map[string]string{originSourceKey: source})
	case pkgdomain.OriginInherited:
		if source == "" {
			return i18n.L("common.origin_state.inherited", "Inherited")
		}
		return i18n.LF("common.origin_state.inherited_from", "Inherited from {source}", map[string]string{originSourceKey: source})
	case pkgdomain.OriginOwn:
		return i18n.L("common.origin_state.own", "Own")
	default:
		return i18n.LocalizedText{}
	}
}

func originDetailHelp(origin pkgdomain.Origin) i18n.LocalizedText {
	switch origin.Kind {
	case pkgdomain.OriginInherited:
		return i18n.L("common.origin_help.inherited", "This entity is available through parent inheritance. Adopt it to pin the upstream version into this project.")
	case pkgdomain.OriginAdopted:
		return i18n.L("common.origin_help.adopted", "This entity is pinned from upstream and stays read-only here. Adapt it to make local changes without changing the source.")
	case pkgdomain.OriginAdoptedReference:
		return i18n.L("common.origin_help.adopted_reference", "This entity is referenced by overrides in this project but has no explicit receipt. Adopt it for a deliberate claim, or Adapt it to make local changes.")
	case pkgdomain.OriginForked:
		return i18n.L("common.origin_help.adapted", "This is your local editable copy. Upstream provenance is still kept so you can trace where it came from.")
	default:
		return i18n.LocalizedText{}
	}
}

type statEntry struct {
	name  string
	count int
}

// filterVisibleFields drops fields marked IsHidden before they're rendered
// into the read-only detail/browse contract. Applied before
// any derived computation — field counts, ComputeSharedPathPrefix — so
// those stay consistent with what's actually rendered. The override EDITOR
// (pkg/weave/project/override_read.go) intentionally does not use this: it
// keeps showing hidden fields (strikethrough + Hidden badge) so curators
// can unhide them.
func filterVisibleFields(fields []pkgdomain.ResolvedField) []pkgdomain.ResolvedField {
	visible := make([]pkgdomain.ResolvedField, 0, len(fields))
	for _, f := range fields {
		if f.IsHidden {
			continue
		}
		visible = append(visible, f)
	}
	return visible
}

func computeCollectionStats(fields []pkgdomain.ResolvedField, sections []ViewSection) pkgdomain.ModelViewStats {
	required := 0
	valueTypeCounts := make(map[string]int)
	for _, f := range fields {
		if f.IsRequired {
			required++
		}
		if f.ExpectedValueType != "" {
			valueTypeCounts[f.ExpectedValueType]++
		}
	}
	catBreakdown, fieldBreakdown, fieldScopeBreakdown, classBreakdown, propBreakdown, scopesCount := computeCollectionBreakdowns(fields, sections)
	return pkgdomain.ModelViewStats{
		TotalFields:          len(fields),
		TotalCategories:      len(sections),
		RequiredFields:       required,
		OptionalFields:       len(fields) - required,
		ValueTypeCounts:      valueTypeCounts,
		ScopesCount:          scopesCount,
		CategoriesBreakdown:  catBreakdown,
		FieldsBreakdown:      fieldBreakdown,
		FieldScopesBreakdown: fieldScopeBreakdown,
		ClassesBreakdown:     classBreakdown,
		PropertiesBreakdown:  propBreakdown,
	}
}

func computeCollectionBreakdowns(fields []pkgdomain.ResolvedField, sections []ViewSection) (
	catBreakdown, fieldBreakdown, fieldScopeBreakdown, classBreakdown, propBreakdown []pkgdomain.StatItem,
	scopesCount int,
) {
	catItems := make([]pkgdomain.StatItem, 0, len(sections))
	maxCatCount := 0
	for _, s := range sections {
		count := 0
		for _, item := range s.Items {
			count += len(item.Fields)
		}
		if count > maxCatCount {
			maxCatCount = count
		}
		catItems = append(catItems, pkgdomain.StatItem{ID: s.ID, Name: s.Name.Get("en", s.ID), Count: count})
	}
	for i := range catItems {
		if maxCatCount > 0 {
			catItems[i].Percentage = (catItems[i].Count * 100) / maxCatCount
		}
	}
	sort.Slice(catItems, func(i, j int) bool {
		if catItems[i].Count == catItems[j].Count {
			return catItems[i].ID < catItems[j].ID
		}
		return catItems[i].Count > catItems[j].Count
	})
	if len(catItems) > 0 {
		catBreakdown = catItems
	}

	fieldCounts := make(map[string]statEntry)
	fieldScopes := make(map[string]int)
	classUsage := make(map[string]statEntry)
	propUsage := make(map[string]statEntry)

	for _, f := range fields {
		fc := fieldCounts[f.ID]
		fc.count++
		if fc.name == "" {
			fc.name = f.DisplayName.Get("en", f.SemanticID)
		}
		fieldCounts[f.ID] = fc

		firstClass := true
		seenClasses := make(map[string]bool)
		seenProps := make(map[string]bool)

		for _, elem := range f.PathElements {
			key := elem.LocalName
			if elem.Prefix != "" {
				key = elem.Prefix + ":" + elem.LocalName
			}

			switch elem.Type {
			case "class":
				if !seenClasses[key] {
					seenClasses[key] = true
					e := classUsage[key]
					e.count++
					if e.name == "" {
						e.name = elem.LocalName
					}
					classUsage[key] = e
				}
				if firstClass {
					firstClass = false
					fieldScopes[key]++
				}
			case "property":
				if !seenProps[key] {
					seenProps[key] = true
					e := propUsage[key]
					e.count++
					if e.name == "" {
						e.name = elem.LocalName
					}
					propUsage[key] = e
				}
			}
		}
	}

	scopesCount = len(fieldScopes)

	fieldBreakdown = buildStatItems(fieldCounts)
	fieldScopeBreakdown = buildStatItemsFromCounts(fieldScopes)
	classBreakdown = buildStatItems(classUsage)
	propBreakdown = buildStatItems(propUsage)

	return
}

func buildStatItems(m map[string]statEntry) []pkgdomain.StatItem {
	if len(m) == 0 {
		return nil
	}

	maxCount := 0
	for _, v := range m {
		if v.count > maxCount {
			maxCount = v.count
		}
	}

	items := make([]pkgdomain.StatItem, 0, len(m))
	for id, v := range m {
		pct := 0
		if maxCount > 0 {
			pct = (v.count * 100) / maxCount
		}
		items = append(items, pkgdomain.StatItem{
			ID:         id,
			Name:       v.name,
			Count:      v.count,
			Percentage: pct,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].ID < items[j].ID
		}
		return items[i].Count > items[j].Count
	})

	return items
}

func buildStatItemsFromCounts(m map[string]int) []pkgdomain.StatItem {
	if len(m) == 0 {
		return nil
	}

	maxCount := 0
	for _, c := range m {
		if c > maxCount {
			maxCount = c
		}
	}

	items := make([]pkgdomain.StatItem, 0, len(m))
	for name, count := range m {
		pct := 0
		if maxCount > 0 {
			pct = (count * 100) / maxCount
		}
		items = append(items, pkgdomain.StatItem{
			ID:         name,
			Name:       name,
			Count:      count,
			Percentage: pct,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].ID < items[j].ID
		}
		return items[i].Count > items[j].Count
	})

	return items
}

func sortSectionsByOrder(sections []ViewSection) {
	sort.Slice(sections, func(i, j int) bool {
		if sections[i].CanonicalOrder == sections[j].CanonicalOrder {
			return sections[i].ID < sections[j].ID
		}
		return sections[i].CanonicalOrder < sections[j].CanonicalOrder
	})
}

func withVersion(raw, activeVersion string) string {
	if activeVersion == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("version", activeVersion)
	u.RawQuery = q.Encode()
	return u.String()
}

func adoptionLabel(adoption pkgdomain.Adoption) pkgdomain.Translations {
	label := adoption.SourceEntityID
	if adoption.EntityType != "" {
		label = adoption.EntityType + " · " + adoption.SourceEntityID
	}
	return pkgdomain.Translations{"en": label, "nl": label}
}

const (
	originSourceKey         = "source"
	detailModels            = "models"
	detailModel             = "model"
	detailCollection        = "collection"
	detailField             = "field"
	detailConceptList       = "concept-list"
	detailConceptListRecord = "concept_list"
)

func entitySegment(kind string) string {
	return map[string]string{detailModel: detailModels, detailCollection: "collections", detailField: "fields", "category": "categories", detailConceptList: "concept-lists", detailConceptListRecord: "concept-lists"}[kind]
}
