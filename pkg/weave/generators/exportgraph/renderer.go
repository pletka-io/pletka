package exportgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/pletka-io/pletka/pkg/weave/generators"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{
		Format:        generators.FormatExportGraph,
		ContentType:   "application/json; charset=utf-8",
		FileExtension: ".exportgraph.json",
		RequiresTree:  true,
	}
}

func (r *Renderer) Render(ctx context.Context, snap *generators.Snapshot, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("exportgraph renderer: snapshot is nil")
	}
	if len(snap.Report.Errors) > 0 {
		return fmt.Errorf("exportgraph renderer: snapshot has %d error(s)", len(snap.Report.Errors))
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Build(snap))
}
