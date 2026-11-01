package projectsnapshot_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func TestFrozenRowsRemainIndependentAcrossConcurrentReaders(t *testing.T) {
	_, input, source := patchFixture(t)
	id := input.Evidence.Identity()
	sourceID, _ := source.Root()
	sourceInput, err := source.Input(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := projectsnapshot.NewComplete(input, []projectsnapshot.ProjectInput{sourceInput})
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := complete.Entity("model", "m")
	if err != nil {
		t.Fatal(err)
	}
	want, err := complete.Content(id)
	if err != nil {
		t.Fatal(err)
	}
	partial, err := fragment.Content(id)
	if err != nil {
		t.Fatal(err)
	}
	view, err := fragment.Compose()
	if err != nil {
		t.Fatal(err)
	}
	files := bundleBytes(t, complete)
	// Corrupt every reachable input cell, not just the slice headers.
	corruptRowCells(reflect.ValueOf(&input.Content).Elem())
	corruptRowCells(reflect.ValueOf(&sourceInput.Content).Elem())
	t.Run("readers", func(t *testing.T) {
		for worker := range 8 {
			t.Run(fmt.Sprint(worker), func(t *testing.T) {
				t.Parallel()
				for range 3 {
					got, err := complete.Content(id)
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(want, got); diff != "" {
						t.Fatal(diff)
					}
					corruptRowCells(reflect.ValueOf(&got).Elem())
					selected, err := fragment.Content(id)
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(partial, selected); diff != "" {
						t.Fatal(diff)
					}
					corruptRowCells(reflect.ValueOf(&selected).Elem())
					composed, err := fragment.Compose()
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(view, composed, cmp.Comparer(func(a, b projectsnapshot.Identity) bool { return a == b })); diff != "" {
						t.Fatal(diff)
					}
					corruptRowCells(reflect.ValueOf(&composed).Elem())
				}
			})
		}
	})
	if diff := cmp.Diff(files, bundleBytes(t, complete)); diff != "" {
		t.Fatal(diff)
	}
}

func corruptRowCells(value reflect.Value) {
	switch value.Kind() {
	case reflect.Struct:
		for i := range value.NumField() {
			if value.Field(i).CanSet() {
				corruptRowCells(value.Field(i))
			}
		}
	case reflect.Pointer:
		if !value.IsNil() {
			corruptRowCells(value.Elem())
		}
	case reflect.Slice:
		for i := range value.Len() {
			corruptRowCells(value.Index(i))
		}
	case reflect.String:
		value.SetString("changed")
	case reflect.Uint8:
		value.SetUint('!')
	case reflect.Int32, reflect.Int64:
		value.SetInt(-1)
	case reflect.Bool:
		value.SetBool(!value.Bool())
	}
}

func TestDraftPatchComparesExactAuthoredJSONBytes(t *testing.T) {
	base, input, _ := patchFixture(t)
	before := input.Content.Fields[0]
	// Whitespace is not a semantic edit, but it is a different recorded row.
	before.UIName = append([]byte(" \n"), before.UIName...)
	_, err := projectsnapshot.PatchDraft(base, projectsnapshot.DraftPatch{
		Target: input.Evidence.Identity(),
		Before: projectsnapshot.Content{Fields: []projectsnapshot.FieldRecord{before}},
		After:  projectsnapshot.Content{Fields: input.Content.Fields[:1]},
	})
	if err == nil {
		t.Fatal("patch accepted semantically equal but byte-different before row")
	}
}
