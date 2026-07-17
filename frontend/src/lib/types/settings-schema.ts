import type { Translations } from './form-schema';

export interface SettingsSchema {
  project_id: string;
  project_name: Translations;
  sections: SettingsSection[];
  warnings?: SettingsWarning[];
  release?: SettingsReleaseView;
}

export interface SettingsReleaseView {
  version: string;
  draft_url: string;
  label: Translations;
}

export interface SettingsSection {
  id: string;
  label: Translations;
  icon: string;
  kind?: 'form' | 'list' | 'composite';
  schema_url?: string;
  href?: string;
  /** Section is a stub — pane shows a "coming soon" message instead of content. */
  placeholder?: boolean;
  /** Required setup is missing — render a red dot in the sidebar. */
  needs_attention?: boolean;
}

export interface SettingsWarning {
  id: string;
  severity: 'info' | 'warning' | 'error';
  message: Translations;
  action_href?: string;
  action_label?: Translations;
}
