import type { LanguageInfo, Translations } from './form-schema';

export interface OntologyPageSchema {
  kind: string;
  title: Translations;
  subtitle?: Translations;
  breadcrumbs?: OntologyPageLink[];
  sections: OntologyPageSection[];
  actions?: OntologyPageAction[];
  import?: OntologyImportConfig;
  ui: {
    languages: LanguageInfo[];
    primary_language: string;
  };
}

export interface OntologyImportConfig {
  probe_url: string;
  commit_url: string;
  max_bytes?: number;
}

export interface OntologyPageSection {
  id: string;
  widget: 'hero' | 'stats-strip' | 'metadata-list' | 'card-list' | 'tabs' | 'entity-list-link' | 'entity-list' | 'callout';
  title?: Translations;
  description?: Translations;
  tone?: string;
  stats?: OntologyPageStat[];
  metadata?: OntologyPageMetadata[];
  cards?: OntologyPageCard[];
  tabs?: OntologyPageTab[];
  links?: OntologyPageLink[];
  schema_url?: string;
  empty_text?: Translations;
}

export interface OntologyPageStat {
  label: Translations;
  value: string;
  help?: Translations;
  tone?: string;
}

export interface OntologyPageMetadata {
  label: Translations;
  value?: string;
  href?: string;
  code?: boolean;
}

export interface OntologyPageCard {
  id?: string;
  title: Translations;
  subtitle?: Translations;
  description?: Translations;
  href?: string;
  badges?: OntologyPageBadge[];
  stats?: OntologyPageStat[];
  metadata?: OntologyPageMetadata[];
  actions?: OntologyPageAction[];
}

export interface OntologyPageBadge {
  label: Translations;
  tone?: string;
}

export interface OntologyPageTab {
  id: string;
  label: Translations;
  href: string;
  active?: boolean;
  count?: number;
}

export interface OntologyPageLink {
  label: Translations;
  href?: string;
  icon?: string;
}

export interface OntologyPageAction {
  id: string;
  label: Translations;
  href?: string;
  method?: string;
  url?: string;
  form_schema_url?: string;
  success_href?: string;
  confirm?: Translations;
  style?: string;
  disabled?: boolean;
}
