package generators

import (
	"context"
	"fmt"
	"io"
)

// FormatSpec describes a registered generator format.
type FormatSpec struct {
	Format        Format
	ContentType   string
	FileExtension string
	RequiresTree  bool
}

// Renderer writes a resolved Snapshot in one output format.
type Renderer interface {
	Spec() FormatSpec
	Render(ctx context.Context, snapshot *Snapshot, w io.Writer) error
}

// Validator is implemented by renderers that can report snapshot issues before rendering.
type Validator interface {
	Validate(ctx context.Context, snapshot *Snapshot) Report
}

// Registry keeps renderer lookup separate from snapshot construction.
type Registry struct {
	renderers map[Format]Renderer
}

func NewRegistry(renderers ...Renderer) (*Registry, error) {
	r := &Registry{renderers: make(map[Format]Renderer, len(renderers))}
	for _, renderer := range renderers {
		if renderer == nil {
			continue
		}
		format := renderer.Spec().Format
		if format == "" {
			return nil, fmt.Errorf("register renderer: empty format")
		}
		if _, exists := r.renderers[format]; exists {
			return nil, fmt.Errorf("register renderer %q: duplicate format", format)
		}
		r.renderers[format] = renderer
	}
	return r, nil
}

func (r *Registry) Renderer(format Format) (Renderer, bool) {
	if r == nil {
		return nil, false
	}
	renderer, ok := r.renderers[format]
	return renderer, ok
}
