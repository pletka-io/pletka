/** API error with status code and message */
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

/** Typed fetch wrapper */
async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
    ...init,
  });

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new ApiError(res.status, text);
  }

  return res.json();
}

/** Ontology autocomplete suggestion */
export interface AutocompleteSuggestion {
  uri: string;
  label: string;
  type: string;
  description?: string;
}

/** Ontology label map */
export type OntologyLabels = Record<string, string>;

/** Full ontology labels data including prefixes and full names */
export interface OntologyLabelsData {
  labels: OntologyLabels;
  prefixes: Record<string, string>;  // short_code -> namespace prefix (e.g. "P1" -> "crm")
  fullNames: Record<string, string>; // short_code -> full local name (e.g. "P1" -> "P1_is_identified_by")
}

// ─── Ontology API ──────────────────────────────────────

/** Ontology autocomplete */
export function ontologyAutocomplete(body: {
  project_id?: string;
  version_id?: string;
  current_path: string[];
  query: string;
  scope_class?: string;
  include_inverse?: boolean;
  include_parent_projects?: boolean;
}): Promise<AutocompleteSuggestion[]> {
  return apiFetch('/api/v1/ontology/autocomplete', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

/** Raw ontology labels response from the API */
interface OntologyLabelsResponse {
  success: boolean;
  project_id: string;
  class_labels: Record<string, string>;
  property_labels: Record<string, string>;
  prefixes?: Record<string, string>;
  full_names?: Record<string, string>;
}

/** Get ontology labels for a project, merged into a flat map */
export async function getOntologyLabels(
  projectId: string,
): Promise<OntologyLabels> {
  const resp = await apiFetch<OntologyLabelsResponse>(
    `/api/projects/${projectId}/ontology-labels`,
  );
  return { ...resp.class_labels, ...resp.property_labels };
}

/** Get full ontology labels data including prefixes and full names */
export async function getOntologyLabelsData(
  projectId: string,
): Promise<OntologyLabelsData> {
  const resp = await apiFetch<OntologyLabelsResponse>(
    `/api/projects/${projectId}/ontology-labels`,
  );
  return {
    labels: { ...resp.class_labels, ...resp.property_labels },
    prefixes: resp.prefixes ?? {},
    fullNames: resp.full_names ?? {},
  };
}

/** Diagram response from the API */
export interface DiagramResponse {
  success: boolean;
  mermaid: string;
  name?: string;
  id?: string;
  count?: number;
  error?: string;
}

async function fetchText(url: string, label: string): Promise<string> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new ApiError(res.status, `Failed to get ${label}: ${res.statusText}`);
  }
  return res.text();
}

/** Get model diagram data (CRITERIA-style ontology Mermaid) */
export function getModelDiagram(
  modelId: string,
  mode = 'ontology',
): Promise<DiagramResponse> {
  return apiFetch(`/gen/models/${modelId}/diagram?mode=${encodeURIComponent(mode)}`);
}

/** Get model as Turtle RDF */
export function getModelTurtle(modelId: string): Promise<string> {
  return fetchText(`/gen/models/${modelId}/turtle`, 'Turtle');
}

/** Get model as JSON-LD */
export function getModelJsonLd(modelId: string): Promise<string> {
  return fetchText(`/gen/models/${modelId}/jsonld`, 'JSON-LD');
}

/** Get model SHACL shapes (serialised as Turtle) */
export function getModelShacl(modelId: string): Promise<string> {
  return fetchText(`/gen/models/${modelId}/shacl`, 'SHACL');
}

/** Get model export graph as JSON */
export function getModelExportGraph(modelId: string): Promise<unknown> {
  return apiFetch(`/gen/models/${modelId}/exportgraph`);
}

/** Get collection diagram as Mermaid */
export function getCollectionDiagram(
  collectionId: string,
  mode = 'ontology',
): Promise<DiagramResponse> {
  return apiFetch(`/gen/collections/${collectionId}/diagram?mode=${encodeURIComponent(mode)}`);
}

/** Get collection as Turtle RDF */
export function getCollectionTurtle(collectionId: string): Promise<string> {
  return fetchText(`/gen/collections/${collectionId}/turtle`, 'Turtle');
}

/** Get collection as JSON-LD */
export function getCollectionJsonLd(collectionId: string): Promise<string> {
  return fetchText(`/gen/collections/${collectionId}/jsonld`, 'JSON-LD');
}

/** Get collection SHACL shapes (serialised as Turtle) */
export function getCollectionShacl(collectionId: string): Promise<string> {
  return fetchText(`/gen/collections/${collectionId}/shacl`, 'SHACL');
}

/** Get collection export graph as JSON */
export function getCollectionExportGraph(collectionId: string): Promise<unknown> {
  return apiFetch(`/gen/collections/${collectionId}/exportgraph`);
}

// ─── Schema-driven derivative fetchers ──────────────────────────────────
//
// The detailview response carries a `capabilities.derivatives` block
// with full URLs for diagram / turtle / jsonld / shacl / exportgraph.
// Frontend reads those URLs and passes them to these generic helpers,
// matching .claude/rules/api-patterns.md (schema is the complete
// contract; frontend never constructs API URLs).
//
// The entity-specific fetchers above (getModelDiagram etc.) predate
// this rule and stay around as a temporary back-compat shim while
// callers migrate. New callers should use these helpers.

/** Fetch a JSON-shaped derivative from a server-provided URL. */
export function getDerivativeJSON<T = unknown>(url: string): Promise<T> {
  return apiFetch<T>(url);
}

/** Fetch a text-shaped derivative (turtle, jsonld text, shacl) from a
 * server-provided URL. */
export function getDerivativeText(url: string, label = 'derivative'): Promise<string> {
  return fetchText(url, label);
}

/** Fetch a diagram response from a server-provided URL. The mode is
 * a server-emitted query param so the URL is fully ready-to-use; if
 * the schema didn't bake mode in, this helper appends it. */
export function getDerivativeDiagram(url: string, mode?: string): Promise<DiagramResponse> {
  if (mode && !url.includes('mode=')) {
    const sep = url.includes('?') ? '&' : '?';
    url = `${url}${sep}mode=${encodeURIComponent(mode)}`;
  }
  return apiFetch<DiagramResponse>(url);
}
