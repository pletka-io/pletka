/**
 * Types for the rich ontology settings pane (PaneView).
 *
 * Shape mirrors pkg/weave/projectontologyversion.PaneView.
 */

export interface PaneView {
  groups: PaneGroup[];
  // Top-level mutation URLs. Absent for read-only callers; absence is
  // also the signal the frontend uses to hide "Add ontology" affordances.
  actions?: PaneActions;
}

export interface PaneActions {
  create_url?: string;
  form_schema_url?: string;
}

export interface PaneGroup {
  base_version_id: string;
  origin: import('./weave-types').OriginInfo;
  base: PaneOntology | null;
  extensions: PaneOntology[];
  available_extensions?: PaneAvailable[];
}

export interface PaneOntology {
  version_id: string;
  ontology_id: string;
  name: string;
  prefix: string;
  ontology_type: 'base' | 'extension';
  version_string: string;
  is_primary: boolean;
  usage_notes?: string;
  // Per-row mutation URLs. Empty/absent for inherited rows and for
  // read-only callers — frontend reads them to decide whether to
  // render the corresponding action button.
  update_url?: string;
  delete_url?: string;
}

export interface PaneAvailable {
  ontology_id: string;
  name: string;
  prefix: string;
  version_id: string;
  version_string: string;
  // POST endpoint that links this extension to the current project.
  // Empty for read-only callers.
  enable_url?: string;
}
