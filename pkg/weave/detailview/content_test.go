package detailview

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

type unreadableDetailSource struct{ calls int }

func (s *unreadableDetailSource) Detail(context.Context, auth.ReadScope, string, string, string) (browserinput.Detail, error) {
	s.calls++
	return browserinput.Detail{}, nil
}

func TestDetailBoundaryAuthorizesCurrentMatchingProjectBeforeLoading(t *testing.T) {
	for _, project := range []domain.Project{
		{Entity: domain.Entity{ID: "P"}, Visibility: "private"},
		{Entity: domain.Entity{ID: "OTHER"}, Visibility: "public"},
	} {
		source := &unreadableDetailSource{}
		h := &Handler{source: source}
		ctx := auth.WithProject(t.Context(), &project)
		_, err := h.loadDetail(ctx, auth.Release("1.0.0"), "P", "model", "M")
		if err == nil || apierror.FromError(err).Code != apierror.CodeNotFound || source.calls != 0 {
			t.Fatalf("unmatched/denied root reached source: calls=%d error=%v", source.calls, err)
		}
	}
}
