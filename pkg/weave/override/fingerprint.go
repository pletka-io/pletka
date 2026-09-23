package override

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
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
		refParts = append(refParts, joinFields(ref.RefType, ref.TargetID, fmt.Sprint(ref.Position)))
	}
	// The leading segments are category, collection, field, position, so
	// sorting the rendered lines sorts rows in the order the design fixes.
	return joinFields(
		r.CategoryID, r.PartOfCollectionID, r.FieldID,
		fmt.Sprint(r.Position), fmt.Sprint(r.CollectionOrder),
		translationsKey(r.DisplayName), translationsKey(r.Description), translationsKey(r.CollectionName),
		r.SetValue,
		fmt.Sprint(r.IsRequired), fmt.Sprint(r.MinOccurs), maxKey(r.MaxOccurs),
		fmt.Sprint(r.IsHidden), r.Visibility,
		joinFields(refParts...),
	)
}

func placementLine(p domain.CollectionPlacement) string {
	return joinFields(
		p.CategoryID, p.CollectionID,
		fmt.Sprint(p.IsRequired), fmt.Sprint(p.MinOccurs), maxKey(p.MaxOccurs), fmt.Sprint(p.IsHidden),
	)
}

// joinFields renders parts as one unambiguous line. Every part is quoted
// first, so a separator, a quote or a newline inside an id, a set value or a
// translation cannot fake a field or row boundary: without quoting, a row
// whose collection id is "a|b" hashes the same as one whose category id ends
// in "|a" and whose collection id is "b".
func joinFields(parts ...string) string {
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		quoted = append(quoted, strconv.Quote(p))
	}
	return strings.Join(quoted, "|")
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
