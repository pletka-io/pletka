package projectread

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Limits bounds batches, retained rows, encoded payload bytes and source projects.
// MaxBytes measures row JSON plus ontology artifacts, not peak process memory.
type Limits struct {
	PageSize          int32
	MaxRows, MaxBytes int64
	MaxProjects       int
}

func (l Limits) validate() error {
	if err := (readsession.Page{Limit: l.PageSize}).Validate(); err != nil {
		return err
	}
	if l.MaxRows < 1 || l.MaxBytes < 1 || l.MaxProjects < 1 {
		return fmt.Errorf("assembly budgets must be positive")
	}
	return nil
}

// BudgetError reports a complete-read refusal, never a successful truncated value.
type BudgetError struct{ Resource string }

func (e *BudgetError) Error() string { return "project assembly exceeds " + e.Resource + " budget" }

// ValidationFields gives callers an actionable failure through the canonical door.
func (e *BudgetError) ValidationFields() map[string][]string {
	return map[string][]string{projectKey: {e.Error()}}
}

type budget struct {
	limits      Limits
	rows, bytes int64
	selection   projectsnapshot.Selection
}

func (b *budget) addBytes(n int64) error {
	if n > b.limits.MaxBytes-b.bytes {
		return &BudgetError{Resource: "bytes"}
	}
	b.bytes += n
	return nil
}
func (b *budget) evidence(e projectsnapshot.Evidence) error {
	if e.Contract() == 2 {
		a, err := e.Legacy()
		if err != nil {
			return err
		}
		return b.addBytes(int64(len(a.JSON)))
	}
	artifacts, err := e.Publications()
	if err != nil {
		return err
	}
	for _, a := range artifacts {
		if err := b.addBytes(int64(len(a.Artifact.JSON))); err != nil {
			return err
		}
	}
	refs, err := e.References()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	return b.addBytes(int64(len(raw)))
}
func pages[T any](ctx context.Context, b *budget, read func(context.Context, readsession.Page) ([]T, error)) ([]T, error) {
	out := []T{}
	for offset := int64(0); ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, err := read(ctx, readsession.Page{Limit: b.limits.PageSize, Offset: offset, Selection: b.selection})
		if err != nil {
			return nil, err
		}
		if rows == nil || len(rows) > int(b.limits.PageSize) {
			return nil, fmt.Errorf("content reader returned an unloaded or invalid page")
		}
		if int64(len(rows)) > b.limits.MaxRows-b.rows {
			return nil, &BudgetError{Resource: "rows"}
		}
		data, err := json.Marshal(rows)
		if err != nil {
			return nil, err
		}
		if err := b.addBytes(int64(len(data))); err != nil {
			return nil, err
		}
		b.rows += int64(len(rows))
		out = append(out, rows...)
		if len(rows) < int(b.limits.PageSize) {
			return out, nil
		}
		offset += int64(len(rows))
	}
}
