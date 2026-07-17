// Package registry is the public contract for Pletka integrations.
//
// An integration plugs an external service (e.g. FORTH 3M) into Pletka
// projects. It declares per-project configuration, the entity formats it
// applies to, and the actions it offers. The integrations hub serves
// generic config + action HTTP endpoints and dispatches by integration ID
// — integrations never own routes.
//
// Platform integrations can live outside the public core module. They expose
// `func New() registry.Integration` and are included by the app composition
// layer when constructing the registry for a given build.
package registry

import (
	"context"
	"io"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

// Format is the generator output format an integration can consume.
// Mirrors the string values of pkg/weave/generators.Format ("x3ml",
// "x3ml-b", "turtle", …) but lives here as a plain string so the
// registry package does not depend on pkg/weave/generators. The hub converts at
// the boundary.
type Format = string

// EntityKind mirrors pkg/weave/generators.EntityKind values
// ("project", "model", "collection", "field") as a plain string for
// the same reason.
type EntityKind = string

// Metadata is the human-facing description of an integration.
type Metadata struct {
	DisplayName domain.Localizable
	Description domain.Localizable
	Icon        string
}

// ConfigRequest carries the context the hub provides when asking an
// integration to build its config form.
type ConfigRequest struct {
	ProjectID string
	Lang      string
	Languages []formschema.LanguageInfo
}

// ActionSpec declares a single action an integration exposes (e.g.
// "upload"). The hub turns each spec into an HTTP route at
// POST /projects/{projectID}/integrations/{integrationID}/actions/{ID}.
type ActionSpec struct {
	ID      string
	Label   domain.Localizable
	Help    domain.Localizable
	Theme   string
	Confirm *formschema.ConfirmConfig
	// AppliesTo restricts the action to a subset of the integration's
	// AppliesTo formats. Empty means the action applies to every format
	// declared by the integration.
	AppliesTo []Format
	// Category groups actions in the UI. "" (default) = primary action
	// rendered as a top-level button on the saved-config row; "admin"
	// = collapses behind a dedicated Admin panel for housekeeping ops
	// (reset, publish, status, reindex, restart).
	Category string
	// Result tells the UI how to render the action's response.
	//   ""              = "toast" (default; small banner + optional link)
	//   "toast"         = same as default
	//   "panel"         = render ActionResult.Panel as a structured
	//                     card inside the admin slide view
	//   "inline-refresh" = run the action, then auto-fire any sibling
	//                     "panel" actions so the panel state reflects
	//                     the new server state
	Result string
}

// ArtifactProvider renders a generator output (e.g. X3ML B zip) into an
// in-memory reader. The hub wires this to the generator service before
// calling RunAction; integrations never touch generator internals.
type ArtifactProvider func(Format) (io.Reader, string, error)

// ProjectArtifactProvider returns a project-level bundle in the
// requested Format (e.g. an arches loader pack zip). Unlike
// ArtifactProvider which scopes to a single entity, this closure spans
// the whole project. The hub wires it for actions that operate at
// project scope; nil otherwise.
type ProjectArtifactProvider func(Format) (io.Reader, string, error)

// ActionInput is what the hub passes to RunAction.
type ActionInput struct {
	ProjectID  string
	EntityKind EntityKind
	EntityID   string
	Format     Format
	// Config holds the decrypted per-project config map. Secret fields
	// declared by SecretFields() are populated here with their plaintext
	// values; they are never logged or echoed back through the API.
	Config          map[string]any
	Artifact        ArtifactProvider
	ProjectArtifact ProjectArtifactProvider
}

// ActionResult is the integration-facing return type. The hub adapts it
// into the JSON ActionResultUI shape.
type ActionResult struct {
	Status    string // "success" | "error"
	Message   domain.Localizable
	LinkURL   string
	LinkLabel domain.Localizable
	// Panel carries structured payload data for actions whose
	// ActionSpec.Result is "panel". The frontend renders this through
	// a generic PanelRenderer instead of (or in addition to) the
	// banner Message. Anything JSON-marshalable; common shapes:
	// status snapshots, audit-log tails, count summaries.
	Panel any
}

// Integration is the contract every integration package implements.
// All methods are pure with respect to global state — config and
// secrets arrive through inputs, never through env or singletons.
type Integration interface {
	ID() string
	Metadata() Metadata
	AppliesTo() []Format
	SecretFields() []string
	ConfigSchema(req ConfigRequest, current map[string]any) *formschema.FormSchema
	ValidateConfig(raw map[string]any) (normalised map[string]any, fieldErrors map[string][]string)
	Actions() []ActionSpec
	RunAction(ctx context.Context, actionID string, in ActionInput) (ActionResult, error)
}
