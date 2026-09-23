package override

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Fingerprint identifies the content of one entity's pattern as the editor
// shows it: placement rows with their value-target refs, plus a model's
// collection placements. Row ids, timestamps and row order are excluded, so
// a save that changes nothing leaves the fingerprint alone. The editor
// carries it from load to save; the save refuses a stale one (409).
func Fingerprint(rows []domain.FieldOverride, refs map[int64][]domain.OverrideRef, placements []domain.CollectionPlacement) string {
	lines := make([]string, 0, len(rows)+len(placements))
	for _, r := range rows {
		lines = append(lines, rowLine(r, refs[r.ID]))
	}
	slices.Sort(lines)
	placementLines := make([]string, 0, len(placements))
	for _, p := range placements {
		placementLines = append(placementLines, placementLine(p))
	}
	slices.Sort(placementLines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n") + "\n--\n" + strings.Join(placementLines, "\n")))
	return hex.EncodeToString(sum[:16])
}

// rowLine renders one override row (plus its sorted refs) as a deterministic
// string. expected_value_type is deliberately excluded: it is a Field-level
// structural property, not part of FieldOverride (see domain-model design
// contract), so it plays no part in this content hash.
func rowLine(r domain.FieldOverride, refs []domain.OverrideRef) string {
	refCopy := slices.Clone(refs)
	slices.SortFunc(refCopy, func(a, b domain.OverrideRef) int {
		return cmp.Or(cmp.Compare(a.RefType, b.RefType), cmp.Compare(a.Position, b.Position), cmp.Compare(a.TargetID, b.TargetID))
	})
	refParts := make([]string, 0, len(refCopy))
	for _, ref := range refCopy {
		refParts = append(refParts, fmt.Sprintf("%s=%s@%d", ref.RefType, ref.TargetID, ref.Position))
	}
	return strings.Join([]string{
		r.FieldID, r.CategoryID, r.PartOfCollectionID,
		fmt.Sprint(r.Position), fmt.Sprint(r.CollectionOrder),
		translationsKey(r.DisplayName), translationsKey(r.Description), translationsKey(r.CollectionName),
		r.SetValue,
		fmt.Sprint(r.IsRequired), fmt.Sprint(r.MinOccurs), maxKey(r.MaxOccurs),
		fmt.Sprint(r.IsHidden), r.Visibility,
		strings.Join(refParts, ","),
	}, "|")
}

func placementLine(p domain.CollectionPlacement) string {
	return strings.Join([]string{
		p.CategoryID, p.CollectionID,
		fmt.Sprint(p.IsRequired), fmt.Sprint(p.MinOccurs), maxKey(p.MaxOccurs), fmt.Sprint(p.IsHidden),
	}, "|")
}

// translationsKey renders a Translations map deterministically (Go's JSON
// encoder sorts map keys).
func translationsKey(t domain.Translations) string {
	if len(t) == 0 {
		return ""
	}
	b, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return string(b)
}

func maxKey(v *int) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(*v)
}
