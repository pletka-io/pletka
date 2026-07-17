// Hand-maintained ontology types for the autocomplete + path-builder
// frontend. It was originally generated from retired repository interfaces,
// but the generated content was just `*Repository = any` stubs that served
// no purpose, while the file accumulated hand-written shapes
// (PathSuggestion, AutocompleteRequest/Response, ScopeMatch, etc.) that
// tygo regen kept clobbering.
//
// The tygo entry has been removed from tygo.yaml so `go tool tygo
// generate` will not touch this file. EDITS HERE ARE SAFE.
//
// Drift risk note: these types must stay in sync with the Go shapes in
// `pkg/weave/ontology/autocomplete/engine.go` (Suggestion, Request) and
// related handlers. A follow-up plan will move these shapes into a
// canonical Go package and re-add a tygo entry — at that point this
// file becomes pure generated output again.

//////////
// source: repository.go (legacy stubs kept for back-compat)

/**
 * NamespaceBindingRepository manages prefix-to-namespace mappings.
 */
export type NamespaceBindingRepository = any;
/**
 * Repository interfaces for the ontology versioning system
 */
export interface Repositories {
  Families: OntologyFamilyRepository;
  Ontologies: OntologyRepository;
  OntologyVersions: OntologyVersionRepository;
  OntologyClasses: OntologyClassRepository;
  OntologyProperties: OntologyPropertyRepository;
  ProjectOntologies: ProjectOntologyVersionRepository;
  NamespaceBindings: NamespaceBindingRepository;
}
/**
 * OntologyFamilyRepository handles ontology family management
 */
export type OntologyFamilyRepository = any;
/**
 * OntologyRepository handles core ontology management
 */
export type OntologyRepository = any;
/**
 * OntologyVersionRepository handles ontology version management
 */
export type OntologyVersionRepository = any;
/**
 * OntologyClassRepository handles class management within versions
 */
export type OntologyClassRepository = any;
/**
 * OntologyPropertyRepository handles property management within versions
 */
export type OntologyPropertyRepository = any;
/**
 * ProjectOntologyVersionRepository handles project-ontology version associations
 */
export type ProjectOntologyVersionRepository = any;
/**
 * AutocompleteRequest represents a request for autocomplete suggestions
 */
export interface AutocompleteRequest {
  project_id: string;
  version_id?: string; // Optional: specific version
  current_path: string[]; // Path built so far
  query: string; // Current search query
  max_results: number /* int */; // Limit results
  include_inverse: boolean; // Include inverse properties
  include_parent_projects: boolean; // Include parent project ontologies
  include_range_suggestions: boolean; // Include class suggestions from property range (default false)
  allow_manual_complete: boolean; // Include "mark as complete" option in suggestions (edit mode only)
  scope_class?: string; // Optional: filter properties by this class as domain
  scope_additional_classes?: string[]; // Optional: union property suggestions across these extra scope classes (multi-class entities)
}
/**
 * AutocompleteResponse is the canonical autocomplete API envelope.
 */
export interface AutocompleteResponse {
  suggestions: PathSuggestion[];
}
/**
 * PathSuggestion represents an autocomplete suggestion
 */
export interface PathSuggestion {
  uri: string;
  local_name: string;
  prefix: string;
  label: { [key: string]: string};
  comment: { [key: string]: string};
  type: string; // "class", "object_property", "datatype_property", "literal"
  domain_classes?: string[];
  range_classes?: string[];
  /**
   * For type === "literal": the canonical xsd:* / rdfs:Literal qname the
   * resulting PathElement should persist as `datatype`. Set by the engine
   * when it synthesises a literal suggestion for a DatatypeProperty.
   */
  datatype?: string;
  /**
   * Rich metadata for hover information
   */
  ontology_info?: OntologyMetadata;
  is_inverse: boolean;
  /**
   * Project inheritance metadata
   */
  source_project?: ProjectInfo;
  inheritance_level: number /* int */; // 0=direct, 1=parent, 2=grandparent, etc.
  is_inherited: boolean;
  /**
   * Class hierarchy inheritance (for scope-based suggestions)
   */
  inherited_from_class?: string; // LocalName of the superclass (e.g., "E4_Period")
  inherited_from_class_label?: string; // Label of the superclass
  class_inheritance_depth?: number /* int */; // 0=direct, 1=parent class, 2=grandparent, etc.
  /**
   * Class hierarchy (for class suggestions) - shows parent classes
   */
  super_classes?: SuperClassInfo[];
  /**
   * Subclasses (for class suggestions) - shows classes that inherit from this class
   */
  sub_classes?: SubClassInfo[];
  /**
   * Ranking information
   */
  relevance: number /* float64 */;
}
/**
 * SuperClassInfo represents a superclass in the hierarchy
 */
export interface SuperClassInfo {
  uri: string;
  local_name: string;
  label?: string;
}
/**
 * SubClassInfo represents a subclass in the hierarchy
 */
export interface SubClassInfo {
  uri: string;
  local_name: string;
  label?: string;
}
/**
 * OntologyMetadata provides context information for suggestions
 */
export interface OntologyMetadata {
  ontology_name: string;
  ontology_prefix: string;
  version_string: string;
  namespace: string;
}
/**
 * ProjectInfo provides project context for inherited suggestions
 */
export interface ProjectInfo {
  project_id: string;
  project_name: string;
  is_parent: boolean;
}
/**
 * OntologyLabelsResult contains labels for classes and properties
 */
export interface OntologyLabelsResult {
  class_labels: { [key: string]: string}; // local_name -> label
  property_labels: { [key: string]: string}; // local_name -> label
  prefixes: { [key: string]: string}; // short_code -> namespace prefix (e.g. "E33" -> "crm")
  full_names: { [key: string]: string}; // short_code -> full local name (e.g. "P1" -> "P1_is_identified_by")
}
/**
 * AutocompleteService provides the core autocomplete functionality
 */
export type AutocompleteService = any;
/**
 * PathValidationResult represents the result of path validation
 */
export interface PathValidationResult {
  is_valid: boolean;
  error_message?: string;
  error_step?: number /* int */; // Which step in the path failed
  suggestions?: string[]; // Alternative suggestions if invalid
}
/**
 * LegacyPathConversionResult represents the result of converting a legacy path
 */
export interface LegacyPathConversionResult {
  original_path: string;
  cleaned_path: string;
  converted_path: PathSuggestion[];
  success: boolean;
  message: string;
  unresolved_items?: string[]; // Elements that couldn't be found in ontology
}
/**
 * ProjectValidationResult represents the result of validating all ontology paths in a project
 */
export interface ProjectValidationResult {
  project_id: string;
  project_name: string;
  total_fields: number /* int */;
  fields_with_paths: number /* int */;
  valid_paths: number /* int */;
  invalid_paths: number /* int */;
  warning_paths: number /* int */; // Paths with suggested fixes
  empty_paths: number /* int */;
  validation_errors?: FieldValidationError[];
  summary: string;
}
/**
 * FieldValidationError represents a validation error for a specific field
 */
export interface FieldValidationError {
  field_id: string;
  field_identifier: string;
  field_name: string;
  ontology_scope?: string; // Base class context (e.g., "E7 Activity")
  ontology_scope_id?: string; // Short form (e.g., "E7")
  ontology_path: string;
  error_message: string;
  error_step?: number /* int */;
  suggestions?: string[];
  unresolved_items?: string[];
  suggested_fixes?: PathElementFix[]; // Suggested corrections for fuzzy matches
  severity: ValidationSeverity; // error, warning, info
}
/**
 * PathElementFix represents a suggested fix for a path element
 */
export interface PathElementFix {
  original: string; // What was in the path
  suggested: string; // What it should be
  match_type: string; // exact, prefix_stripped, normalized, prefix_match, fuzzy
  relevance: number /* float64 */; // Match confidence (0.0-1.0)
}
/**
 * ValidationSeverity indicates the severity of a validation issue
 */
export type ValidationSeverity = string;
export const SeverityError: ValidationSeverity = "error"; // Cannot be resolved
export const SeverityWarning: ValidationSeverity = "warning"; // Can be resolved with suggested fix
export const SeverityInfo: ValidationSeverity = "info"; // Informational (e.g., fuzzy match used)
/**
 * FieldValidationSuccess represents a successfully validated field (for reporting)
 */
export interface FieldValidationSuccess {
  field_id: string;
  field_identifier: string;
  field_name: string;
  ontology_scope?: string;
  ontology_scope_id?: string;
  ontology_path: string;
  resolved_elements: number /* int */;
}
