import type { Translations } from './form-schema';

export interface AdminSchema {
  title: Translations;
  sections: AdminSection[];
  warnings?: AdminWarning[];
}

export interface AdminSection {
  id: string;
  label: Translations;
  icon: string;
  kind?: 'form' | 'list' | 'entity-list' | 'composite' | 'external' | 'git-restore';
  schema_url?: string;
  href?: string;
  placeholder?: boolean;
}

export interface AdminWarning {
  id: string;
  severity: 'info' | 'warning' | 'error';
  message: Translations;
  action_href?: string;
  action_label?: Translations;
}
