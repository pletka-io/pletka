package gitmaterializer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
)

// Entity types of models/<M>/grafts.yaml and of the opaque files under
// generators/.
const (
	entityTypeModelGrafts   = "model_grafts"
	entityTypeGeneratorFile = "generator_file"
)

// graftDoc is one entry of models/<M>/grafts.yaml. MaxOccursSet records
// that the key is present, so "max_occurs: null" (unbounded) differs from no
// override.
type graftDoc struct {
	IsHidden     bool                `json:"is_hidden,omitempty"`
	MaxOccurs    *int                `json:"max_occurs,omitempty"`
	MaxOccursSet bool                `json:"-"`
	DisplayName  domain.Translations `json:"display_name,omitempty"`
}

func (g *graftDoc) UnmarshalJSON(b []byte) error {
	type plain graftDoc
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	if err := json.Unmarshal(b, (*plain)(g)); err != nil {
		return err
	}
	_, g.MaxOccursSet = keys["max_occurs"]
	return nil
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
				ProjectID: projectID, ModelID: item.OwnerID, Address: a, IsHidden: g.IsHidden, MaxOccurs: max, MaxOccursSet: g.MaxOccursSet, DisplayName: name,
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

// graftsYAML renders one model's rows as grafts.yaml.
func graftsYAML(rows []sqlcgen.WeaveGraftOverride) ([]byte, error) {
	out := make([]domain.GraftOverride, 0, len(rows))
	for _, r := range rows {
		g := domain.GraftOverride{Address: r.Address, IsHidden: r.IsHidden, MaxOccursSet: r.MaxOccursSet}
		if r.MaxOccurs != nil {
			n := int(*r.MaxOccurs)
			g.MaxOccurs = &n
		}
		if len(r.DisplayName) > 0 {
			if err := json.Unmarshal(r.DisplayName, &g.DisplayName); err != nil {
				return nil, fmt.Errorf("graft %s/%s display_name: %w", r.ModelID, r.Address, err)
			}
		}
		out = append(out, g)
	}
	return canonical.Grafts(out)
}

// writeProjectGrafts writes models/<M>/grafts.yaml for every model with graft
// overrides.
func (m *Materializer) writeProjectGrafts(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListGraftOverridesByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list graft overrides for project %s: %w", projectID, err)
	}
	byModel := map[string][]sqlcgen.WeaveGraftOverride{}
	for _, r := range rows {
		byModel[r.ModelID] = append(byModel[r.ModelID], r)
	}
	for model, rs := range byModel {
		payload, err := graftsYAML(rs)
		if err != nil {
			return err
		}
		if err := writeEntityFile(workDir, path.Join("models", model, "grafts.yaml"), payload); err != nil {
			return err
		}
	}
	return nil
}

// writeGeneratorFiles writes generators/** back byte for byte.
func (m *Materializer) writeGeneratorFiles(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListGeneratorFilesByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list generator files for project %s: %w", projectID, err)
	}
	for _, r := range rows {
		// The path is a DB value: refuse anything outside generators/.
		if clean := path.Clean(r.Path); clean != r.Path || !strings.HasPrefix(clean, "generators/") {
			return fmt.Errorf("generator file %q: path outside generators/", r.Path)
		}
		if err := writeEntityFile(workDir, r.Path, r.Content); err != nil {
			return err
		}
	}
	return nil
}
