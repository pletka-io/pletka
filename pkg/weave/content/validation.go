package content

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/frontendmanifest"
)

// ValidateWidgetReferences loads content schemas from the given sources and
// verifies every emitted block type is declared as a frontend content widget.
func ValidateWidgetReferences(ctx context.Context, sources []ContentSource, catalog *frontendmanifest.Catalog) error {
	if catalog == nil {
		return fmt.Errorf("content: frontend manifest catalog is required")
	}
	pages, err := NewLoader(nil).LoadFromSources(ctx, sources)
	if err != nil {
		return fmt.Errorf("content: load pages for validation: %w", err)
	}

	var missing []string
	for slug, byLang := range pages {
		for lang, schema := range byLang {
			for _, block := range schema.Blocks {
				if block.Type == "" {
					continue
				}
				if !catalog.Has(frontendmanifest.KindContentWidget, block.Type) {
					missing = append(missing, fmt.Sprintf("%s.%s: contentWidgets %q", slug, lang, block.Type))
				}
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("content pages reference frontend widgets missing from manifest:\n- %s", strings.Join(missing, "\n- "))
	}
	return nil
}
