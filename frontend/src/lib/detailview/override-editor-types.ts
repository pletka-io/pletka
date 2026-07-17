import type { PathElement, Translations, OriginInfo } from '$lib/types/weave-types';

export interface OverrideEditorResponse {
  entity_type: 'model' | 'collection';
  entity_id: string;
  project_id: string;
  version: number;
  scope: OverrideEditorScope;
  categories: OverrideEditorCategory[];
  /** Project's full category roster for dropdowns (used + unused). */
  available_categories: OverrideEditorCategoryRef[];
  available: OverrideEditorAvailable;
  capabilities: OverrideEditorCapabilities;
}

export interface OverrideEditorCategoryRef {
  id: string;
  semantic_id?: string;
  name?: Translations;
  status?: string;
  canonical_order?: number;
}

export interface OverrideEditorScope {
  prefix?: string;
  local_name: string;
  display: string;
}

export interface OverrideEditorAvailable {
  search_url: string;
  path_suggestions_url: string;
  adopt_collection_url?: string;
  field_sidebar_schema_url?: string;
  collection_group_sidebar_schema_url?: string;
}

export interface OverrideEditorCapabilities {
  can_add_field: boolean;
  can_add_collection: boolean;
  can_reorder: boolean;
  can_edit_overrides: boolean;
  can_hide_fields: boolean;
}

export interface OverrideEditorCategory {
  category_id: string;
  semantic_id?: string;
  category_name: Translations;
  position: number;
  items: OverrideEditorItem[];
}

// CollectionPlacement mirrors domain.CollectionPlacement:
// model-level constraints of one collection group. Absent = optional,
// 0..unbounded, visible.
export interface CollectionPlacement {
  id?: number;
  project_id?: string;
  model_id?: string;
  category_id?: string;
  collection_id?: string;
  is_required: boolean;
  min_occurs: number;
  max_occurs?: number | null;
  is_hidden: boolean;
}

export interface OverrideEditorItem {
  widget: 'field-group' | 'collection-group';
  id: string;
  semantic_id?: string;
  name?: Translations;
  position: number;
  field_count: number;
  shared_path_prefix?: PathElement[];
  placement?: CollectionPlacement;
  fields: OverrideEditorField[];
}

export interface OverrideRef {
  id: string;
  semantic_id?: string;
  name?: Translations;
  url?: string;
}

export interface OverrideEditorField {
  field_id: string;
  override_id: number;
  position: number;
  display_name: Translations;
  description?: Translations;
  ontology_path?: string;
  path_elements?: PathElement[];
  category_id?: string;
  part_of_collection_id?: string;
  expected_value_type?: string;
  expected_resource_models?: string[];
  expected_collection_models?: string[];
  expected_concept_lists?: string[];
  /** Resolved refs for display chips ({id, semantic_id, name}). Read-only mirror of the *_models arrays. */
  expected_resource_model_refs?: OverrideRef[];
  expected_collection_model_refs?: OverrideRef[];
  expected_concept_list_refs?: OverrideRef[];
  set_value?: string;
  is_required: boolean;
  min_occurs: number;
  max_occurs?: number | null;
  is_hidden: boolean;
  visibility?: string;
}

export interface OverrideSearchResult {
  id: string;
  semantic_id: string;
  system_name: string;
  ui_name: Translations;
  description?: Translations;
  ontology_scope?: PathElement;
  ontology_path?: string;
  path_elements?: PathElement[];
  expected_value_type?: string;
  expected_resource_models?: string[];
  expected_collection_models?: string[];
  expected_concept_lists?: string[];
  expected_resource_model_refs?: OverrideRef[];
  expected_collection_model_refs?: OverrideRef[];
  expected_concept_list_refs?: OverrideRef[];
  project_id: string;
  origin: OriginInfo;
  is_linked: boolean;
  adoption_count: number;
  url?: string;
}

export interface OverrideSearchResponse {
  items: OverrideSearchResult[];
  total_count: number;
}

export interface OverridePathSuggestion {
  local_name: string;
  prefix: string;
  type: string;
  field_count: number;
  display: string;
}

export interface OverridePathSuggestionsResponse {
  suggestions: OverridePathSuggestion[];
  current_path: string;
  depth: number;
}

export interface OverrideCategoryOption {
  id: string;
  semantic_id?: string;
  ui_name: Translations;
  canonical_order: number;
}

export interface AdoptCollectionResponse {
  category_id: string;
  item: OverrideEditorItem;
}
