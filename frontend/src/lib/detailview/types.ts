// frontend/src/lib/detailview/types.ts
//
// This file defines the current frontend contract for the detailview platform.
// The filename and API route still use the legacy compatibility name,
// "entity-view", but conceptually this belongs to detailview.

import type { ActorRef, Translations, PathElement, EntityRef, OriginInfo } from '$lib/types/weave-types';
import type { ActionGroupSchema } from '$lib/types/form-schema';

/** Matches Go pkg/weave/detailview.EntityViewResponse */
export interface EntityViewResponse {
  entity: EntityViewMeta;
  capabilities: ViewCapabilities;
  view_mode: string;
  sections: ViewSection[];
  entries?: ConceptListEntry[];
  adoptions?: AdoptionItem[];
  refs: ViewRefs;
  stats_url: string;
  release?: DetailReleaseView;
}

export interface DetailReleaseView {
  version: string;
  /** Present only when the viewer can edit the project (ProjectEdit). */
  draft_url?: string;
  label: Translations;
}

export interface EntityViewMeta {
  id: string;
  type: string; // "model" | "collection"
  name: Translations;
  description?: Translations;
  system_name: string;
  status: string;
  /** Live entity's state vs the latest release: draft | published | modified |
   *  new. Derived on read; drives the header badge instead of raw status.
   *  Empty in release-view mode. */
  publication_state?: string;
  /** Orthogonal to status — a draft or published entity can be deprecated. */
  deprecated?: boolean;
  origin: OriginInfo;
  ontology_scope?: ViewScopeInfo;
  created_at: string;
  updated_at: string;
  model_type?: string;
  list_type?: string;
  list_type_uri?: string;
  parent_term_uri?: string;
  list_type_label?: string;
  parent_term?: VocabularyEntryRef;
  vocabulary_id?: string;
  vocabulary_label?: string;
  source_vocabulary?: VocabularyRef;
  entry_count?: number;
  ontology_path?: string;
  path_elements?: PathElement[];
  expected_value_type?: string;
  set_value?: string;
  category_id?: string;
}

export interface VocabularyRef {
  id: string;
  semantic_id?: string;
  system_name?: string;
  name?: Translations;
  base_uri?: string;
  url?: string;
}

export interface VocabularyEntryRef {
  id?: string;
  vocabulary_id?: string;
  uri?: string;
  label?: Translations;
  scope_note?: Translations;
  broader_uri?: string;
  broader_path?: VocabularyEntryRef[];
  external_id?: string;
}

export interface ViewScopeInfo {
  local_name: string;
  prefix: string;
}

export interface ViewCapabilities {
  editable: boolean;
  can_add_fields: boolean;
  can_reorder: boolean;
  save_url?: string;
  metadata_url?: string;
  adopt_url?: string;
  fork_url?: string;
  search_fields_url?: string;
  examples_schema_url?: string;
  concept_list?: ConceptListCap;
  derivatives?: DerivativesCap;
  /** Reuse-tab endpoint URL — emitted only for field views today.
   *  Frontend renders the Reuse tab when this URL is present. */
  reuse_url?: string;
  /** Lifecycle actions for the "More actions" kebab, emitted only for an
   *  editable owned entity on the current version. Exactly one of
   *  deprecate_url / activate_url is present per current status; delete_url
   *  is destructive (confirm + redirect to the project page). */
  delete_url?: string;
  deprecate_url?: string;
  activate_url?: string;
}

export interface ConceptListCap {
  search_entries_url: string;
  add_entry_url?: string;
  update_entry_url?: string;
  remove_entry_url?: string;
  reorder_url?: string;
  /** Sealed list: membership locked; disable add + show badge (#3599). */
  is_closed?: boolean;
  /** POST {label, scope_note} to author a local term into this list. */
  create_term_url?: string;
  /** PATCH {is_closed} to lock/unlock membership. */
  seal_url?: string;
  /** Per-term hierarchy endpoint; {conceptID} = vocabulary entry id. GET/POST, DELETE at .../{edgeID}. */
  broader_url_template?: string;
}

/** One relationship's usages: same-project models/collections plus
 *  cross-project groups. */
export interface ReuseSection {
  models: FieldUsageRef[];
  collections: FieldUsageRef[];
  /** Cross-project usages grouped by consuming project. */
  other_projects?: ProjectUsageGroup[];
}

/** Payload of GET capabilities.reuse_url. Splits usages by relationship:
 *  included_in = membership (this entity is a member of / bundled into the
 *  listed entities); referenced_by = value-target (a field there points at
 *  this as its expected value type). A section is absent when the entity type
 *  cannot have that relationship (field: included_in only; model:
 *  referenced_by only; collection: both). */
export interface FieldReuseResponse {
  included_in?: ReuseSection;
  referenced_by?: ReuseSection;
}

export interface ConceptListEntry {
  id: string;
  vocabulary_entry_id: string;
  uri?: string;
  label?: Translations;
  scope_note?: Translations;
  external_id?: string;
  broader_uri?: string;
  broader_path?: string[];
  broader_path_items?: VocabularyEntryRef[];
  custom_label?: Translations;
  position: number;
}

export interface ConceptListSearchResult {
  id?: string;
  concept_list_id?: string;
  vocabulary_entry_id?: string;
  position?: number;
  custom_label?: Translations;
  entry: {
    id?: string;
    vocabulary_id: string;
    uri: string;
    label?: Translations;
    scope_note?: Translations;
    broader_uri?: string;
    broader_path?: string[];
    external_id?: string;
    broader_path_items?: VocabularyEntryRef[];
    hydrated?: boolean;
  };
}

export interface FieldUsageRef {
  id: string;
  semantic_id?: string;
  system_name?: string;
  name: Translations;
  description?: Translations;
  url?: string;
  /** Consuming project — equal to the field's project for same-project usage. */
  project_id?: string;
  project_name?: string;
}

/** One consuming project's usages, for the "Other projects" sub-tab. */
export interface ProjectUsageGroup {
  project_id: string;
  project_name?: string;
  models?: FieldUsageRef[];
  collections?: FieldUsageRef[];
}

/** Server-emitted URLs for the entity's derivatives (diagram + RDF
 *  formats + query / mapping outputs). DiagramTab reads from these
 *  instead of constructing the URLs itself — see
 *  .claude/rules/api-patterns.md. */
export interface DerivativesCap {
  /** Optional: omitted when no renderer for the format is registered
   *  in this build (e.g. core-only builds lack shacl/arches/cytoscape). */
  diagram_url?: string;
  turtle_url?: string;
  jsonld_url?: string;
  shacl_url?: string;
  /** SPARQL SELECT (or COUNT) query against the entity. Honours
   *  ?count=1 and ?limit=N query params for variants. */
  sparql_url?: string;
  /** X3ML form A — single mapping with full-path links. */
  x3ml_a_url?: string;
  /** X3ML form B — primary spine mapping plus per-spine-class secondary mappings. */
  x3ml_b_url?: string;
  /** ZIP: X3ML form A plus each linked ontology's raw RDFS schema file. */
  x3ml_a_zip_url?: string;
  /** ZIP: X3ML form B plus each linked ontology's raw RDFS schema file. */
  x3ml_b_zip_url?: string;
  /** ResearchSpace YAML config. Server emits only on model entities. */
  researchspace_url?: string;
  /** Arches Resource Graph JSON. Models only, super-admin only. */
  arches_url?: string;
  /** Renderer-neutral Snapshot JSON (tree + path_node ids). Models only, super-admin only. */
  snapshot_url?: string;
  /** Human-readable ASCII tree of the Snapshot keyed by PathNodeID/InstanceID.
   *  Models only, super-admin only — curator review surface. */
  ascii_tree_url?: string;
  exportgraph_url?: string;
  /** Cytoscape graph JSON; parallel to mermaid diagram URL. */
  cytoscape_url?: string;
  /** Per-entity CSV download URL. Member-only — empty for non-members
   *  so DiagramTab hides the CSV sub-tab. Verifies the entity against
   *  the project verification dump. */
  csv_url?: string;
  /** Per-project integration action groups (e.g. "Upload to 3M").
   *  Each group bundles one action with a list of dispatchable targets
   *  (one per enabled config). DiagramTab renders via
   *  ActionGroup.svelte — falls back to a plain button when single-target. */
  integration_actions?: ActionGroupSchema[];
}

export interface ViewSection {
  widget: string; // "category-group"
  id: string;
  name: Translations;
  canonical_order: number;
  items: ViewItem[];
}

export interface ViewItem {
  widget: string; // "collection-group" | "field-group"
  id: string;
  name?: Translations;
  ontology_scope?: string;
  field_count: number;
  fields: ViewField[];
  shared_path_prefix?: PathElement[];
  /** Standalone detail URL of the underlying collection. Empty for the
   *  synthetic 'Direct Fields' group. Schema-driven
   *  click-through to the source collection. */
  source_url?: string;
  /** Collection-in-model constraints. Hidden groups never
   *  reach this read-only contract; absent = optional/0..unbounded. */
  placement?: {
    is_required: boolean;
    min_occurs: number;
    max_occurs?: number | null;
    is_hidden: boolean;
  };
}

export interface ViewField {
  widget: string; // "field-override"
  override_id: number;
  field_id: string;
  field_semantic_id: string;
  field_system_name: string;
  origin: OriginInfo;
  position: number;
  display_name: Translations;
  description?: Translations;
  expected_value_type?: string;
  set_value?: string;
  is_required: boolean;
  is_hidden: boolean;
  ontology_path?: string;
  path_elements?: PathElement[];
  resource_model_refs?: EntityRef[];
  collection_model_refs?: EntityRef[];
  category_id?: string;
  collection_id?: string;
  /** Standalone detail URL of the source field (e.g. /projects/LA/fields/LAF.10).
   *  Honours inheritance: when the field comes from an ancestor project
   *  the URL points to that project. */
  source_field_url?: string;
  /** Standalone detail URL of the wrapping collection, when collection_id
   *  is set. Drives the "from {collection}" link on field rows. */
  source_collection_url?: string;
}

export interface ViewRefs {
  categories?: Record<string, ViewRefEntry>;
  collections?: Record<string, ViewRefEntry>;
  models?: Record<string, ViewRefEntry>;
}

export interface ViewRefEntry {
  name: Translations;
  scope?: string;
  order?: number;
  origin?: OriginInfo;
  url?: string;
}

export interface AdoptionItem {
  entity_type: string;
  source_project_id: string;
  source_entity_id: string;
  source_url?: string;
  adopted_at: string;
  created_by?: ActorRef;
  origin: OriginInfo;
  label: Translations;
}

export interface StatItem {
  id: string;
  name: string;
  count: number;
  percentage: number;
}

export interface ModelViewStats {
  total_fields: number;
  total_categories: number;
  total_collections: number;
  overridden_fields: number;
  required_fields: number;
  optional_fields: number;
  models_using?: number;
  collections_using?: number;
  field_override_count?: number;
  value_type_counts: Record<string, number>;
  scopes_count: number;
  categories_breakdown: StatItem[];
  fields_breakdown: StatItem[];
  field_scopes_breakdown: StatItem[];
  coll_scopes_breakdown: StatItem[];
  classes_breakdown: StatItem[];
  properties_breakdown: StatItem[];
}
