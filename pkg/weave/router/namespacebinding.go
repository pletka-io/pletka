package router

import (
	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/weave/namespacebinding"
)

func mountNamespaceBinding(r chi.Router, host namespacebinding.Host) {
	mountNamespaceBindingContribution(r, host, "namespacebinding.routes")
}

func mountNamespaceBindingAdmin(r chi.Router, host namespacebinding.Host) {
	mountNamespaceBindingContribution(r, host, "namespacebinding.admin.routes")
}

func mountNamespaceBindingContribution(r chi.Router, host namespacebinding.Host, mountID string) {
	mountProviderContribution(r, "namespacebinding", mountID, namespacebinding.Slice{Host: host})
}
