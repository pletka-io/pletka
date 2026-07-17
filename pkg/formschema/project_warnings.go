package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// ProjectSetupState captures the "is this project ready to use" signals the
// settings schema and project page need to render warnings and indicators.
// Add a field here when introducing a new warning rule below.
type ProjectSetupState struct {
	HasOntology bool
}

// projectWarningRule is the data-driven shape of a single warning. New
// warnings get appended to projectWarningRules; ComputeProjectWarnings
// applies them in order.
type projectWarningRule struct {
	id          string
	severity    string
	applies     func(ProjectSetupState) bool
	capability  auth.Capability
	message     domain.Localizable
	actionPath  func(projectID string) string
	actionLabel domain.Localizable
}

// projectWarningRules is the single source of truth for "what's wrong with
// this project". Order is presentation order in the UI banners.
var projectWarningRules = []projectWarningRule{
	{
		id:         "no-ontology",
		severity:   "warning",
		applies:    func(s ProjectSetupState) bool { return !s.HasOntology },
		capability: auth.ProjectEdit,
		message:    i18n.L("project_warnings.no_ontology.message", "Add at least one ontology before you can create fields, models, or collections in this project."),
		actionPath: func(id string) string {
			return fmt.Sprintf("/projects/%s/settings#ontology", id)
		},
		actionLabel: i18n.L("project_warnings.no_ontology.action", "Configure ontology"),
	},
}

// ComputeProjectWarnings returns the warnings that apply to the project
// for the given viewer. A warning is included only when its predicate
// matches AND the viewer has the capability needed to act on it — read-
// only viewers never see "fix this" prompts they can't action.
func ComputeProjectWarnings(projectID string, setup ProjectSetupState, snap *auth.AuthSnapshot, res auth.Resource) []SettingsWarning {
	var out []SettingsWarning
	for _, rule := range projectWarningRules {
		if !rule.applies(setup) {
			continue
		}
		if !snap.Can(rule.capability, res, nil) {
			continue
		}
		out = append(out, SettingsWarning{
			ID:          rule.id,
			Severity:    rule.severity,
			Message:     rule.message,
			ActionHref:  rule.actionPath(projectID),
			ActionLabel: rule.actionLabel,
		})
	}
	return out
}
