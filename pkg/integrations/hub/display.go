package hub

import (
	"context"

	"github.com/pletka-io/pletka/pkg/integrations/registry"
)

// ManagedLink is one operator link shown on a managed config row (e.g.
// the fleet instance's admin page).
type ManagedLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// ManagedLinksResolver returns the operator links for a managed fleet
// instance. The build that owns the fleet supplies it; core only
// renders what comes back.
type ManagedLinksResolver func(ctx context.Context, instanceID string) []ManagedLink

// SetManagedLinksResolver installs the links resolver. Idempotent.
func (s *Service) SetManagedLinksResolver(r ManagedLinksResolver) {
	s.managedLinks = r
}

// ResolveManagedLinks returns the operator links for a managed config.
// Fleet admin pages are super-admin only, so other viewers get none.
func (s *Service) ResolveManagedLinks(ctx context.Context, superAdmin bool, instanceID string) []ManagedLink {
	if !superAdmin || instanceID == "" || s.managedLinks == nil {
		return nil
	}
	return s.managedLinks(ctx, instanceID)
}

// ConfigTarget is the value of the integration's TargetField in cfg,
// shown next to the config label. Managed configs return "" — their
// managed badge already says where they point.
func ConfigTarget(md registry.Metadata, cfg *ProjectIntegration) string {
	if md.TargetField == "" || cfg.ManagedInstanceID != "" {
		return ""
	}
	v, _ := cfg.Config[md.TargetField].(string)
	return v
}

// ConfigUnconnected reports a config that points nowhere: the
// integration declares a target field, yet the config has neither a
// managed instance nor a target value. Typically its managed instance
// was deleted, which cleared the link.
func ConfigUnconnected(md registry.Metadata, cfg *ProjectIntegration) bool {
	return md.TargetField != "" && cfg.ManagedInstanceID == "" && ConfigTarget(md, cfg) == ""
}
