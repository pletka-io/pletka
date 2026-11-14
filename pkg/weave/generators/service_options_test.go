package generators

import (
	"context"
	"io"
	"testing"
)

type referencedSpecRenderer struct{ include bool }

func (r referencedSpecRenderer) Spec() FormatSpec {
	return FormatSpec{Format: "referenced-test", IncludeReferenced: r.include}
}

func (referencedSpecRenderer) Render(context.Context, *Snapshot, io.Writer) error { return nil }

func TestOptionsForAsksForWhatTheRendererNeeds(t *testing.T) {
	reg, err := NewRegistry(referencedSpecRenderer{include: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{registry: reg}
	if got := s.OptionsFor("referenced-test", Options{}); !got.IncludeReferenced {
		t.Fatal("IncludeReferenced = false, want true for a renderer that asks for it")
	}
	if got := s.OptionsFor("unknown", Options{}); got.IncludeReferenced {
		t.Fatal("IncludeReferenced = true for an unknown format")
	}
	if got := s.OptionsFor("unknown", Options{IncludeReferenced: true}); !got.IncludeReferenced {
		t.Fatal("OptionsFor cleared a caller's IncludeReferenced")
	}
}
