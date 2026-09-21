import type { LanguageInfo, SchemaEndpoint, SchemaUI, Translations } from './form-schema';

export interface ExampleConceptSource {
  id: string;
  semantic_id?: string;
  name?: Translations;
  url?: string;
  search_url: string;
}

export interface ExampleIssue {
  severity: 'error' | 'warning';
  code: string;
  field_id?: string;
  override_id?: number;
  occurrence_index?: number;
  message?: Translations;
}

export interface ExampleOccurrence {
  occurrence_index: number;
  value?: {
    kind?: string;
    string_value?: string;
    number_value?: number;
    bool_value?: boolean;
    date_value?: string;
    uri_value?: string;
    concept_uri?: string;
    concept_label?: string;
    example_id?: string;
    target_entity_id?: string;
    target_label?: string;
  };
  issues?: ExampleIssue[];
}

export interface ExampleFormField {
  override_id: number;
  field_id: string;
  field_semantic_id: string;
  label: Translations;
  help?: Translations;
  widget: string;
  value_kind: 'string' | 'integer' | 'date' | 'uri' | 'concept' | 'example_ref';
  expected_value_type?: string;
  required?: boolean;
  repeatable?: boolean;
  min_occurs?: number;
  max_occurs?: number | null;
  hidden?: boolean;
  set_value?: string;
  resource_models?: Array<{ id: string; semantic_id?: string; name?: Translations }>;
  collection_models?: Array<{ id: string; semantic_id?: string; name?: Translations }>;
  concept_lists?: Array<{ id: string; semantic_id?: string; name?: Translations; url?: string }>;
  concept_sources?: ExampleConceptSource[];
  issues?: ExampleIssue[];
  occurrences?: ExampleOccurrence[];
}

export interface ExampleFormGroup {
  id: string;
  label: Translations;
  position?: number;
  shared_path_prefix?: Array<{ predicate?: string; class?: string }>;
  fields: ExampleFormField[];
}

export interface ExampleFormSection {
  id: string;
  label: Translations;
  canonical_order?: number;
  groups?: ExampleFormGroup[];
  direct_fields?: ExampleFormField[];
}

export interface ExampleFormSchema {
  kind: 'example-form';
  example_id?: string;
  target: {
    entity_type: string;
    entity_id: string;
    name?: Translations;
  };
  endpoint?: SchemaEndpoint;
  sections: ExampleFormSection[];
  issues?: ExampleIssue[];
  ui: SchemaUI & { languages: LanguageInfo[] };
}
