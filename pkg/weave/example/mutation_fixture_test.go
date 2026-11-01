package example

import (
	"context"
	"fmt"
	"maps"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

// Existing rule tests use selected value fixtures. Live-view adapters exist only
// here; real transaction/source membership behavior is covered in PostgreSQL.
type testViews interface {
	ModelView(context.Context, string, string) (*domain.ModelView, error)
	CollectionView(context.Context, string, string) ([]domain.ResolvedField, error)
}
type testService struct {
	*Service
	store *fakeStore
	views testViews
}
type fixtureMutation struct {
	ctx     context.Context
	project string
	store   *fakeStore
	views   testViews
}

func newTestService(store *fakeStore, views testViews, opts ...ServiceOption) *testService {
	boundary := &Mutations{run: func(ctx context.Context, project string, fn func(mutation) error) error {
		beforeExamples, beforeValues := maps.Clone(store.examples), maps.Clone(store.values)
		err := fn(&fixtureMutation{ctx: ctx, project: project, store: store, views: views})
		if err != nil {
			store.examples, store.values = beforeExamples, beforeValues
		}
		return err
	}}
	return &testService{Service: NewService(boundary, opts...), store: store, views: views}
}
func fixtureContext(ctx context.Context) context.Context {
	if !auth.ReadScopeFromContext(ctx).Valid() {
		return auth.WithReadScope(ctx, auth.Draft())
	}
	return ctx
}
func (s *testService) Create(ctx context.Context, project string, in CreateInput) (*ExampleRecord, error) {
	return s.Service.Create(ctx, auth.ReadScopeFromContext(fixtureContext(ctx)), project, in)
}
func (s *testService) Update(ctx context.Context, project, id string, in UpdateInput) (*ExampleRecord, error) {
	return s.Service.Update(ctx, auth.ReadScopeFromContext(fixtureContext(ctx)), project, id, in)
}
func (s *testService) Delete(ctx context.Context, project, id string) error {
	return s.Service.Delete(ctx, auth.ReadScopeFromContext(fixtureContext(ctx)), project, id)
}
func (m *fixtureMutation) prepare(_, id string, depth int) (*slotResolver, valueValidator, error) {
	view, err := m.views.ModelView(m.ctx, id, m.project)
	if err != nil {
		return nil, valueValidator{}, err
	}
	if view == nil {
		return nil, valueValidator{}, fmt.Errorf("missing selected model")
	}
	r := fixtureResolver(m.ctx, m.project, m.views, view, depth)
	v := valueValidator{conceptAllowed: func(ctx context.Context, uri string, _ []domain.EntityRef) (bool, error) {
		return m.store.ConceptURIAllowedForLists(ctx, uri, nil)
	}, example: func(_ context.Context, id string) (*domain.Example, error) { return m.linkedExample(id) }}
	return r, v, nil
}
func fixtureResolver(ctx context.Context, project string, views testViews, view *domain.ModelView, depth int) *slotResolver {
	return &slotResolver{ctx: ctx, projectID: project, views: views, maxDepth: depth, view: view, groups: groupOfOverride(view), model: buildFieldByOverride(view), collections: map[string][]domain.ResolvedField{}}
}
func (s *testService) newSlotResolver(ctx context.Context, project string, view *domain.ModelView) *slotResolver {
	return fixtureResolver(ctx, project, s.views, view, s.maxDepth)
}
func (m *fixtureMutation) example(id string) (*domain.Example, error) {
	ex, err := m.store.GetByID(m.ctx, id)
	if ex != nil && ex.ProjectID != m.project {
		return nil, nil
	}
	return ex, err
}
func (m *fixtureMutation) linkedExample(id string) (*domain.Example, error) {
	return m.store.GetByID(m.ctx, id)
}
func (*fixtureMutation) stubTarget(string) error { return nil }
func (m *fixtureMutation) create(ex *domain.Example, values []domain.ExampleValue) error {
	return m.store.CreateWithValues(m.ctx, ex, values)
}
func (m *fixtureMutation) update(ex *domain.Example, values []domain.ExampleValue) error {
	return m.store.UpdateWithValues(m.ctx, ex, values)
}
func (m *fixtureMutation) delete(id string) error {
	ex, err := m.example(id)
	if err != nil {
		return err
	}
	if ex == nil {
		return exampleNotFound{}
	}
	return m.store.Delete(m.ctx, id)
}

// Read assertions in the rule suite exercise the same pure validator/builder.
// The HTTP reads themselves use ProjectRead and are tested against real inputs.
func (s *testService) Get(ctx context.Context, scope auth.ReadScope, project, id string) (*ExampleRecord, error) {
	if err := requireDraftEditing(scope); err != nil {
		return nil, err
	}
	m := fixtureMutation{ctx: ctx, project: project, store: s.store, views: s.views}
	ex, err := m.example(id)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exampleNotFound{}
	}
	values, err := s.store.ListValues(ctx, id)
	if err != nil {
		return nil, err
	}
	resolver, validator, err := m.prepare(string(ex.EntityType), ex.EntityID, s.maxDepth)
	if err != nil {
		return nil, err
	}
	report, err := validator.validateValues(ctx, resolver, values, false)
	report.ExampleID = id
	return &ExampleRecord{Example: ex, Values: values, Validation: report}, err
}
func (s *testService) BuildFormSchema(ctx context.Context, scope auth.ReadScope, project, mode, kind, id, exampleID, lang string, languages []formschema.LanguageInfo) (*ExampleFormSchema, error) {
	if err := requireDraftEditing(scope); err != nil {
		return nil, err
	}
	var record *ExampleRecord
	m := fixtureMutation{ctx: ctx, project: project, store: s.store, views: s.views}
	if mode == formschema.ModeEdit {
		ex, err := m.example(exampleID)
		if err != nil {
			return nil, err
		}
		if ex == nil {
			return nil, exampleNotFound{}
		}
		values, err := s.store.ListValues(ctx, exampleID)
		if err != nil {
			return nil, err
		}
		record = &ExampleRecord{Example: ex, Values: values}
		id = ex.EntityID
	}
	r, v, err := m.prepare(kind, id, s.maxDepth)
	if err != nil {
		return nil, err
	}
	if record != nil {
		record.Validation, err = v.validateValues(ctx, r, record.Values, false)
		if err != nil {
			return nil, err
		}
	}
	schema, err := buildExampleFormSchema(r, exampleID, id, domain.Translations{"en": id}, record, lang, languages)
	if err != nil {
		return nil, err
	}
	setExampleEndpoint(schema, project, exampleID)
	return schema, nil
}

func (s *testService) validateModelValues(ctx context.Context, values []domain.ExampleValue) (domain.ExampleValidationReport, error) {
	m := fixtureMutation{ctx: ctx, project: "P1", store: s.store, views: s.views}
	r, v, err := m.prepare(modelKind, "M1", s.maxDepth)
	if err != nil {
		return domain.ExampleValidationReport{}, err
	}
	return v.validateValues(ctx, r, values, false)
}
