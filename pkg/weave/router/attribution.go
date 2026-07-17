package router

import (
	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/weave/attribution"
)

func mountAttribution(r chi.Router, host attribution.Host) {
	mountProviderContribution(r, "attribution", "attribution.routes", attribution.Slice{Host: host})
}
