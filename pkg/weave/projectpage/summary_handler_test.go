package projectpage

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

type countingSummarySource struct {
	calls int
	value browserinput.ProjectSummary
}

func (s *countingSummarySource) Summary(context.Context, auth.ReadScope, string) (browserinput.ProjectSummary, error) {
	s.calls++
	return s.value, nil
}

func TestSummaryAuthorizesCurrentMatchingRootBeforeLoading(t *testing.T) {
	for _, p := range []domain.Project{{Entity: domain.Entity{ID: "P"}, Visibility: "private"}, {Entity: domain.Entity{ID: "OTHER"}, Visibility: "public"}} {
		source := &countingSummarySource{}
		h := &Handler{source: source}
		_, err := h.loadSummary(auth.WithProject(t.Context(), &p), auth.Release("1.0.0"), "P")
		if err == nil || apierror.FromError(err).Code != apierror.CodeNotFound || source.calls != 0 {
			t.Fatalf("denied root reached summary: %v calls=%d", err, source.calls)
		}
	}
}

func TestSummaryRejectsSelectionMismatchAndCannotSupplyAuthorization(t *testing.T) {
	captured := browserinput.ProjectHeader{Record: projectsnapshot.ProjectRecord{ID: "P", VersionNumber: "1.0.0", OwnerID: "HISTORICAL", Visibility: "public", UIName: []byte(`{"en":"Retained"}`)}}
	display, err := captured.Project()
	if err != nil {
		t.Fatal(err)
	}
	if display.OwnerID != "" || display.Visibility != "" || display.UIName.Get("en") != "Retained" {
		t.Fatalf("display row retained authorization policy: %+v", display)
	}
	source := &countingSummarySource{value: browserinput.ProjectSummary{Header: captured}}
	h := &Handler{source: source}
	ctx := auth.WithProject(t.Context(), &domain.Project{Entity: domain.Entity{ID: "P"}, Visibility: "public"})
	if _, err := h.loadSummary(ctx, auth.Release("2.0.0"), "P"); err == nil {
		t.Fatal("accepted a source with a different selection")
	}
}
