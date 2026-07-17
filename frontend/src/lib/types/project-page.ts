// frontend/src/lib/types/project-page.ts

import type { ActorRef, OriginInfo, Translations } from './weave-types';

/** Matches Go formschema.ProjectPageSchema */
export interface ProjectPageSchema {
  entity: ProjectPageEntity;
  tabs: ProjectPageTab[];
  nav_links?: ProjectPageNavLink[];
  warnings?: ProjectPageWarning[];
  release?: ProjectReleaseView;
  ui: {
    languages: LanguageInfo[];
    primary_language: string;
  };
}

/** Matches Go formschema.SettingsWarning (reused on the project page). */
export interface ProjectPageWarning {
  id: string;
  severity: 'info' | 'warning' | 'error';
  message: Translations;
  action_href?: string;
  action_label?: Translations;
}

export interface ProjectPageEntity {
  id: string;
  name: Translations;
  description?: Translations;
  /** Derived project publication state: draft | published | modified. */
  status: string;
  /** Count of live entities new/modified since the latest release. */
  unreleased_changes?: number;
}

export interface ProjectPageTab {
  id: string;
  label: Translations;
  icon: string;
  /** Schema endpoint for leaf tabs that render inside the page. */
  content_url?: string;
  /** Full-page navigation target — used by right-aligned meta tabs (Settings, Exports). */
  href?: string;
  count?: number;
  /** Per-child counts for parent tabs, e.g. [12, 45, 617, 0]. */
  breakdown?: number[];
  default?: boolean;
  disabled?: boolean;
  tooltip?: string;
  /** "" or "left" → left side; "right" → muted meta nav slot. */
  align?: 'left' | 'right';
  /** "" or "primary" → standard chip; "muted" → smaller, gray. */
  variant?: 'primary' | 'muted';
  /** Sub-tabs rendered as a second row when this or any child is active. */
  children?: ProjectPageTab[];
}

export interface ProjectPageNavLink {
  label: Translations;
  href: string;
  icon: string;
}

export interface ProjectReleaseView {
  version: string;
  draft_url: string;
  label: Translations;
}

export interface ProjectReleaseTabSchema {
  kind: 'releases';
  project_id: string;
  current_version?: string;
  can_create: boolean;
  create_form_schema_url?: string;
  create_blocked_message?: Translations;
  dependency_settings_url?: string;
  draft_parent_dependencies?: ProjectReleaseParentDependency[];
  draft_url: string;
  items: ProjectReleaseTabItem[];
  empty_message?: Translations;
}

export interface ProjectReleaseParentDependency {
  parent_project_id: string;
  label: Translations;
  primary: boolean;
}

export interface ProjectReleaseTabItem {
  version: string;
  title?: string;
  description?: string;
  created_at: string;
  created_by_id?: string;
  view_url: string;
  active: boolean;
  label: Translations;
}

/** Adoptions tab schema — flat list of explicit project-level receipts.
 *  Per docs/plans/2026-06-07-adoption-ux-review.md: each item shows
 *  eager dependency counts; the frontend lazily fetches the per-kind
 *  list via closure_url_template when the curator clicks a chip.
 *  All user-visible strings live on `labels` (server-resolved
 *  LocalizedText) so the component renders via tr() — schema-driven
 *  UI rule. */
export interface ProjectAdoptionsTabSchema {
  kind: 'adoptions';
  project_id: string;
  current_version?: string;
  draft_url: string;
  items: ProjectAdoptionTabItem[];
  empty_message?: Translations;
  labels: ProjectAdoptionsLabels;
}

export interface ProjectAdoptionsLabels {
  title?: Translations;
  description?: Translations;
  from?: Translations;
  source_link?: Translations;
  adopted_prefix?: Translations;
  by_prefix?: Translations;
  models?: Translations;
  collections?: Translations;
  fields?: Translations;
  loading?: Translations;
  load_error_prefix?: Translations;
  section_empty?: Translations;
  source_column_label?: Translations;
}

export interface ProjectAdoptionTabItem {
  entity_type: string;
  source_project_id: string;
  source_entity_id: string;
  /** URL into the CURRENT project's view of the adopted entity. Primary link. */
  current_url?: string;
  /** URL into the SOURCE project's own page for the same entity. Secondary link. */
  source_url?: string;
  adopted_at: string;
  created_by?: ActorRef;
  origin: OriginInfo;
  label: Translations;
  /** Source entity's UIName so the card header reads
   *  "Person · LAM.9" instead of just "LAM.9". Optional —
   *  Svelte falls back to source_entity_id if absent. */
  name?: Translations;
  /** Transitive counts (full closure via override refs). */
  model_count: number;
  collection_count: number;
  field_count: number;
  /** Direct (depth-1) counts. Pair with their transitive
   *  siblings so the chip can show "5 / 11" — direct over
   *  transitive — letting curators tell receipts apart even
   *  when their closures fully overlap. */
  model_count_direct: number;
  collection_count_direct: number;
  field_count_direct: number;
  /** "/projects/{id}/adoptions/{srcProj}/{srcEnt}/closure?kind={kind}".
   *  Frontend substitutes {kind} = models|collections|fields. */
  closure_url_template?: string;
}

/** Single section of a receipt's bill-of-materials closure. */
export interface ProjectAdoptionClosureResponse {
  kind: 'models' | 'collections' | 'fields';
  count: number;
  items: ProjectAdoptionClosureItem[];
}

export interface ProjectAdoptionClosureItem {
  id: string;
  semantic_id?: string;
  name?: Translations;
  source_project_id: string;
  current_url?: string;
  source_url?: string;
}

/** Matches Go formschema.ProjectOverviewSchema */
export interface ProjectOverviewSchema {
  sections: ProjectOverviewSection[];
  ui: {
    languages: LanguageInfo[];
    primary_language: string;
  };
}

export interface ProjectOverviewSection {
  widget: string; // "readme" | "description" | "stats-cards" | "info-sidebar" | "ontologies" | "credits"
  title?: Translations;
  content?: string;
  content_i18n?: Translations;
  items?: ProjectOverviewItem[];
  credits?: CreditsGroup[];
}

export interface CreditsGroup {
  kind: string;
  label: Translations;
  items: CreditsItem[];
}

export interface CreditsItem {
  actor_id: string;
  actor_name: string;
  actor_type?: string;
  note?: string;
}

export interface ProjectOverviewItem {
  key?: string;
  label: Translations;
  value?: string;
  count?: number;
  icon?: string;
  color?: string;
  style?: string;
  type?: string;
  /** Optional sub-line rendered below the main label/value (ontologies widget). */
  subline?: string;
  /** Per-kind usage counters (ontologies widget — used vs total). */
  classes_used?: number;
  classes_total?: number;
  properties_used?: number;
  properties_total?: number;
  /** When set, info-sidebar renders the item as an anchor. */
  url?: string;
  origin: OriginInfo;
}

interface LanguageInfo {
  code: string;
  label: string;
}
