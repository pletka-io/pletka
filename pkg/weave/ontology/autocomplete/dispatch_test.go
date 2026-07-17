package autocomplete

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type fakeSuggest struct {
	name string
	err  error
	out  []Suggestion
}

func (f fakeSuggest) GetSuggestions(_ context.Context, _ Request) ([]Suggestion, error) {
	return f.out, f.err
}

func TestDispatch_ModeRouting(t *testing.T) {
	indexedFake := fakeSuggest{name: "indexed", out: []Suggestion{{Qname: "from:indexed"}}}
	directFake := fakeSuggest{name: "direct", out: []Suggestion{{Qname: "from:direct"}}}

	t.Run("ModeDirect routes to direct", func(t *testing.T) {
		d := &DispatchEngine{
			indexed:   indexedFake,
			directSug: directFake,
			mode:      ModeDirect,
			log:       slog.Default(),
		}
		got, err := d.GetSuggestions(context.Background(), Request{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Qname != "from:direct" {
			t.Fatalf("want direct result, got %v", got)
		}
	})

	t.Run("ModeIndexed routes to indexed", func(t *testing.T) {
		d := &DispatchEngine{
			indexed:   indexedFake,
			directSug: directFake,
			mode:      ModeIndexed,
			log:       slog.Default(),
		}
		got, err := d.GetSuggestions(context.Background(), Request{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Qname != "from:indexed" {
			t.Fatalf("want indexed result, got %v", got)
		}
	})
}

func TestDispatch_FallbackOnIndexedError(t *testing.T) {
	d := &DispatchEngine{
		indexed:   fakeSuggest{out: nil, err: errors.New("boom")},
		directSug: fakeSuggest{out: []Suggestion{{Qname: "from:direct"}}},
		mode:      ModeIndexedWithFallback,
		log:       slog.Default(),
	}
	got, err := d.GetSuggestions(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Qname != "from:direct" {
		t.Fatalf("want direct fallback, got %v", got)
	}
}

func TestDispatch_OverrideGating(t *testing.T) {
	mk := func(allow, super bool, src string) Mode {
		d := &DispatchEngine{mode: ModeIndexed, allowOverride: allow, log: slog.Default()}
		return d.modeFor(Request{Source: src, SuperAdmin: super})
	}
	if mk(true, true, "direct") != ModeDirect {
		t.Fatal("super-admin + allowed + ?source=direct should force direct")
	}
	if mk(true, false, "direct") != ModeIndexed {
		t.Fatal("non-super-admin must not override")
	}
	if mk(false, true, "direct") != ModeIndexed {
		t.Fatal("override disabled must ignore ?source")
	}
}
