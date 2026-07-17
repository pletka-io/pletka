/**
 * Weave domain types — matches Go types in pkg/domain/*.go
 * These replace the legacy generated types for the model editor.
 */

/** Multilingual text map (language code → text) */
export type Translations = Record<string, string>;

export interface ActorRef {
  id?: string;
  label?: string;
}

export interface OriginInfo {
  kind: 'own' | 'inherited' | 'adopted' | 'adopted_reference' | 'forked';
  source_project_id?: string;
  source_project_label?: string;
  source_entity_id?: string;
}

/** Ontology path element */
export interface PathElement {
  type: string;
  uri: string;
  prefix: string;
  local_name: string;
  datatype?: string;
  position: number;
  class_code?: string;
  instance_id?: string;
  ontology_version_id?: string;
}

/** Legacy <br><br> subfield path — read-only, shares the parent field metadata. */
export interface SubfieldPath {
  path_elements: PathElement[];
  expected_value_type?: string;
  scope?: string;
  source?: string;
}

/** Weave project */
export interface WeaveProject {
  id: string; // IDPrefix: "LA"
  system_name?: string;
  ui_name?: Translations;
  description?: Translations;
  status: string;
  namespace?: string;
  parent_project_id?: string;
}

/** Weave model (identity + ontology scope) */
export interface WeaveModel {
  id: string; // semantic ID: "LAM.1"
  system_name?: string;
  ui_name?: Translations;
  description?: Translations;
  status: string;
  project_id: string;
  ontology_scope?: PathElement;
}

/** Weave collection (identity + ontology scope) */
export interface WeaveCollection {
  id: string; // semantic ID: "LAC.17"
  system_name?: string;
  ui_name?: Translations;
  description?: Translations;
  status: string;
  project_id: string;
  ontology_scope?: PathElement;
  collection_number?: number;
  canonical_collection_order?: number;
}

/** Resolved reference to a model or collection */
export interface EntityRef {
  id: string;
  semantic_id: string;
  name?: Translations;
  origin?: OriginInfo;
  url?: string;
}

/** A field with its winning override applied — ready for display */
export interface ResolvedField {
  id: string; // field semantic ID: "LAF.228"
  semantic_id: string;
  system_name: string;
  ontology_path?: string;
  path_elements?: PathElement[];

  display_name: Translations;
  description?: Translations;
  position: number;

  expected_value_type?: string;
  set_value?: string;
  resource_models?: EntityRef[];
  collection_models?: EntityRef[];

  is_required: boolean;
  is_hidden: boolean;
  is_external: boolean; // true when field is from a different project

  override_source: string; // "", "model", "collection"
  override_id: number;

  category_id?: string;
  part_of_collection_id?: string;
  collection_order: number;
  collection_name?: Translations;
}

/** Collection group within a category */
export interface CollectionGroup {
  id: string;
  name: Translations;
  position: number;
  fields: ResolvedField[];
  shared_path_prefix?: PathElement[];
}

/** Category group containing collections of fields */
export interface CategoryGroup {
  id: string;
  name: Translations;
  position: number;
  collections: CollectionGroup[];
}

/** Summary stats for a model view — all computed server-side */
export interface ModelViewStats {
  total_fields: number;
  total_categories: number;
  total_collections: number;
  overridden_fields: number;
  required_fields: number;
  optional_fields: number;
  value_type_counts: Record<string, number>;
}

/** Complete model view response from the API */
export interface ModelViewResponse {
  model: WeaveModel;
  categories: CategoryGroup[];
  stats: ModelViewStats;
}

/** Helper to get a translated value with fallback */
export function tr(translations: Translations | undefined, lang = 'en', fallback = ''): string {
  if (!translations) return fallback;
  return translations[lang] ?? translations['en'] ?? Object.values(translations)[0] ?? fallback;
}

/** @deprecated Use field.is_external from the API instead */
export function isExternalField(fieldId: string, projectId: string): boolean {
  return !fieldId.startsWith(projectId);
}
