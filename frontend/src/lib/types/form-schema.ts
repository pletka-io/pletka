export interface Translations {
  [lang: string]: string;
}

export interface ConfirmConfig {
  title: Translations;
  message: Translations;
  confirm_label: Translations;
}

export interface SchemaEndpoint {
  method: string;
  url: string;
  confirm?: ConfirmConfig;
}

export interface FormSchema {
  entity_type: string;
  mode: string;
  theme?: string;
  endpoint?: SchemaEndpoint;
  delete?: DeleteAction;
  sections: Section[];
  context?: { source?: string; model_id?: string; mode: string };
  ui: SchemaUI;
}

/** Mirrors Go formschema.DeleteAction. Renders a destructive button
 *  inside the form when present (typically only in edit mode). */
export interface DeleteAction {
  url: string;
  label?: Translations;
  confirm?: ConfirmConfig;
  success_redirect_url?: string;
  success_message?: Translations;
}

export interface SchemaUI {
  submit_label?: Translations;
  cancel_label?: Translations;
  success_message?: Translations;
  /** Navigate here after successful submit. Supports {id} substituted from response. */
  success_redirect_url_template?: string;
  /** Names a key in the submit response holding a one-time secret to show
   *  once in a copy dialog (e.g. a generated password). */
  reveal_field?: string;
  reveal_label?: Translations;
  languages: LanguageInfo[];
  primary_language: string;
}

export interface LanguageInfo {
  code: string;
  name: string;
  flag: string;
}

export interface Section {
  id: string;
  label: Translations;
  collapsed?: boolean;
  fields: FieldDef[];
  /** When set, hides the entire section (legend + fieldset frame)
   *  if the rule doesn't match. Mirrors FieldDef.visible_when but
   *  applied at the section grouping level, so the empty frame
   *  doesn't render when all children are individually hidden. */
  visible_when?: VisibilityRule;
}

export interface FieldDef {
  name: string;
  widget: string;
  required?: boolean;
  readonly?: boolean;
  immutable_after_create?: boolean;
  derived_from?: string;
  label: Translations;
  help?: Translations;
  value: any;
  resolved_value?: ResolvedValue;
  validation?: ValidationRules;
  options?: SelectOption[];
  /** Template URL with {field_name} substitutions; fetched on load (field-migration) or on dependency change (settings-fixes). */
  options_url?: string;
  /** Template URL with {field_name} substitutions for typeahead search widgets. */
  search_url?: string;
  /** Endpoint for inline async checks (e.g. prefix availability). Receives ?prefix=<value>. */
  check_url?: string;
  create_url?: string;
  create_label?: Translations;
  visible_when?: VisibilityRule;
  entity_type?: string;
  /** Field names whose values this field depends on (cascading selects). */
  depends_on?: string[];
  /** Field names that must be non-empty before this field is visible. */
  hidden_until_filled?: string[];
}

export interface ResolvedValue {
  effective: any;
  layers: { source: string; value: any; label?: Translations }[];
}

export interface ValidationRules {
  min_length?: number;
  max_length?: number;
  pattern?: string;
  unique_within?: string;
}

export interface VisibilityRule {
  field: string;
  equals: string;
}

export interface SelectOption {
  value: string;
  label: Translations;
  description?: Translations;
  status?: string;
  semantic_id?: string;
  /** IDPrefix of the ancestor project an inherited option came from
   *  (e.g. "LA"). Empty for options in the current project. Pickers
   *  render it so same-named models/collections from a parent project
   *  are distinguishable from local ones. */
  source_project_id?: string;
  /** Friendly UI name of source_project_id's project. Preferred over
   *  source_project_id wherever both are present; falls back to the
   *  raw id when a label could not be resolved. */
  source_project_label?: string;
  /** Optional inline hint shown below the dropdown when this option
   *  is selected. Free-form plain text (URL, last-synced timestamp,
   *  health note). Surfaced by /admin/arches/api/instances/options. */
  hint?: string;
}

/** Mirrors Go formschema.ActionSchema. A single button that POSTs to
 *  Endpoint and renders the returned ActionResultUI. Used by the
 *  integrations hub to surface per-integration actions. */
export interface ActionSchema {
  kind: 'action';
  id: string;
  label: Translations;
  help?: Translations;
  theme?: string;
  endpoint: SchemaEndpoint;
  disabled?: boolean;
  disabled_reason?: Translations;
  /** "" (default) = primary action on the saved-config row.
   *  "admin"      = collapses behind a dedicated Admin slide-panel. */
  category?: string;
  /** "" or "toast" = brief banner (default).
   *  "panel"        = render ActionResultUI.panel via PanelRenderer.
   *  "inline-refresh" = run, then auto-fire sibling panel actions. */
  result?: 'toast' | 'panel' | 'inline-refresh' | '';
}

/** Mirrors Go formschema.ActionResultUI. JSON body returned by an
 *  action endpoint. */
export interface ActionResultUI {
  status: 'success' | 'error';
  message: Translations;
  link_url?: string;
  link_label?: Translations;
  /** Structured payload for actions whose ActionSchema.result is
   *  "panel". Frontend dispatches on `panel.kind` to a renderer
   *  (status grid, audit table, etc.). */
  panel?: {
    kind: string;
    data: any;
  };
}

/** Mirrors Go formschema.ActionGroupSchema. One logical action with
 *  multiple targets — used by integrations that a project has
 *  multi-configured. The frontend renders a target selector + single
 *  action button when targets.length > 1, and a plain button when 1. */
export interface ActionGroupSchema {
  kind: 'action-group';
  id: string;
  label: Translations;
  help?: Translations;
  theme?: string;
  targets: ActionTarget[];
}

/** Mirrors Go formschema.ActionTarget. */
export interface ActionTarget {
  id: string;
  label: string;
  endpoint: SchemaEndpoint;
}

// Helper to get translated text with fallback
export function tr(translations: Translations | undefined, lang: string): string {
  if (!translations) return '';
  return translations[lang] || translations['en'] || Object.values(translations)[0] || '';
}

// Simple slugify matching Go's slugifyName()
export function slugify(value: string): string {
  return value.trim().toLowerCase().replace(/[\s-]+/g, '_').replace(/[^a-z0-9_]/g, '');
}

export function routeSlugify(value: string): string {
  return value
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .replace(/-{2,}/g, '-');
}
