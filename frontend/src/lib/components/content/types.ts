/**
 * Content-page schema types — the JSON shape the server emits and the
 * dispatcher consumes. Mirrors pkg/weave/content/schema.go.
 */

export interface NavMeta {
  label?: string;
  order?: number;
  visible?: boolean;
}

export interface SEOMeta {
  title?: string;
  description?: string;
}

export interface Block {
  type: string;
  data?: Record<string, unknown>;
}

export interface PageSchema {
  slug: string;
  lang: string;
  template: string;
  nav?: NavMeta;
  seo?: SEOMeta;
  blocks: Block[];
}
