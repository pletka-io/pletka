package pages

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// fakeReleaseReader is a test double for auth.LatestReleaseReader.
type fakeReleaseReader struct {
	version string
	err     error
	// failIfCalled makes the test fail if LatestReleaseVersion is invoked —
	// used to assert the reader is not consulted (explicit ?version=, or an
	// editor viewer).
	failIfCalled *testing.T
}

func (f *fakeReleaseReader) LatestReleaseVersion(_ context.Context, _ string) (string, error) {
	if f.failIfCalled != nil {
		f.failIfCalled.Fatal("LatestReleaseVersion should not be called")
	}
	return f.version, f.err
}

// newTestProjectPages builds a *ProjectPages behind a real renderer + i18n
// manager (in-memory backend), matching the pattern in
// pkg/weave/authpages/pages_test.go. ProjectDetailPage is mounted in
// production with only auth.WrapProjectRead (pkg/weave/pages Mount) — not
// through a slice's projectRead middleware chain — so this test drives the
// handler directly with the project/snapshot already attached to context,
// the same way that middleware would have left it.
func newTestProjectPages(t *testing.T, reader auth.LatestReleaseReader) *ProjectPages {
	t.Helper()

	i18nMgr, err := i18n.New(i18n.Config{
		DefaultLanguage:  "en",
		FallbackLanguage: "en",
		Storage:          backend.NewMemoryBackend(),
		Languages: []i18n.Language{
			{Code: "en", Name: "English", EnglishName: "English", Direction: "ltr", Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	renderer, err := weavetemplates.NewRenderer(func(names ...string) template.HTML {
		return ""
	}, i18nMgr, weavetemplates.AnalyticsConfig{})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	return NewProjectPages(slog.Default(), renderer, nil, i18nMgr, nil, reader)
}

func requestWithProjectContext(target string, project *domain.Project, snap *auth.AuthSnapshot) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	ctx := req.Context()
	ctx = auth.WithProject(ctx, project)
	ctx = auth.WithSnapshot(ctx, snap)
	return req.WithContext(ctx)
}

func publicProjectWithRelease() *domain.Project {
	return &domain.Project{Entity: domain.Entity{ID: "TPC"}, OwnerID: "OWNER", Visibility: "public"}
}

func TestProjectDetailPage_AnonymousPublicViewer_UsesLatestRelease(t *testing.T) {
	pages := newTestProjectPages(t, &fakeReleaseReader{version: "1.2.0"})
	project := publicProjectWithRelease()
	snap := &auth.AuthSnapshot{IsAnonymous: true}

	req := requestWithProjectContext("/projects/TPC", project, snap)
	rr := httptest.NewRecorder()

	pages.ProjectDetailPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-prop-schema-url="/projects/TPC/page-schema?version=1.2.0"`) {
		t.Fatalf("schema-url missing resolved release version: %s", body)
	}
}

func TestProjectDetailPage_Editor_ServesHotNoVersion(t *testing.T) {
	pages := newTestProjectPages(t, &fakeReleaseReader{failIfCalled: t})
	project := publicProjectWithRelease()
	snap := &auth.AuthSnapshot{Roles: map[string]string{"project:TPC": "maintainer"}}

	req := requestWithProjectContext("/projects/TPC", project, snap)
	rr := httptest.NewRecorder()

	pages.ProjectDetailPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-prop-schema-url="/projects/TPC/page-schema"`) {
		t.Fatalf("editor schema-url should be version-less: %s", body)
	}
	if strings.Contains(body, "version=") {
		t.Fatalf("editor schema-url should not carry a version: %s", body)
	}
}

func TestProjectDetailPage_ExplicitVersion_WinsOverLatest(t *testing.T) {
	pages := newTestProjectPages(t, &fakeReleaseReader{failIfCalled: t})
	project := publicProjectWithRelease()
	snap := &auth.AuthSnapshot{IsAnonymous: true}

	req := requestWithProjectContext("/projects/TPC?version=0.1.0", project, snap)
	rr := httptest.NewRecorder()

	pages.ProjectDetailPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-prop-schema-url="/projects/TPC/page-schema?version=0.1.0"`) {
		t.Fatalf("explicit version should win: %s", body)
	}
}

func TestProjectDetailPage_AnonymousPublicViewer_NoRelease_ServesHot(t *testing.T) {
	pages := newTestProjectPages(t, &fakeReleaseReader{version: ""})
	project := publicProjectWithRelease()
	snap := &auth.AuthSnapshot{IsAnonymous: true}

	req := requestWithProjectContext("/projects/TPC", project, snap)
	rr := httptest.NewRecorder()

	pages.ProjectDetailPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-prop-schema-url="/projects/TPC/page-schema"`) {
		t.Fatalf("no-release schema-url should be version-less: %s", body)
	}
}
