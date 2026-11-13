package gitmaterializer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Entity types of models/<M>/grafts.yaml and of the opaque files under
// generators/.
const (
	entityTypeModelGrafts   = "model_grafts"
	entityTypeGeneratorFile = "generator_file"
)

// graftDoc is one entry of models/<M>/grafts.yaml.
type graftDoc struct {
	IsHidden    bool                `json:"is_hidden,omitempty"`
	MaxOccurs   *int                `json:"max_occurs,omitempty"`
	DisplayName domain.Translations `json:"display_name,omitempty"`
}

// hydrateGraftsTx inserts every grafts.yaml of the snapshot. The caller has
// cleared the project's rows in the same transaction.
func (m *Materializer) hydrateGraftsTx(ctx context.Context, q *sqlcgen.Queries, projectID string, snap *ProjectSnapshot) error {
	models := map[string]bool{}
	for _, item := range snap.Entities.Models {
		models[item.EntityID] = true
	}
	for _, item := range snap.Entities.Grafts {
		if !models[item.OwnerID] {
			return fmt.Errorf("%s: model %s is not in the tree", item.Path, item.OwnerID)
		}
		doc, err := decodeTreeFile[map[string]graftDoc](item.Path, item.Payload)
		if err != nil {
			return err
		}
		addrs := make([]string, 0, len(doc))
		for a := range doc {
			addrs = append(addrs, a)
		}
		sort.Strings(addrs)
		for _, a := range addrs {
			g := doc[a]
			var max *int32
			if g.MaxOccurs != nil {
				if *g.MaxOccurs < 0 || *g.MaxOccurs > math.MaxInt32 {
					return fmt.Errorf("%s: %s: max_occurs out of range", item.Path, a)
				}
				n := int32(*g.MaxOccurs)
				max = &n
			}
			var name []byte
			if len(g.DisplayName) > 0 {
				if name, err = json.Marshal(g.DisplayName); err != nil {
					return fmt.Errorf("%s: %s: %w", item.Path, a, err)
				}
			}
			if err := q.WeaveInsertGraftOverride(ctx, sqlcgen.WeaveInsertGraftOverrideParams{
				ProjectID: projectID, ModelID: item.OwnerID, Address: a, IsHidden: g.IsHidden, MaxOccurs: max, DisplayName: name,
			}); err != nil {
				return fmt.Errorf("%s: %s: %w", item.Path, a, err)
			}
		}
	}
	return nil
}

// hydrateGeneratorFilesTx stores generators/** as opaque files.
func hydrateGeneratorFilesTx(ctx context.Context, q *sqlcgen.Queries, projectID string, snap *ProjectSnapshot) error {
	for _, item := range snap.Entities.GeneratorFiles {
		sum := sha256.Sum256(item.Payload)
		if err := q.WeaveInsertGeneratorFile(ctx, sqlcgen.WeaveInsertGeneratorFileParams{
			ProjectID: projectID, Path: item.Path, Content: item.Payload, Sha256: hex.EncodeToString(sum[:]),
		}); err != nil {
			return fmt.Errorf("%s: %w", item.Path, err)
		}
	}
	return nil
}
