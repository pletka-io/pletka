package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeModels struct {
	models  []*domain.Model
	gotOpts []domain.QueryOption
}

// List emulates the store's SQL-backed status filter (Task 1): a non-empty
// cfg.Filters["status"] narrows both the returned rows and the count.
func (f *fakeModels) List(_ context.Context, _ string, opts ...domain.QueryOption) ([]*domain.Model, int64, error) {
	f.gotOpts = opts
	cfg := domain.ApplyOptions(opts)
	status, _ := cfg.Filters["status"].(string)
	if status == "" {
		return f.models, int64(len(f.models)), nil
	}
	var matched []*domain.Model
	for _, m := range f.models {
		if string(m.Status) == status {
			matched = append(matched, m)
		}
	}
	return matched, int64(len(matched)), nil
}
func (f *fakeModels) Get(_ context.Context, _, id string) (*domain.Model, error) {
	for _, m := range f.models {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, errors.New("not found")
}

// modelWith builds a domain.Model with the fields the entity tools tests
// need. status is converted to domain.Status explicitly — Entity.Status is
// a defined string type, so a plain string variable does not implicitly
// convert (see domain.Status doc comment).
func modelWith(id, semantic, status string) *domain.Model {
	m := &domain.Model{}
	m.ID = id
	m.SemanticID = semantic
	m.Status = domain.Status(status)
	return m
}

func testHostEntities() Host {
	h := testHostProjects() // from tools_projects_test.go — readable project LA
	h.Models = &fakeModels{models: []*domain.Model{
		modelWith("01ULIDAAA", "LAM.1", "published"),
		modelWith("01ULIDBBB", "LAM.2", "draft"),
	}}
	return h
}

func TestListEntitiesUnknownType(t *testing.T) {
	h := testHostEntities()
	if _, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "widget"}); err == nil {
		t.Fatal("unknown entity_type must error")
	}
}

func TestListEntitiesStatusFilter(t *testing.T) {
	h := testHostEntities()
	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "model", Status: "draft"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 1 || out.Entities[0].SemanticID != "LAM.2" {
		t.Fatalf("want only LAM.2, got %+v", out.Entities)
	}
	if out.TotalCount != 1 {
		t.Fatalf("want TotalCount 1 (post-filter match count), got %d", out.TotalCount)
	}
}

func TestListEntitiesBaseClass(t *testing.T) {
	h := testHostEntities()
	scoped := modelWith("01ULIDCCC", "LAM.3", "published")
	scoped.OntologyScope = domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E22_Human-Made_Object"}
	h.Models = &fakeModels{models: []*domain.Model{scoped, modelWith("01ULIDDDD", "LAM.4", "published")}}

	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "model"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 2 {
		t.Fatalf("want 2 entities, got %d", len(out.Entities))
	}
	if out.Entities[0].BaseClass != "crm:E22_Human-Made_Object" {
		t.Fatalf("want BaseClass crm:E22_Human-Made_Object, got %q", out.Entities[0].BaseClass)
	}
	if out.Entities[1].BaseClass != "" {
		t.Fatalf("want empty BaseClass for zero-value OntologyScope, got %q", out.Entities[1].BaseClass)
	}
}

type fakeFields struct {
	fields []*domain.Field
	// refs, keyed by field ID, is returned by BatchUsageRefs for that field;
	// fields with no entry get a zero-value domain.FieldUsageList (no owners).
	refs map[string]domain.FieldUsageList
	// gotBatchUsageProjectID/gotBatchUsageFieldIDs record BatchUsageRefs'
	// most recent call args so tests can assert exactly the page's field IDs
	// were requested.
	gotBatchUsageProjectID string
	gotBatchUsageFieldIDs  []string
	// batchUsageErr, when set, is returned by BatchUsageRefs instead of refs.
	batchUsageErr error
}

// List implements limit enforcement: returns at most cfg.Limit rows, but total
// is the full count of matching fields. This models the real store behavior.
func (f *fakeFields) List(_ context.Context, _ string, opts ...domain.QueryOption) ([]*domain.Field, int64, error) {
	cfg := domain.ApplyOptions(opts)
	total := int64(len(f.fields))

	limit := cfg.Limit
	if limit <= 0 {
		limit = len(f.fields)
	}
	offset := cfg.Offset
	if offset < 0 {
		offset = 0
	}

	end := offset + limit
	if end > len(f.fields) {
		end = len(f.fields)
	}
	if offset > len(f.fields) {
		offset = len(f.fields)
	}

	return f.fields[offset:end], total, nil
}
func (f *fakeFields) GetByIdentifier(_ context.Context, _, id string) (*domain.Field, error) {
	for _, fld := range f.fields {
		if fld.ID == id {
			return fld, nil
		}
	}
	return nil, errors.New("not found")
}

// BatchUsageRefs records the call args and returns f.refs (or f.batchUsageErr
// if set) so tests can inject ownership and assert what was requested.
func (f *fakeFields) BatchUsageRefs(_ context.Context, projectID string, fieldIDs []string) (map[string]domain.FieldUsageList, error) {
	f.gotBatchUsageProjectID = projectID
	f.gotBatchUsageFieldIDs = fieldIDs
	if f.batchUsageErr != nil {
		return nil, f.batchUsageErr
	}
	if f.refs == nil {
		return map[string]domain.FieldUsageList{}, nil
	}
	return f.refs, nil
}

func TestListEntitiesFieldPathElements(t *testing.T) {
	h := testHostEntities()
	fld := &domain.Field{}
	fld.ID = "01ULIDFFF"
	fld.SemanticID = "LAF.1"
	fld.Status = domain.Status("published")
	fld.PathElements = []domain.PathElement{
		{Type: "class", Prefix: "crm", LocalName: "E21_Person"},
		{Type: "property", Prefix: "crm", LocalName: "P1_is_identified_by"},
	}
	h.Fields = &fakeFields{fields: []*domain.Field{fld}}

	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "field"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 1 {
		t.Fatalf("want 1 entity, got %d", len(out.Entities))
	}
	if len(out.Entities[0].PathElements) != 2 {
		t.Fatalf("want 2 path elements, got %d", len(out.Entities[0].PathElements))
	}
	if out.Entities[0].PathElements[0].LocalName != "E21_Person" {
		t.Fatalf("want first path element LocalName E21_Person, got %q", out.Entities[0].PathElements[0].LocalName)
	}
}

// TestListEntitiesFieldOwnership verifies field rows carry owning
// models/collections from BatchUsageRefs: a field with refs gets populated
// semantic-ID slices, a field with none gets nil (the orphan signal). It
// also asserts the fake received exactly the page's field IDs.
func TestListEntitiesFieldOwnership(t *testing.T) {
	h := testHostEntities()
	owned := &domain.Field{}
	owned.ID = "01ULIDOWN"
	owned.SemanticID = "LAF.1"
	owned.Status = domain.Status("published")

	orphan := &domain.Field{}
	orphan.ID = "01ULIDORP"
	orphan.SemanticID = "LAF.2"
	orphan.Status = domain.Status("published")

	fake := &fakeFields{
		fields: []*domain.Field{owned, orphan},
		refs: map[string]domain.FieldUsageList{
			"01ULIDOWN": {
				Models:      []domain.FieldUsageRef{{SemanticID: "LAM.9"}},
				Collections: []domain.FieldUsageRef{{SemanticID: "LAC.22"}},
			},
		},
	}
	h.Fields = fake

	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "field"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 2 {
		t.Fatalf("want 2 entities, got %d", len(out.Entities))
	}

	if diff := cmp.Diff([]string{"LAM.9"}, out.Entities[0].Models); diff != "" {
		t.Fatalf("owned row Models mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"LAC.22"}, out.Entities[0].Collections); diff != "" {
		t.Fatalf("owned row Collections mismatch (-want +got):\n%s", diff)
	}
	if out.Entities[1].Models != nil {
		t.Fatalf("want nil Models for orphan row, got %v", out.Entities[1].Models)
	}
	if out.Entities[1].Collections != nil {
		t.Fatalf("want nil Collections for orphan row, got %v", out.Entities[1].Collections)
	}

	if fake.gotBatchUsageProjectID != "LA" {
		t.Fatalf("want BatchUsageRefs projectID LA, got %q", fake.gotBatchUsageProjectID)
	}
	if diff := cmp.Diff([]string{"01ULIDOWN", "01ULIDORP"}, fake.gotBatchUsageFieldIDs); diff != "" {
		t.Fatalf("BatchUsageRefs field IDs mismatch (-want +got):\n%s", diff)
	}
}

// TestListEntitiesFieldOwnershipError verifies an ownership-fetch error fails
// the whole list_entities call — silent absence would read as "orphan".
func TestListEntitiesFieldOwnershipError(t *testing.T) {
	h := testHostEntities()
	fld := &domain.Field{}
	fld.ID = "01ULIDERR"
	fld.SemanticID = "LAF.1"
	fld.Status = domain.Status("published")
	h.Fields = &fakeFields{fields: []*domain.Field{fld}, batchUsageErr: errors.New("boom")}

	_, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "field"})
	if err == nil {
		t.Fatal("want error when BatchUsageRefs fails")
	}
	if !strings.Contains(err.Error(), "field ownership") {
		t.Fatalf("want wrapped %q error, got %v", "field ownership", err)
	}
}

// TestListEntitiesFacetDoesNotFetchOwnership verifies facet mode returns
// before any ownership fetch — BatchUsageRefs must not be called.
func TestListEntitiesFacetDoesNotFetchOwnership(t *testing.T) {
	h := testHostEntities()
	fake := &fakeFields{fields: []*domain.Field{
		fieldWithPath("01A", "LAF.1", "crm", "P1_is_identified_by"),
	}}
	h.Fields = fake

	_, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "field", Facet: "path_root"})
	if err != nil {
		t.Fatalf("facet: %v", err)
	}
	if fake.gotBatchUsageFieldIDs != nil {
		t.Fatalf("want BatchUsageRefs not called in facet mode, got fieldIDs %v", fake.gotBatchUsageFieldIDs)
	}
}

// fieldWithPath builds a domain.Field whose PathElements[0] is a single
// property step, for facet-bucketing tests.
func fieldWithPath(id, semantic, prefix, local string) *domain.Field {
	f := &domain.Field{}
	f.ID = id
	f.SemanticID = semantic
	f.Status = domain.Status("published")
	f.PathElements = []domain.PathElement{
		{Type: "property", Prefix: prefix, LocalName: local},
	}
	return f
}

func TestListEntitiesFacetPathRoot(t *testing.T) {
	h := testHostEntities()
	h.Fields = &fakeFields{fields: []*domain.Field{
		fieldWithPath("01A", "LAF.1", "crm", "P1_is_identified_by"),
		fieldWithPath("01B", "LAF.2", "crm", "P1_is_identified_by"),
		fieldWithPath("01C", "LAF.3", "aaao", "ZP42_intentionally_initiated"),
	}}
	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "field", Facet: "path_root"})
	if err != nil {
		t.Fatalf("facet: %v", err)
	}
	if out.Facet != "path_root" || len(out.Buckets) != 2 {
		t.Fatalf("buckets: %+v", out)
	}
	if out.Buckets[0].Value != "crm:P1_is_identified_by" || out.Buckets[0].Count != 2 {
		t.Fatalf("top bucket wrong: %+v", out.Buckets[0])
	}
	// Check that IDs are present and not truncated
	if len(out.Buckets[0].IDs) != 2 {
		t.Fatalf("top bucket IDs length: want 2, got %d (%v)", len(out.Buckets[0].IDs), out.Buckets[0].IDs)
	}
	if out.Buckets[0].IDs[0] != "LAF.1" || out.Buckets[0].IDs[1] != "LAF.2" {
		t.Fatalf("top bucket IDs: want [LAF.1, LAF.2], got %v", out.Buckets[0].IDs)
	}
	if out.Buckets[0].Truncated {
		t.Fatal("top bucket should not be truncated")
	}
	if len(out.Entities) != 0 {
		t.Fatal("facet mode must not return entity rows")
	}
	if out.TotalCount != 3 {
		t.Fatalf("total = %d, want 3 (fields aggregated)", out.TotalCount)
	}
	if out.FacetTruncated {
		t.Fatal("FacetTruncated: want false when scan covers all matches")
	}
}

func TestListEntitiesFacetIDCap(t *testing.T) {
	h := testHostEntities()
	// Generate 101 fake fields sharing one path root
	fields := make([]*domain.Field, 101)
	for i := 0; i < 101; i++ {
		ulid := fmt.Sprintf("01ULID%03d", i)
		semantic := fmt.Sprintf("LAF.%d", i+1)
		fields[i] = fieldWithPath(ulid, semantic, "crm", "P1_is_identified_by")
	}
	h.Fields = &fakeFields{fields: fields}

	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "field", Facet: "path_root"})
	if err != nil {
		t.Fatalf("facet: %v", err)
	}
	if len(out.Buckets) != 1 {
		t.Fatalf("want 1 bucket, got %d", len(out.Buckets))
	}

	bucket := out.Buckets[0]
	if bucket.Count != 101 {
		t.Fatalf("Count: want 101 (true total), got %d", bucket.Count)
	}
	if len(bucket.IDs) != 100 {
		t.Fatalf("IDs length: want 100 (capped), got %d", len(bucket.IDs))
	}
	if !bucket.Truncated {
		t.Fatal("Truncated: want true when IDs are capped")
	}
	if out.TotalCount != 101 {
		t.Fatalf("TotalCount: want 101, got %d", out.TotalCount)
	}
	// FacetTruncated is false because all 101 fields fit within fallbackScanLimit (10000)
	if out.FacetTruncated {
		t.Fatal("FacetTruncated: want false when scan covers all matches")
	}
}

func TestListEntitiesFacetRejectsBadInput(t *testing.T) {
	h := testHostEntities()
	if _, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "model", Facet: "path_root"}); err == nil {
		t.Fatal("facet on non-field must error")
	}
	if _, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "field", Facet: "namespace"}); err == nil {
		t.Fatal("unknown facet must error")
	}
}

type fakeCollections struct {
	collections []*domain.Collection
	gotOpts     []domain.QueryOption
}

// List emulates the store's SQL-backed status filter: a non-empty
// cfg.Filters["status"] narrows both the returned rows and the count.
func (f *fakeCollections) List(_ context.Context, _ string, opts ...domain.QueryOption) ([]*domain.Collection, int64, error) {
	f.gotOpts = opts
	cfg := domain.ApplyOptions(opts)
	status, _ := cfg.Filters["status"].(string)
	if status == "" {
		return f.collections, int64(len(f.collections)), nil
	}
	var matched []*domain.Collection
	for _, c := range f.collections {
		if string(c.Status) == status {
			matched = append(matched, c)
		}
	}
	return matched, int64(len(matched)), nil
}
func (f *fakeCollections) Get(_ context.Context, _, id string) (*domain.Collection, error) {
	for _, c := range f.collections {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, errors.New("not found")
}

// collectionWith builds a domain.Collection with the fields the entity tools tests need.
func collectionWith(id, semantic, status string) *domain.Collection {
	c := &domain.Collection{}
	c.ID = id
	c.SemanticID = semantic
	c.Status = domain.Status(status)
	return c
}

type fakeCategories struct {
	categories []*domain.Category
}

// List ignores opts entirely — the category store does not support paging
// or filters (see task brief); the tool compensates in memory.
func (f *fakeCategories) List(_ context.Context, _ string, _ ...domain.QueryOption) ([]*domain.Category, error) {
	return f.categories, nil
}
func (f *fakeCategories) Get(_ context.Context, _, id string) (*domain.Category, error) {
	for _, c := range f.categories {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, errors.New("not found")
}

func categoryWith(id, semantic, status string) *domain.Category {
	c := &domain.Category{}
	c.ID = id
	c.SemanticID = semantic
	c.Status = domain.Status(status)
	return c
}

func TestListEntitiesCategoryFilterBeforePaging(t *testing.T) {
	h := testHostEntities()
	h.Categories = &fakeCategories{categories: []*domain.Category{
		categoryWith("01C1", "LA.CAT.1", "published"),
		categoryWith("01C2", "LA.CAT.2", "draft"),
		categoryWith("01C3", "LA.CAT.3", "published"),
		categoryWith("01C4", "LA.CAT.4", "draft"),
		categoryWith("01C5", "LA.CAT.5", "published"),
	}}

	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "category", Status: "published", Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 2 {
		t.Fatalf("want 2 rows, got %d", len(out.Entities))
	}
	if out.TotalCount != 3 {
		t.Fatalf("want TotalCount 3 (all published), got %d", out.TotalCount)
	}

	out2, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "category", Status: "published", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("list offset: %v", err)
	}
	if len(out2.Entities) != 1 {
		t.Fatalf("want 1 row at offset 2, got %d", len(out2.Entities))
	}
	if out2.TotalCount != 3 {
		t.Fatalf("want TotalCount 3, got %d", out2.TotalCount)
	}
}

func TestListEntitiesCategoryStatusCaseInsensitive(t *testing.T) {
	h := testHostEntities()
	h.Categories = &fakeCategories{categories: []*domain.Category{
		categoryWith("01C1", "LA.CAT.1", "published"),
		categoryWith("01C2", "LA.CAT.2", "draft"),
	}}

	// Test mixed-case status filter: "Published" should match "published" after normalization
	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "category", Status: "Published"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 1 {
		t.Fatalf("want 1 row, got %d", len(out.Entities))
	}
	if out.Entities[0].SemanticID != "LA.CAT.1" {
		t.Fatalf("want LA.CAT.1, got %s", out.Entities[0].SemanticID)
	}
	if out.TotalCount != 1 {
		t.Fatalf("want TotalCount 1, got %d", out.TotalCount)
	}
}

func TestListEntitiesCategoryStatusPaddedAndCased(t *testing.T) {
	h := testHostEntities()
	h.Categories = &fakeCategories{categories: []*domain.Category{
		categoryWith("01C1", "LA.CAT.1", "published"),
		categoryWith("01C2", "LA.CAT.2", "draft"),
	}}

	// Test padded and mixed-case status filter: " published " should match "published" after trim and lowercase
	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "category", Status: " published "})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 1 {
		t.Fatalf("want 1 row, got %d", len(out.Entities))
	}
	if out.Entities[0].SemanticID != "LA.CAT.1" {
		t.Fatalf("want LA.CAT.1, got %s", out.Entities[0].SemanticID)
	}
	if out.TotalCount != 1 {
		t.Fatalf("want TotalCount 1, got %d", out.TotalCount)
	}
}

func TestListEntitiesCollectionStatusFilterAndBaseClass(t *testing.T) {
	h := testHostEntities()
	scoped := collectionWith("01ULIDCCC", "LAC.1", "published")
	scoped.OntologyScope = domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E67_Birth"}
	unscoped := collectionWith("01ULIDDDD", "LAC.2", "published")
	draft := collectionWith("01ULIDEEE", "LAC.3", "draft")

	h.Collections = &fakeCollections{collections: []*domain.Collection{scoped, unscoped, draft}}

	// Test status filter with mixed-case input
	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "collection", Status: "Published"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 2 {
		t.Fatalf("want 2 published entities, got %d", len(out.Entities))
	}
	if out.TotalCount != 2 {
		t.Fatalf("want TotalCount 2, got %d", out.TotalCount)
	}

	// Verify BaseClass is set for scoped collection
	var foundScoped bool
	for _, e := range out.Entities {
		if e.SemanticID == "LAC.1" {
			foundScoped = true
			if e.BaseClass != "crm:E67_Birth" {
				t.Fatalf("want BaseClass crm:E67_Birth, got %q", e.BaseClass)
			}
		}
		if e.SemanticID == "LAC.2" && e.BaseClass != "" {
			t.Fatalf("want empty BaseClass for unscoped collection, got %q", e.BaseClass)
		}
	}
	if !foundScoped {
		t.Fatal("scoped collection LAC.1 not found in results")
	}
}

func TestGetEntitySemanticFallback(t *testing.T) {
	h := testHostEntities()
	out, err := getEntity(context.Background(), h, getEntityInput{ProjectID: "LA", EntityType: "model", ID: "LAM.2"})
	if err != nil {
		t.Fatalf("get by semantic id: %v", err)
	}
	m, ok := out.Entity.(*domain.Model)
	if !ok || m.SemanticID != "LAM.2" {
		t.Fatalf("want LAM.2 model, got %#v", out.Entity)
	}

	// Verify the fallback scan includes the limit option
	fake := h.Models.(*fakeModels)
	if len(fake.gotOpts) == 0 {
		t.Fatal("expected at least one query option (limit)")
	}
	cfg := domain.ApplyOptions(fake.gotOpts)
	if cfg.Limit != 10000 {
		t.Fatalf("expected limit %d, got %d", 10000, cfg.Limit)
	}
}
