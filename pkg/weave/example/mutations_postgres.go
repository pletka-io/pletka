package example

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserhttp"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/txscope"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// NewPostgresMutations binds writes and the same source used by read forms to a
// single repeatable-read transaction. The source must use this pool; readsession
// rejects another pool when it borrows the transaction. Capture enrollment stays
// explicit: an enrolled project still requires an enclosing capture operation.
func NewPostgresMutations(pool *pgxpool.Pool, source browserhttp.DetailSource) (*Mutations, error) {
	if pool == nil || source == nil {
		return nil, fmt.Errorf("example mutation boundary requires pool and source")
	}
	return &Mutations{run: func(ctx context.Context, project string, fn func(mutation) error) error {

		if strings.TrimSpace(project) == "" || fn == nil {
			return fmt.Errorf("example mutation requires project and callback")
		}
		err := txscope.Run(ctx, pool, func(bound context.Context) error {
			m := &postgresMutation{ctx: bound, project: project, source: source, reads: readsession.NewFactory(pool), writes: NewPostgresStore(pool), open: true, links: map[string]*domain.Example{}, targets: map[string]bool{}}
			defer func() { m.open = false }()
			return fn(m)
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01") {
			return mutationConflict{cause: err}
		}
		return err
	}}, nil
}

type postgresMutation struct {
	ctx     context.Context
	project string
	source  browserhttp.DetailSource
	reads   *readsession.Factory
	writes  mutationWriter
	open    bool
	links   map[string]*domain.Example
	targets map[string]bool
}

func (m *postgresMutation) check() error {
	if !m.open {
		return fmt.Errorf("example mutation is closed")
	}
	return m.ctx.Err()
}
func (m *postgresMutation) detail(kind, id string) (browserinput.Detail, error) {
	if err := m.check(); err != nil {
		return browserinput.Detail{}, err
	}
	input, err := m.source.Detail(m.ctx, auth.Draft(), m.project, kind, id)
	if err != nil {
		return browserinput.Detail{}, err
	}
	consumer, err := input.Fragment.Consumer()
	root, rootErr := input.Fragment.Root()
	if err != nil || rootErr != nil || consumer.ProjectID() != m.project || consumer.Version() != "" || root.Kind != kind || root.ID != id {
		return browserinput.Detail{}, fmt.Errorf("mutation source returned a different selection")
	}
	return input, nil
}
func (m *postgresMutation) prepare(kind, id string, maxDepth int) (*slotResolver, valueValidator, error) {
	input, err := m.detail(kind, id)
	if err != nil {
		return nil, valueValidator{}, err
	}
	p, err := newExampleProjection(input.Fragment)
	if err != nil {
		return nil, valueValidator{}, err
	}
	resolver, err := p.resolver(m.ctx, kind, id, maxDepth)
	if err != nil {
		return nil, valueValidator{}, err
	}
	validator := valueValidator{conceptAllowed: p.conceptAllowed, example: func(_ context.Context, id string) (*domain.Example, error) { return m.linkedExample(id) }}
	return resolver, validator, nil
}

// Existing parent metadata is read without following its old value graph. This
// lets a save/delete repair stale values while still enforcing draft ownership.
func (m *postgresMutation) example(id string) (*domain.Example, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	var ex *domain.Example
	err := m.reads.Read(m.ctx, auth.Draft(), m.project, func(session *readsession.Session) error {
		reader, err := BindContentReader(session)
		if err != nil {
			return err
		}
		rows, err := reader.ContentExamples(m.ctx, readsession.Page{Limit: 1, Selection: projectsnapshot.Selection{exampleKind: []string{id}}})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ex, err = projectExampleRecord(rows[0])
		return err
	})
	return ex, err
}
func (m *postgresMutation) linkedExample(id string) (*domain.Example, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	if ex, ok := m.links[id]; ok {
		return ex, nil
	}
	// Owned candidates need no old value closure; foreign candidates must resolve
	// through the consumer's selected source membership, never a global hot lookup.
	own, err := m.example(id)
	if err != nil {
		return nil, err
	}
	if own != nil {
		m.links[id] = own
		return own, nil
	}
	input, err := m.detail(exampleKind, id)
	if err != nil {
		return nil, err
	}
	root, err := input.Fragment.Root()
	if err != nil {
		return nil, err
	}
	content, err := input.Fragment.Content(root.Source)
	if err != nil {
		return nil, err
	}
	for _, row := range content.Examples {
		if row.ID == id {
			ex, err := projectExampleRecord(row)
			if err == nil {
				m.links[id] = ex
			}
			return ex, err
		}
	}
	return nil, fmt.Errorf("linked example is outside retained input")
}
func (m *postgresMutation) stubTarget(id string) error {
	if err := m.check(); err != nil {
		return err
	}
	if m.targets[id] {
		return nil
	}
	_, err := m.detail(modelKind, id)
	if err == nil {
		m.targets[id] = true
	}
	return err
}

// Link integrity is independent of slot validation: stale/moved slots may be
// saved as evidence, but every active reference must still have a selected source.
func (m *postgresMutation) checkLinks(values []domain.ExampleValue) error {
	for _, value := range values {
		if value.LinkedExampleID != nil {
			if _, err := m.linkedExample(*value.LinkedExampleID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *postgresMutation) create(ex *domain.Example, values []domain.ExampleValue) error {
	if err := m.check(); err != nil {
		return err
	}
	if ex == nil || ex.ProjectID != m.project {
		return fmt.Errorf("example write cannot leave its project")
	}
	if err := m.checkLinks(values); err != nil {
		return err
	}
	if err := m.writes.CreateWithValues(m.ctx, ex, values); err != nil {
		return err
	}
	m.links[ex.ID] = ex
	return nil
}
func (m *postgresMutation) update(ex *domain.Example, values []domain.ExampleValue) error {
	if err := m.check(); err != nil {
		return err
	}
	if ex == nil || ex.ProjectID != m.project {
		return fmt.Errorf("example write cannot leave its project")
	}
	own, err := m.example(ex.ID)
	if err != nil {
		return err
	}
	if own == nil {
		return exampleNotFound{}
	}
	if err := m.checkLinks(values); err != nil {
		return err
	}
	if err := m.writes.UpdateWithValues(m.ctx, ex, values); err != nil {
		return err
	}
	m.links[ex.ID] = ex
	return nil
}
func (m *postgresMutation) delete(id string) error {
	own, err := m.example(id)
	if err != nil {
		return err
	}
	if own == nil {
		return exampleNotFound{}
	}
	return m.writes.Delete(m.ctx, id)
}
