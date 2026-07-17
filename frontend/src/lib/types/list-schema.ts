import type { Translations, LanguageInfo } from './form-schema';

export interface ListSchema {
  entity_type: string;
  title: Translations;
  empty_state?: {
    icon: string;
    title: Translations;
    message: Translations;
  };
  data_url: string;
  data_key: string;
  capabilities: Capabilities;
  columns: Column[];
  row_actions: RowAction[];
  ui: {
    languages: LanguageInfo[];
    primary_language: string;
  };
  /** Names a boolean row field; truthy rows hide inline row actions. */
  per_row_readonly_field?: string;
}

export interface Capabilities {
  reorder?: {
    enabled: boolean;
    url: string;
    order_field: string;
  };
  inline_rename?: {
    enabled: boolean;
    field: string;
    save_url_template: string;
  };
  create?: {
    label: Translations;
    form_schema_url: string;
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

export interface Column {
  key: string;
  label?: Translations;
  type: 'translation' | 'badge' | 'text_badge' | 'computed_badge' | 'text' | 'deprecated_badge' | 'boolean_badge';
  primary?: boolean;
  secondary?: boolean;
  truncate?: boolean;
  badge_style?: string;
  zero_style?: string;
  hide_zero?: boolean;
  compute?: string;
  /** Maps the column's string value to a badge style (used by type="text_badge").
   *  Falls back to badge_style when the value isn't in the map. */
  badge_style_by_value?: Record<string, string>;
}

export interface RowAction {
  id: string;
  icon: string;
  label: Translations;
  style?: string;
  /** Optional URL template — substitute {id} with row id. When set, the
   *  frontend POSTs/etc here on click instead of using the built-in
   *  default for action.id. */
  url_template?: string;
  /** HTTP verb to use with url_template. Default varies by action.id. */
  method?: string;
  /** Simple expression evaluated against the row payload to gate
   *  visibility. Supported syntax:
   *    "fieldname"   — visible when truthy
   *    "!fieldname"  — visible when falsy
   *  Empty/undefined means always visible. */
  visible_when?: string;
}

/** Evaluate a RowAction.visible_when expression against a row payload.
 *  Empty/undefined expressions return true. Returns true if the row
 *  matches the expression. */
export function evalVisibleWhen(expr: string | undefined, row: Record<string, unknown>): boolean {
  if (!expr) return true;
  const trimmed = expr.trim();
  if (trimmed.startsWith('!')) {
    return !row[trimmed.slice(1).trim()];
  }
  return Boolean(row[trimmed]);
}

/** Group is one section of a grouped list data response. */
export interface Group {
  id: string;
  label: Translations;
  subtitle?: Translations;
  badges?: Badge[];
  collapsible: boolean;
  collapsed: boolean;
  read_only?: boolean;
  actions?: RowAction[];
  items: Record<string, unknown>[];
}

export interface Badge {
  label: string;
  tone?: 'primary' | 'neutral' | 'warning';
}
