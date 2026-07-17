package templates

import (
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/frontendrefs"
)

func TestRendererRenderIslandPage(t *testing.T) {
	r, err := NewRenderer(func(names ...string) template.HTML {
		return template.HTML(strings.Join(names, ","))
	}, nil, AnalyticsConfig{})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	rr := httptest.NewRecorder()
	page := IslandPage{
		Title:   "Projects",
		Lang:    "en",
		Heading: "Projects",
		Island: IslandMount{
			Name:         frontendrefs.Island("entity-list"),
			Dependencies: []string{frontendrefs.Island("entity-list")},
			Props: map[string]string{
				"schema-url": "/projects/entity-list-schema",
				"lang":       "en",
			},
		},
	}

	if err := r.RenderIslandPage(rr, page); err != nil {
		t.Fatalf("RenderIslandPage() error = %v", err)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `data-island="entity-list"`) {
		t.Fatalf("response missing island mount: %s", body)
	}
	if !strings.Contains(body, `data-prop-schema-url="/projects/entity-list-schema"`) {
		t.Fatalf("response missing schema prop: %s", body)
	}
}
