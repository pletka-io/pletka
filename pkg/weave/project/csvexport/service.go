package csvexport

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
)

func sprintfArgs(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

//go:embed downloads.gohtml
var templateFS embed.FS

// OntologyVersionLookup resolves an ontology-version row by ID. The
// ontology slice satisfies it via its Service; this slice keeps a
// narrow contract instead of pulling in the full reader interface.
type OntologyVersionLookup interface {
	GetByID(ctx context.Context, id string) (*domain.OntologyVersion, error)
}

// OntologyLookup resolves an ontology master row by ID for the
// "ontology name" column on the linked-ontologies CSV.
type OntologyLookup interface {
	GetByID(ctx context.Context, id string) (*domain.Ontology, error)
}

// Service is the slice's HTTP entry point. It holds the WeaveStore
// (the only data dependency now that the legacy GORM Repositories
// are gone), the ontology readers needed for the linked-ontologies
// CSV, and the parsed page template.
type Service struct {
	weave            domain.WeaveStore
	ontologies       OntologyLookup
	ontologyVersions OntologyVersionLookup
	i18n             i18n.Manager
	session          *session.Manager
	logger           *slog.Logger
	tmpl             *template.Template
}

// NewService builds the service. Returns an error only if the embedded
// template fails to parse, which would be a build-time bug.
//
// ontologies / ontologyVersions are nullable — when nil, the
// linked-ontologies export degrades to ID-only rows.
//
// i18nManager / sessionManager are nullable — when nil, page labels
// fall back to English defaults. Pass real instances from deps to get
// localised UI.
func NewService(
	weave domain.WeaveStore,
	ontologies OntologyLookup,
	ontologyVersions OntologyVersionLookup,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	logger *slog.Logger,
) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	tmpl, err := template.New("downloads.gohtml").ParseFS(templateFS, "downloads.gohtml")
	if err != nil {
		return nil, err
	}
	return &Service{
		weave:            weave,
		ontologies:       ontologies,
		ontologyVersions: ontologyVersions,
		i18n:             i18nManager,
		session:          sessionManager,
		logger:           logger,
		tmpl:             tmpl,
	}, nil
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
