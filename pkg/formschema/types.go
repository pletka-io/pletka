package formschema

import (
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
)

// FormSchema is the top-level response from the form schema API.
type FormSchema struct {
	EntityType string          `json:"entity_type"`
	Mode       string          `json:"mode"`            // create, edit, view, override
	Theme      string          `json:"theme,omitempty"` // "default", "danger"
	Endpoint   *SchemaEndpoint `json:"endpoint,omitempty"`
	Delete     *DeleteAction   `json:"delete,omitempty"`
	Sections   []Section       `json:"sections"`
	Context    *SchemaContext  `json:"context,omitempty"`
	UI         SchemaUI        `json:"ui"`
}

// DeleteAction declares a destructive button that lives inside an edit
// form. When present, FormRenderer shows a danger-styled "Delete"
// control next to submit/cancel; clicking it pops the configured
// confirmation, then issues an HTTP DELETE to URL. On success the
// browser navigates to SuccessRedirectURL (defaults to the entity
// list).
type DeleteAction struct {
	URL                string             `json:"url"`
	Label              domain.Localizable `json:"label,omitempty"`
	Confirm            *ConfirmConfig     `json:"confirm,omitempty"`
	SuccessRedirectURL string             `json:"success_redirect_url,omitempty"`
	SuccessMessage     domain.Localizable `json:"success_message,omitempty"`
}

// SchemaEndpoint describes where to submit the form.
type SchemaEndpoint struct {
	Method  string         `json:"method"` // POST, PUT
	URL     string         `json:"url"`
	Confirm *ConfirmConfig `json:"confirm,omitempty"`
}

// ConfirmConfig triggers a confirmation dialog before form submission.
// Localizable fields carry either a raw domain.Translations literal or
// an i18n.LocalizedText (key + fallback) — both satisfy the interface.
type ConfirmConfig struct {
	Title        domain.Localizable `json:"title"`
	Message      domain.Localizable `json:"message"`
	ConfirmLabel domain.Localizable `json:"confirm_label"`
}

// SchemaContext provides information about the invocation context.
type SchemaContext struct {
	Source  string `json:"source,omitempty"` // "model", "collection", "standalone"
	ModelID string `json:"model_id,omitempty"`
	Mode    string `json:"mode"` // create, edit, override
}

// SchemaUI contains translated UI strings for the form chrome.
//
// Label-bearing fields use the `domain.Localizable` interface so they
// accept either a raw `domain.Translations` literal (legacy) or an
// `i18n.LocalizedText` (key + fallback, walker-enriched at the
// response edge). Both marshal to the same `{lang: value}` JSON
// shape; the interface gives compile-time safety against assigning
// arbitrary values.
type SchemaUI struct {
	SubmitLabel    domain.Localizable `json:"submit_label,omitempty"`
	CancelLabel    domain.Localizable `json:"cancel_label,omitempty"`
	SuccessMessage domain.Localizable `json:"success_message,omitempty"`
	// SuccessRedirectURLTemplate, when set, navigates the browser to this URL
	// after a successful submit. Supports {id} placeholder substituted from
	// the create response body. Overrides any onsuccess parent callback.
	SuccessRedirectURLTemplate string `json:"success_redirect_url_template,omitempty"`
	// RevealField, when set, names a key in the create/submit response body
	// whose value is a one-time secret (e.g. a generated password). The
	// frontend shows it once in a copy dialog after success. Domain
	// knowledge stays here — the renderer is generic.
	RevealField string             `json:"reveal_field,omitempty"`
	RevealLabel domain.Localizable `json:"reveal_label,omitempty"`
	Languages   []LanguageInfo     `json:"languages"`
	PrimaryLang string             `json:"primary_language"`
}

// LanguageInfo provides metadata for a language used in multilingual form fields.
type LanguageInfo struct {
	Code string `json:"code"` // ISO 639-1 code (e.g., "en")
	Name string `json:"name"` // Native name (e.g., "Nederlands")
	Flag string `json:"flag"` // Flag emoji (e.g., "🇳🇱")
}

// Section groups related fields with an optional label and collapse state.
//
// VisibleWhen is an optional gate that hides the entire section (legend
// + fieldset frame) when the rule doesn't match. Useful for sections
// that exist conditionally on a top-level toggle (e.g. the arches
// integration's "Arches connection (manual)" section, which lives
// only when source=manual). Without this, declaring per-field
// visible_when on every child still leaves an empty fieldset.
type Section struct {
	ID          string             `json:"id"`
	Label       domain.Localizable `json:"label"`
	Collapsed   bool               `json:"collapsed,omitempty"`
	Fields      []FieldDef         `json:"fields"`
	VisibleWhen *VisibilityRule    `json:"visible_when,omitempty"`
}

// FieldDef describes a single form field: its widget, validation, value, and options.
type FieldDef struct {
	Name                 string             `json:"name"`
	Widget               string             `json:"widget"` // multilingual-text, select, system-name-preview, etc.
	Required             bool               `json:"required,omitempty"`
	Readonly             bool               `json:"readonly,omitempty"`
	ImmutableAfterCreate bool               `json:"immutable_after_create,omitempty"`
	DerivedFrom          string             `json:"derived_from,omitempty"`
	Label                domain.Localizable `json:"label"`
	Help                 domain.Localizable `json:"help,omitempty"`
	Value                any                `json:"value"`
	ResolvedValue        *ResolvedValue     `json:"resolved_value,omitempty"`
	Validation           *ValidationRules   `json:"validation,omitempty"`
	Options              []SelectOption     `json:"options,omitempty"`
	OptionsURL           string             `json:"options_url,omitempty"`
	SearchURL            string             `json:"search_url,omitempty"`
	CheckURL             string             `json:"check_url,omitempty"`
	CreateURL            string             `json:"create_url,omitempty"`
	CreateLabel          domain.Localizable `json:"create_label,omitempty"`
	VisibleWhen          *VisibilityRule    `json:"visible_when,omitempty"`
	EntityType           string             `json:"entity_type,omitempty"`

	// Dependent-field metadata — used by FormRenderer to wire cascading
	// selects (e.g. ontology → version → extensions). Optional.
	// OptionsURL is declared above and shared with the create-capability field
	// from the field-migration branch; here it also carries {field_name}
	// template tokens when DependsOn is set.
	DependsOn         []string `json:"depends_on,omitempty"`
	HiddenUntilFilled []string `json:"hidden_until_filled,omitempty"`

	// Internal: not serialized. Used for role-based filtering.
	MinRole string `json:"-"`
}

// ResolvedValue shows the effective value and its override chain.
type ResolvedValue struct {
	Effective any               `json:"effective"`
	Layers    []ResolutionLayer `json:"layers"`
}

// ResolutionLayer represents one level in the override chain.
type ResolutionLayer struct {
	Source string             `json:"source"` // "field", "model", "collection"
	Value  any                `json:"value"`
	Label  domain.Localizable `json:"label,omitempty"`
}

// ValidationRules defines client-side and server-side validation constraints.
type ValidationRules struct {
	MinLength    *int   `json:"min_length,omitempty"`
	MaxLength    *int   `json:"max_length,omitempty"`
	Pattern      string `json:"pattern,omitempty"`
	UniqueWithin string `json:"unique_within,omitempty"` // "project_entity_type"
}

// VisibilityRule controls conditional field visibility.
// The field is shown only when the referenced field's value matches.
type VisibilityRule struct {
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

// SelectOption represents a choice in a select/dropdown or radio-group widget.
type SelectOption struct {
	Value       string             `json:"value"`
	Label       domain.Localizable `json:"label"`
	Description domain.Localizable `json:"description,omitempty"`
	Status      string             `json:"status,omitempty"`
	// SemanticID is the entity's human-readable id (e.g. "TPE.CAT.1").
	// Populated for category/model/collection options so dropdowns can
	// render a stable badge alongside the label.
	SemanticID string `json:"semantic_id,omitempty"`
	// SourceProjectID identifies the ancestor project an inherited
	// option came from. Empty for options that belong to the current
	// project. Drives an "inherited from X" affordance in the picker.
	SourceProjectID string `json:"source_project_id,omitempty"`
	// SourceProjectLabel is the friendly UI name of SourceProjectID's
	// project, resolved once per ancestor in the chain walk (not per
	// option). Empty for options that belong to the current project.
	// The frontend prefers this over SourceProjectID, falling back to
	// the raw id when a label could not be resolved.
	SourceProjectLabel string `json:"source_project_label,omitempty"`
}

// Widget type constants.
const (
	WidgetMultilingualText      = "multilingual-text"
	WidgetMultilingualTextarea  = "multilingual-textarea"
	WidgetSystemNamePreview     = "system-name-preview"
	WidgetSelect                = "select"
	WidgetSearchSelect          = "search-select"
	WidgetVocabularyEntryPicker = "vocabulary-entry-picker"
	WidgetOntologyPath          = "ontology-path"
	WidgetSubfieldPaths         = "subfield-paths"
	WidgetCheckbox              = "checkbox"
	WidgetText                  = "text"
	WidgetTextarea              = "textarea"
	WidgetNumber                = "number"
	WidgetHidden                = "hidden"
	WidgetRadioGroup            = "radio-group"
	WidgetReadonlyStat          = "readonly-stat"
	WidgetReadonlyTable         = "readonly-table"
	WidgetPillMultiSelect       = "pill-multi-select"
	WidgetPrefixInput           = "prefix-input"
	WidgetSlugInput             = "slug-input"
	WidgetPassword              = "password"
	// WidgetOntologyTree renders an expandable extends-hierarchy multi-select
	// (base's direct children, drill into sub-extensions).
	WidgetOntologyTree = "ontology-tree"
)

// Marker calls make schema-emitted frontend widgets discoverable by manifest
// conformance tests without changing the public constant API.
var _ = []string{
	frontendrefs.FormWidget("multilingual-text"),
	frontendrefs.FormWidget("multilingual-textarea"),
	frontendrefs.FormWidget("system-name-preview"),
	frontendrefs.FormWidget("select"),
	frontendrefs.FormWidget("search-select"),
	frontendrefs.FormWidget("vocabulary-entry-picker"),
	frontendrefs.FormWidget("ontology-path"),
	frontendrefs.FormWidget("subfield-paths"),
	frontendrefs.FormWidget("checkbox"),
	frontendrefs.FormWidget("text"),
	frontendrefs.FormWidget("textarea"),
	frontendrefs.FormWidget("number"),
	frontendrefs.FormWidget("radio-group"),
	frontendrefs.FormWidget("readonly-stat"),
	frontendrefs.FormWidget("readonly-table"),
	frontendrefs.FormWidget("pill-multi-select"),
	frontendrefs.FormWidget("prefix-input"),
	frontendrefs.FormWidget("slug-input"),
	frontendrefs.FormWidget("password"),
	frontendrefs.FormWidget("ontology-tree"),
}

// Mode constants
const (
	ModeCreate   = "create"
	ModeEdit     = "edit"
	ModeView     = "view"
	ModeOverride = "override"
)

// FilterForRole removes fields and sections the given role cannot access.
// Returns a new FormSchema (does not mutate the original).
func (s *FormSchema) FilterForRole(role string) *FormSchema {
	roleLevel := roleToLevel(role)

	filtered := *s
	filtered.Sections = nil

	for _, section := range s.Sections {
		var fields []FieldDef
		for _, f := range section.Fields {
			if f.MinRole == "" || roleToLevel(f.MinRole) <= roleLevel {
				fields = append(fields, f)
			}
		}
		if len(fields) > 0 {
			sec := section
			sec.Fields = fields
			filtered.Sections = append(filtered.Sections, sec)
		}
	}

	// Viewers get no endpoint (can't submit)
	if role == "viewer" {
		filtered.Endpoint = nil
	}

	return &filtered
}

func roleToLevel(role string) int {
	switch role {
	case "viewer":
		return 0
	case "contributor":
		return 1
	case "maintainer":
		return 2
	case "owner":
		return 3
	case "admin", "superadmin":
		return 4
	default:
		return 0
	}
}

// MakeReadOnly turns a mutable form schema into a read-only view contract.
// It preserves field values and labels but removes submit/delete affordances
// and marks every field readonly so widgets render in non-editable mode.
func (s *FormSchema) MakeReadOnly() {
	if s == nil {
		return
	}
	s.Endpoint = nil
	s.Delete = nil
	for si := range s.Sections {
		for fi := range s.Sections[si].Fields {
			s.Sections[si].Fields[fi].Readonly = true
			s.Sections[si].Fields[fi].CreateURL = ""
			s.Sections[si].Fields[fi].CreateLabel = nil
			s.Sections[si].Fields[fi].CheckURL = ""
		}
	}
}

// CompositePaneSchema is the envelope returned at a pane-schema URL when a
// settings section needs more than one panel. Each panel points at a child
// schema URL (form-schema or list-schema) that the frontend island dispatcher
// mounts in order. The envelope itself carries no domain knowledge — only
// ordering and display metadata.
type CompositePaneSchema struct {
	Kind     string             `json:"kind"` // always "composite-pane"
	Title    domain.Localizable `json:"title,omitempty"`
	Subtitle domain.Localizable `json:"subtitle,omitempty"`
	Panels   []CompositePanel   `json:"panels"`
}

// CompositePanel references one child schema (form or list) within a
// composite pane. The dispatcher probes the SchemaURL once to determine
// whether to render via FormRenderer or ListManager — unless Kind is
// set, in which case the panel uses a dedicated Svelte component
// (e.g. "linked-ontologies" for the rich grouped ontology pane).
type CompositePanel struct {
	ID        string             `json:"id"`
	Label     domain.Localizable `json:"label,omitempty"`
	SchemaURL string             `json:"schema_url"`

	// Kind is an optional discriminator that routes the panel to a
	// dedicated component instead of the generic FormRenderer /
	// ListManager probe. Empty = use the probe (default).
	Kind string `json:"kind,omitempty"`
}

// ActionSchema describes a single button that POSTs to an endpoint and
// renders the returned ActionResultUI. Used by the integrations hub to
// surface per-integration actions (e.g. "Upload to 3M") without the
// frontend knowing anything about the integration. Reuses SchemaEndpoint
// + ConfirmConfig so existing FormRenderer confirm handling applies.
type ActionSchema struct {
	Kind           string             `json:"kind"` // always "action"
	ID             string             `json:"id"`
	Label          domain.Localizable `json:"label"`
	Help           domain.Localizable `json:"help,omitempty"`
	Theme          string             `json:"theme,omitempty"`
	Endpoint       SchemaEndpoint     `json:"endpoint"`
	Disabled       bool               `json:"disabled,omitempty"`
	DisabledReason domain.Localizable `json:"disabled_reason,omitempty"`
	// Category groups actions in the UI. "" (default) = primary
	// action shown on the saved-config row; "admin" = collapses
	// behind a dedicated Admin panel for housekeeping ops.
	Category string `json:"category,omitempty"`
	// Result tells the UI how to render this action's response.
	// "" or "toast"      = brief banner + optional link (default)
	// "panel"            = render ActionResultUI.Panel via PanelRenderer
	// "inline-refresh"   = run, then auto-fire sibling "panel" actions
	Result string `json:"result,omitempty"`
}

// ActionResultUI is the JSON body returned by an action endpoint. The
// frontend renders Status as a banner; LinkURL/LinkLabel become an
// inline anchor when present.
type ActionResultUI struct {
	Status    string             `json:"status"` // "success" | "error"
	Message   domain.Localizable `json:"message"`
	LinkURL   string             `json:"link_url,omitempty"`
	LinkLabel domain.Localizable `json:"link_label,omitempty"`
	// Panel carries structured payload data when the corresponding
	// ActionSchema.Result is "panel". Generic — the frontend reads
	// "kind" inside the payload to pick a renderer (status, audit,
	// counts, ...). Empty for toast actions.
	Panel any `json:"panel,omitempty"`
}

// ActionGroupSchema bundles one logical action that can be dispatched
// to multiple targets — used by the integrations hub when a project
// has multiple configs for the same integration. The frontend renders
// a target selector + a single action button; when Targets has length
// 1 it degrades to a plain ActionSchema render so single-config
// projects keep the simpler UX.
type ActionGroupSchema struct {
	Kind    string             `json:"kind"` // always "action-group"
	ID      string             `json:"id"`
	Label   domain.Localizable `json:"label"`
	Help    domain.Localizable `json:"help,omitempty"`
	Theme   string             `json:"theme,omitempty"`
	Targets []ActionTarget     `json:"targets"`
}

// ActionTarget is one dispatchable destination for an ActionGroupSchema.
type ActionTarget struct {
	ID       string         `json:"id"`    // typically the config_id
	Label    string         `json:"label"` // operator-typed display
	Endpoint SchemaEndpoint `json:"endpoint"`
}
