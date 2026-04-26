// Type declarations for the Pletka JSON schema as the renderer consumes it.
//
// These types are the renderer's mirror of what `server/widgets/` and
// `server/relations/` emit. They are intentionally thin and forward-compatible:
// unknown widget types fall through to a generic dispatcher, unknown link
// relations are simply not invoked, unknown capability fields are ignored.
// Adding fields on the server side never breaks an existing renderer.
//
// Pletka encodes "permission grant = presence" in two complementary shapes:
//
//   * Typed `capabilities` — the default for mutations whose shape the
//     renderer knows. Each field is an optional struct; presence == permitted.
//   * Generic `links` — for open-ended hypermedia (navigation, breadcrumbs,
//     related entities, pagination, alternative views). Each link's `rel`
//     is the verb; presence in the array == permitted.
//
// See docs/authorization-model.md for the rule on which to use.

// ---------------------------------------------------------------------------
// Generic link relation (open-ended hypermedia).
// ---------------------------------------------------------------------------

/** A hypermedia link. The presence of a link is the permission grant. */
export interface Link {
  /** The relation name (e.g. "self", "parent", "next-page", "share"). */
  rel: string;
  /** Absolute or relative URL the link points to. */
  href: string;
  /** HTTP method to use; defaults to GET on the consuming side. */
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  /** Optional human-readable label, used by renderers that need a button text. */
  title?: string;
  /** Optional content type the server expects in the request body. */
  type?: string;
}

// ---------------------------------------------------------------------------
// Typed capabilities (the formschema/list/page pattern).
//
// Pletka's existing FormSchema / ListSchema embed a typed Capabilities struct
// where each field is a nullable pointer; non-nil = permitted. The TS mirror
// uses optional fields with the same meaning. New cap kinds are added by
// extending this interface (a schema-minor change for additive caps).
// ---------------------------------------------------------------------------

export interface ReorderCap {
  enabled: boolean;
  url: string;
  order_field: string;
}

export interface InlineRenameCap {
  enabled: boolean;
  field: string;
  save_url_template: string;
}

export interface CreateCap {
  url: string;
  /** Optional schema URL to fetch the create-form schema from. */
  schema_url?: string;
  /** Optional flag indicating the action requires authentication. */
  auth_required?: boolean;
}

export interface EditCap {
  url: string;
  schema_url?: string;
  auth_required?: boolean;
}

export interface DeleteCap {
  url: string;
  /** Optional confirmation copy (multilingual map). */
  confirm?: Record<string, string>;
  auth_required?: boolean;
}

export interface StatsCap {
  url: string;
}

export interface Capabilities {
  reorder?: ReorderCap;
  inline_rename?: InlineRenameCap;
  create?: CreateCap;
  edit?: EditCap;
  delete?: DeleteCap;
  stats?: StatsCap;
  /** Forwards-compatible: unknown caps are ignored by the renderer. */
  [key: string]: unknown;
}

// ---------------------------------------------------------------------------
// Widget envelope.
// ---------------------------------------------------------------------------

/** Base shape for any widget. Concrete widgets extend this with extra fields. */
export interface Widget {
  /** Discriminator: which widget type this is. */
  type: string;
  /** Stable id for diffing / focus management. */
  id?: string;
  /** Children, if the widget is a layout container. */
  children?: Widget[];
  /** Typed mutation grants scoped to this widget. */
  capabilities?: Capabilities;
  /** Open-ended hypermedia links scoped to this widget. */
  links?: Link[];
  /** Free-form payload that the concrete widget renderer interprets. */
  [key: string]: unknown;
}

// ---------------------------------------------------------------------------
// Page envelope.
// ---------------------------------------------------------------------------

/** Top-level envelope for a page response. */
export interface PageSchema {
  /** Schema major.minor.patch this page conforms to. */
  schemaVersion: string;
  /** Page title for the document head. */
  title?: string;
  /** Locale code (BCP-47), e.g. "en", "nl-BE". */
  lang?: string;
  /** Page-level open-ended hypermedia links. */
  links?: Link[];
  /** Page-level typed mutation grants (e.g. delete on the represented resource). */
  capabilities?: Capabilities;
  /** Root widget tree to render. */
  body: Widget;
  /** Optional non-rendered metadata (e.g. user, feature flags, locale settings). */
  meta?: Record<string, unknown>;
}
