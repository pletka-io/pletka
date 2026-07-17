package actoradmin

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	weaveadmin "github.com/pletka-io/pletka/pkg/weave/admin"
)

type Host struct {
	Store        Store
	Auth         domain.AuthStore
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

func (h Host) Validate() error {
	var missing []string
	if h.Store == nil {
		missing = append(missing, "Store")
	}
	if len(missing) > 0 {
		return fmt.Errorf("actoradmin host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Mount(parent chi.Router, h Host) {
	if err := h.Validate(); err != nil {
		panic(err)
	}
	svc := NewService(h.Store, h.Auth, h.Logger)
	handler := NewHandler(svc, h.Languages, h.LangResolver)

	// Admin surface: /admin/users + /admin/institutions, super-admin only.
	parent.Group(func(r chi.Router) {
		r.Use(weaveadmin.RequireSuperAdmin)
		handler.Mount(r)
	})

	// Self-edit surface: /me/form-schema + /me. Handler-level auth
	// check rejects anonymous callers; any logged-in user can edit
	// their own profile.
	handler.MountSelf(parent)
}
