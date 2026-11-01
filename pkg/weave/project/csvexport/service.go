package csvexport

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/session"
)

func sprintfArgs(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

//go:embed downloads.gohtml
var templateFS embed.FS

// Source loads one complete detached selection. The configured assembler source
// owns the read transaction and source authorization; exporters cannot load rows.
type Source interface {
	Project(context.Context, auth.ReadScope, string) (projectsnapshot.CompleteProject, error)
}

// ProjectAccess supplies current project ownership for authorization only.
// Exported names and metadata always come from Source.
type ProjectAccess interface {
	GetByID(context.Context, string) (*domain.Project, error)
}

// Service keeps current access policy separate from retained export content.
type Service struct {
	source   Source
	projects ProjectAccess
	i18n     i18n.Manager
	session  *session.Manager
	logger   *slog.Logger
	tmpl     *template.Template
}

// NewService requires the shared assembly boundary and current authorization
// lookup. Localization dependencies may be nil for English defaults.
func NewService(source Source, projects ProjectAccess, i18nManager i18n.Manager, sessionManager *session.Manager, logger *slog.Logger) (*Service, error) {
	if source == nil || projects == nil {
		return nil, fmt.Errorf("project CSV requires source and project access")
	}
	if logger == nil {
		logger = slog.Default()
	}
	tmpl, err := template.New("downloads.gohtml").ParseFS(templateFS, "downloads.gohtml")
	if err != nil {
		return nil, err
	}
	return &Service{source: source, projects: projects, i18n: i18nManager, session: sessionManager, logger: logger, tmpl: tmpl}, nil
}

// currentLang resolves the active language from the session, falling
// back to "en". Mirrors the helper used in pkg/weave/detailview.
func (s *Service) currentLang(r *http.Request) string {
	if s.session != nil {
		if lang := s.session.Language(r.Context()); lang != "" {
			return lang
		}
	}
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return "en"
}

// translate returns t(key, args...) when an i18n manager is available;
// otherwise returns fallback. Used so the page still renders a sensible
// English default in test setups that don't wire an i18n.Manager.
func (s *Service) translate(lang, key, fallback string, args ...any) string {
	if s.i18n == nil {
		if len(args) == 0 {
			return fallback
		}
		// Fallback uses the same %s placeholder convention as the
		// translation values, so callers don't need a separate
		// fmt.Sprintf for the default case.
		return sprintfArgs(fallback, args...)
	}
	return s.i18n.T(key, lang, args...)
}
