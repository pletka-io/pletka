package router

import (
	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/weave/category"
)

func mountCategory(r chi.Router, host category.Host) {
	mountProviderContribution(r, "category", "category.routes", category.Slice{Host: host})
}
