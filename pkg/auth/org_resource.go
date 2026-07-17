package auth

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
)

type organizationBySlugReader interface {
	GetBySlug(ctx context.Context, slug string) (*domain.Organization, error)
}

type orgKey struct{}

// WithOrg attaches an organization to the request context.
func WithOrg(ctx context.Context, org *domain.Organization) context.Context {
	return context.WithValue(ctx, orgKey{}, org)
}

// OrgFromContext returns the organization attached by WithOrgResource.
func OrgFromContext(ctx context.Context) *domain.Organization {
	if org, ok := ctx.Value(orgKey{}).(*domain.Organization); ok && org != nil {
		return org
	}
	return nil
}

// OrgResourceFromContext returns an auth.Resource for the current org scope.
func OrgResourceFromContext(ctx context.Context) Resource {
	org := OrgFromContext(ctx)
	if org == nil {
		return Resource{ScopeType: "org"}
	}
	visibility := org.Visibility
	if visibility == "" {
		visibility = "private"
	}
	return Resource{
		ScopeType:  "org",
		ID:         org.ID,
		Visibility: visibility,
	}
}

// WithOrgResource loads the organization identified by the {slug} URL
// parameter and attaches it to the request context.
func WithOrgResource(reader organizationBySlugReader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := chi.URLParam(r, "slug")
			if slug == "" {
				next.ServeHTTP(w, r)
				return
			}
			org, err := reader.GetBySlug(r.Context(), slug)
			if err != nil {
				http.Error(w, "failed to load organization", http.StatusInternalServerError)
				return
			}
			if org == nil {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithOrg(r.Context(), org)))
		})
	}
}

// RequireOrgRead gates a route on the OrgRead capability for the org
// resource attached to the request context by WithOrgResource. Chain it
// after WithOrgResource. Unauthorized requests get a 404 (not 403) so org
// existence isn't leaked to callers without read access.
func RequireOrgRead(next http.Handler) http.Handler {
	return requireOrgCapability(OrgRead, next)
}

// RequireOrgEdit gates a route on the OrgEdit capability for the org
// resource attached to the request context by WithOrgResource. Chain it
// after WithOrgResource.
func RequireOrgEdit(next http.Handler) http.Handler {
	return requireOrgCapability(OrgEdit, next)
}

func requireOrgCapability(capability Capability, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !FromContext(r.Context()).Can(capability, OrgResourceFromContext(r.Context()), nil) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
