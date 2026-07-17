import type { Translations, LanguageInfo } from './form-schema';

export interface EntityListCapabilities {
  create?: {
    label: Translations;
    form_schema_url: string;
  };
  adopt?: {
    label: Translations;
    options_url: string;
    url: string;
    /** "Adopting into {project}" — server-resolved with the project
     *  name interpolated. Renders above the search box. */
    destination_label?: Translations;
  };
  edit?: {
    form_schema_url_template: string;
  };
  delete?: {
    url_template: string;
    reassignment?: {
      enabled: boolean;
      entity_label: Translations;
      count_field: string;
      options_from: string;
    };
  };
  stats?: {
    url_template: string;
  };
}

export interface AdoptableOption {
  value: string;
  label: Translations;
  semantic_id?: string;
  source_project_id: string;
  source_project_name?: string;
  ontology_scope?: string;
}

export interface RowAction {
  id: string;
  icon: string;
  label: Translations;
  style?: string;
  url_template?: string;
  method?: string;
  visible_when?: string;
}

export function evalVisibleWhen(expr: string | undefined, row: Record<string, unknown>): boolean {
  if (!expr) return true;
  // Support a conjunction of simple terms ("owned && !in_use"): every term
  // must hold. A term is "field" (truthy) or "!field" (falsy). A missing field
  // is falsy, so "!in_use" is true when the row omits in_use — models/
  // collections that don't emit the flag keep their prior visibility.
  return expr.split('&&').every((raw) => {
    const t = raw.trim();
    if (!t) return true;
    if (t.startsWith('!')) return !row[t.slice(1).trim()];
    return Boolean(row[t]);
  });
}

export interface ViewMode {
  id: string;
  label: Translations;
  default?: boolean;
  /** Optional widget name; when present, selecting this mode swaps the active row widget. */
  widget?: string;
}

export interface EntityListSchema {
  entity_type: string;
  title: Translations;
  empty_state?: {
    icon: string;
    title: Translations;
    message: Translations;
  };
  data_url: string;
  data_key: string;
  detail_url_template: string;
  edit_url_template?: string;
  project_id: string;

  // Toolbar
  search?: SearchConfig;
  filters?: FilterConfig[];
  sort_options: SortOption[];
  default_sort: string;

  // Display
  row_widget?: string;
  view_modes?: ViewMode[];
  row_layout: EntityRowLayout;
  column_header?: ColumnHeaderConfig;
  grouping?: GroupConfig;
  pagination?: PaginationConfig;
  editor?: EntityListEditor;

  // Actions
  capabilities?: EntityListCapabilities;
  row_actions?: RowAction[];

  ui: {
    languages: LanguageInfo[];
    primary_language: string;
  };
}

export interface EntityListEditor {
  widget: string;
  /** API URLs (or {id}-style url templates) the editor widget calls —
   *  schema-provided so the frontend never constructs an API URL.
   *  Keys are widget-specific. */
  endpoints?: Record<string, string>;
}

export interface SearchConfig {
  placeholder: Translations;
  param_name: string;
}

export interface FilterConfig {
  key: string;
  label: Translations;
  param_name: string;
  type: string;
  /** Inline options; used when small/cheap to embed. */
  options?: FilterOption[];
  /** URL to GET `{options: FilterOption[]}` lazily when the drawer opens. */
  options_url?: string;
  /** "checklist" (default, fetched once) or "typeahead" (search-as-you-type). */
  options_type?: 'checklist' | 'typeahead';
  /** When true, multiple values can be selected; joined with "," on the wire. */
  multi?: boolean;
}

export interface FilterOption {
  value: string;
  label: Translations;
  /** Pre-selected when the URL carries no value for the parent filter
   *  param. Applied as the initial filter state on first render. */
  default?: boolean;
}

export interface SortOption {
  value: string;
  label: Translations;
}

export interface EntityRowLayout {
  title_field: string;
  subtitle_field: string;
  byline_field?: string;
  identity_fields: IdentityField[];
  semantic_badges: BadgeConfig[];
  process_badges: BadgeConfig[];
  count_columns?: CountColumn[];
  /** Field holding the []PathElement ontology path; rendered as a class/
   *  property pill chain on its own line in detailed view (fields). */
  path_field?: string;
  /** Optional leading box rendered before the title — used for the
   *  ontology scope class on model/collection/field rows. */
  lead_box?: LeadBoxConfig;
}

export interface LeadBoxConfig {
  key: string;
  style?: string;
}

export interface CountColumn {
  key: string;
  label: Translations;
  zero_placeholder?: string;
}

export interface ColumnHeaderConfig {
  show: boolean;
  id_label?: Translations;
  title_label?: Translations;
}

export interface IdentityField {
  key: string;
  style: string;
}

export interface BadgeConfig {
  key: string;
  type: 'text' | 'status' | 'ownership' | 'count' | 'flag';
  style?: string;
  label?: Translations;
  hide_empty?: boolean;
}

export interface GroupConfig {
  options: GroupOption[];
  default_key: string;
  param_name: string;
}

export interface GroupOption {
  value: string;
  label: Translations;
}

export interface PaginationConfig {
  page_size: number;
  param_name: string;
  per_page_name: string;
  page_size_options?: number[];
}
