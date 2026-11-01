package generators

import (
	"context"
	"fmt"
	"io"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Source owns all loading and closes its read session before returning detached
// values. Renderers never receive this capability.
type Source interface {
	Project(context.Context, auth.ReadScope, string) (projectsnapshot.CompleteProject, error)
	Entity(context.Context, auth.ReadScope, string, string, string) (projectsnapshot.EntityFragment, error)
}

// Service projects retained fragments and dispatches pure renderers.
type Service struct {
	source   Source
	registry *Registry
}

// NewService requires one assembly boundary, rather than per-slice readers.
func NewService(source Source, registry *Registry) *Service {
	return &Service{source: source, registry: registry}
}

func (s *Service) snapshot(ctx context.Context, scope auth.ReadScope, project, kind, id string, opts Options) (*Snapshot, error) {
	if s == nil || s.source == nil || !scope.Valid() {
		return nil, fmt.Errorf("generator requires a source and explicit read scope")
	}
	value, err := s.source.Entity(ctx, scope, project, kind, id)
	if err != nil {
		return nil, err
	}
	consumer, err := value.Consumer()
	if err != nil {
		return nil, err
	}
	root, err := value.Root()
	if err != nil {
		return nil, err
	}
	if consumer.ProjectID() != project || consumer.Version() != scope.Version() || consumer.IsDraft() != !scope.IsRelease() || root.Kind != kind || root.ID != id {
		return nil, fmt.Errorf("generator source returned a different selection")
	}
	return BuildSnapshot(value, opts)
}

// SnapshotForModel assembles one explicit selection before projection.
func (s *Service) SnapshotForModel(ctx context.Context, scope auth.ReadScope, projectID, id string, opts Options) (*Snapshot, error) {
	return s.snapshot(ctx, scope, projectID, string(EntityModel), id, opts)
}

// GenerateModel finishes assembly before invoking the renderer.
func (s *Service) GenerateModel(ctx context.Context, scope auth.ReadScope, projectID, id string, format Format, w io.Writer, opts Options) error {
	snap, err := s.SnapshotForModel(ctx, scope, projectID, id, opts)
	if err != nil {
		return err
	}
	return s.render(ctx, format, snap, w)
}

// SnapshotForCollection assembles one explicit selection before projection.
func (s *Service) SnapshotForCollection(ctx context.Context, scope auth.ReadScope, projectID, id string, opts Options) (*Snapshot, error) {
	return s.snapshot(ctx, scope, projectID, string(EntityCollection), id, opts)
}

// GenerateCollection finishes assembly before invoking the renderer.
func (s *Service) GenerateCollection(ctx context.Context, scope auth.ReadScope, projectID, id string, format Format, w io.Writer, opts Options) error {
	snap, err := s.SnapshotForCollection(ctx, scope, projectID, id, opts)
	if err != nil {
		return err
	}
	return s.render(ctx, format, snap, w)
}

// SnapshotForField assembles one explicit selection before projection.
func (s *Service) SnapshotForField(ctx context.Context, scope auth.ReadScope, projectID, id string, opts Options) (*Snapshot, error) {
	return s.snapshot(ctx, scope, projectID, string(EntityField), id, opts)
}

// GenerateField finishes assembly before invoking the renderer.
func (s *Service) GenerateField(ctx context.Context, scope auth.ReadScope, projectID, id string, format Format, w io.Writer, opts Options) error {
	snap, err := s.SnapshotForField(ctx, scope, projectID, id, opts)
	if err != nil {
		return err
	}
	return s.render(ctx, format, snap, w)
}
func (s *Service) RenderSnapshot(ctx context.Context, format Format, snapshot *Snapshot, w io.Writer) error {
	return s.render(ctx, format, snapshot, w)
}

func (s *Service) render(ctx context.Context, format Format, snapshot *Snapshot, w io.Writer) error {
	if s == nil || s.registry == nil {
		return fmt.Errorf("generator service: registry is nil")
	}
	renderer, ok := s.registry.Renderer(format)
	if !ok {
		return fmt.Errorf("generator renderer %q: not registered", format)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	copy, err := cloneSnapshot(snapshot)
	if err != nil {
		return err
	}
	if len(copy.Report.Errors) > 0 {
		return fmt.Errorf("generator input contains errors: %v", copy.Report.Errors)
	}
	return renderer.Render(ctx, copy, w)
}

// computeSharedPathPrefix mirrors weave/resolve.ComputeSharedPathPrefix
// without taking a dependency back on the weave package (cycle).
func computeSharedPathPrefix(fields []domain.ResolvedField) []domain.PathElement {
	if len(fields) == 0 {
		return nil
	}
	first := fields[0].PathElements
	if len(first) == 0 {
		return nil
	}
	prefixLen := len(first)
	for _, f := range fields[1:] {
		if len(f.PathElements) < prefixLen {
			prefixLen = len(f.PathElements)
		}
		for i := 0; i < prefixLen; i++ {
			if f.PathElements[i].URI != first[i].URI {
				prefixLen = i
				break
			}
		}
		if prefixLen == 0 {
			return nil
		}
	}
	if prefixLen == 0 {
		return nil
	}
	return append([]domain.PathElement(nil), first[:prefixLen]...)
}
