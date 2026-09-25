package example

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/domain"
)

type postgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{pool: pool}
}

func (s *postgresStore) CreateWithValues(ctx context.Context, ex *domain.Example, values []domain.ExampleValue) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO weave_examples (id, project_id, entity_type, entity_id, title, description, status, version_number)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, ex.ID, ex.ProjectID, string(ex.EntityType), ex.EntityID, toJSON(ex.Title), toJSON(ex.Description), string(ex.Status), ex.VersionNumber); err != nil {
		return err
	}
	if err := insertValues(ctx, tx, ex.ID, values); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *postgresStore) GetByID(ctx context.Context, id string) (*domain.Example, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, entity_type, entity_id, title, description, status, version_number, created_at, updated_at
		FROM weave_examples
		WHERE id = $1
	`, id)
	return scanExample(row)
}

func (s *postgresStore) UpdateWithValues(ctx context.Context, ex *domain.Example, values []domain.ExampleValue) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ct, err := tx.Exec(ctx, `
		UPDATE weave_examples
		SET title = $2, description = $3, status = $4, updated_at = now()
		WHERE id = $1
	`, ex.ID, toJSON(ex.Title), toJSON(ex.Description), string(ex.Status))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("example not found")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM weave_example_values WHERE example_id = $1`, ex.ID); err != nil {
		return err
	}
	if err := insertValues(ctx, tx, ex.ID, values); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *postgresStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM weave_examples WHERE id = $1`, id)
	return err
}

func (s *postgresStore) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Example, int64, error) {
	cfg := domain.ApplyOptions(opts)
	args := []any{}
	where := []string{"1=1"}
	if cfg.ProjectID != "" {
		args = append(args, cfg.ProjectID)
		where = append(where, fmt.Sprintf("project_id = $%d", len(args)))
	}
	if v, ok := cfg.Filters["entity_type"].(string); ok && strings.TrimSpace(v) != "" {
		args = append(args, v)
		where = append(where, fmt.Sprintf("entity_type = $%d", len(args)))
	}
	if v, ok := cfg.Filters["entity_id"].(string); ok && strings.TrimSpace(v) != "" {
		args = append(args, v)
		where = append(where, fmt.Sprintf("entity_id = $%d", len(args)))
	}
	if v, ok := cfg.Filters["status"].(string); ok && strings.TrimSpace(v) != "" {
		args = append(args, v)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if strings.TrimSpace(cfg.Search) != "" {
		args = append(args, "%"+cfg.Search+"%")
		where = append(where, fmt.Sprintf("(coalesce(title::text,'') ILIKE $%d OR coalesce(description::text,'') ILIKE $%d)", len(args), len(args)))
	}
	countSQL := "SELECT count(*) FROM weave_examples WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	orderBy := "updated_at DESC"
	switch cfg.OrderBy {
	case "created_at":
		orderBy = "created_at DESC"
	case "status":
		orderBy = "status ASC, updated_at DESC"
	}
	args = append(args, cfg.Limit, cfg.Offset)
	sql := `
		SELECT id, project_id, entity_type, entity_id, title, description, status, version_number, created_at, updated_at
		FROM weave_examples
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY ` + orderBy + `
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*domain.Example, 0)
	for rows.Next() {
		ex, err := scanExample(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, ex)
	}
	return out, total, rows.Err()
}

func (s *postgresStore) ListValues(ctx context.Context, exampleID string) ([]domain.ExampleValue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, example_id, override_id, field_id, coalesce(part_of_collection_id, ''), occurrence_index, slot_path,
		       value_kind, value_payload, text_value, number_value, date_value, uri_value,
		       concept_uri, linked_example_id, created_at, updated_at
		FROM weave_example_values
		WHERE example_id = $1
		ORDER BY override_id ASC, occurrence_index ASC, slot_path ASC
	`, exampleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.ExampleValue, 0)
	for rows.Next() {
		v, err := scanExampleValue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *postgresStore) ConceptURIAllowedForLists(ctx context.Context, uri string, conceptListIDs []string) (bool, error) {
	if strings.TrimSpace(uri) == "" || len(conceptListIDs) == 0 {
		return true, nil
	}
	var allowed bool
	// A field bound to a control list accepts only that list's own entries —
	// open or sealed. open/sealed governs whether the list can still grow, not
	// which values a field accepts; the value must always be a member (#3599).
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM weave_concept_lists cl
			JOIN weave_vocabulary_entries ve
			  ON ve.uri = $1
			JOIN weave_concept_list_entries cle
			  ON cle.concept_list_id = cl.id
			 AND cle.vocabulary_entry_id = ve.id
			WHERE (cl.id = ANY($2::text[]) OR cl.semantic_id = ANY($2::text[]))
		)
	`, uri, conceptListIDs).Scan(&allowed)
	return allowed, err
}

func insertValues(ctx context.Context, tx pgx.Tx, exampleID string, values []domain.ExampleValue) error {
	for _, v := range values {
		payload, err := json.Marshal(v.ValuePayload)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO weave_example_values (
				example_id, override_id, field_id, part_of_collection_id, occurrence_index, slot_path,
				value_kind, value_payload, text_value, number_value, date_value, uri_value,
				concept_uri, linked_example_id, version_number
			) VALUES ($1,$2,$3,nullif($4,''),$5,$6,$7,$8,$9,$10,nullif($11,'')::date,$12,$13,$14,$15)
		`, exampleID, v.OverrideID, v.FieldID, v.PartOfCollectionID, v.OccurrenceIndex, v.SlotPath,
			string(v.ValueKind), payload, v.TextValue, v.NumberValue, valueString(v.DateValue), v.URIValue,
			v.ConceptURI, v.LinkedExampleID, "",
		); err != nil {
			return err
		}
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanExample(row rowScanner) (*domain.Example, error) {
	var (
		ex          domain.Example
		entityType  string
		status      string
		title       []byte
		description []byte
	)
	if err := row.Scan(&ex.ID, &ex.ProjectID, &entityType, &ex.EntityID, &title, &description, &status, &ex.VersionNumber, &ex.CreatedAt, &ex.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ex.EntityType = domain.ExampleEntityType(entityType)
	ex.Status = domain.ExampleStatus(status)
	_ = json.Unmarshal(title, &ex.Title)
	_ = json.Unmarshal(description, &ex.Description)
	return &ex, nil
}

func scanExampleValue(row rowScanner) (domain.ExampleValue, error) {
	var (
		v          domain.ExampleValue
		kind       string
		payload    []byte
		number     *float64
		dateVal    *time.Time
		linkedID   *string
		conceptURI *string
		textValue  *string
		uriValue   *string
	)
	if err := row.Scan(&v.ID, &v.ExampleID, &v.OverrideID, &v.FieldID, &v.PartOfCollectionID, &v.OccurrenceIndex, &v.SlotPath,
		&kind, &payload, &textValue, &number, &dateVal, &uriValue, &conceptURI, &linkedID, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return v, err
	}
	v.ValueKind = domain.ExampleValueKind(kind)
	_ = json.Unmarshal(payload, &v.ValuePayload)
	v.TextValue = textValue
	v.NumberValue = number
	if dateVal != nil {
		s := dateVal.Format("2006-01-02")
		v.DateValue = &s
	}
	v.URIValue = uriValue
	v.ConceptURI = conceptURI
	v.LinkedExampleID = linkedID
	return v, nil
}

func toJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}

func valueString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
