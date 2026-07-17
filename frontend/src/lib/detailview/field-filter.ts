// Pure, client-side substring filter shared by both the read-only entity
// view and the override editor. Walks the in-memory tree (no backend
// round-trip) and returns the set of override_ids whose field text
// contains the query plus the same ids in tree-walk order so the caller
// can keep a stable next/prev nav cursor.
//
// The shape we accept is the lowest common denominator of
// EntityViewResponse.sections and OverrideEditorResponse.categories —
// both nest items with fields keyed by override_id.

interface FilterableField {
  override_id: number;
  field_id?: string;
  field_semantic_id?: string;
  display_name?: Record<string, string>;
  description?: Record<string, string>;
  expected_value_type?: string;
  ontology_path?: string;
}

interface FilterableItem {
  fields: FilterableField[];
}

interface FilterableCategory {
  items: FilterableItem[];
}

export interface FilterResult {
  set: Set<number>;
  order: number[];
}

const EMPTY: FilterResult = { set: new Set(), order: [] };

export function filterFields(query: string, categories: FilterableCategory[]): FilterResult {
  const q = query.trim().toLowerCase();
  if (!q) return EMPTY;

  const order: number[] = [];
  const set = new Set<number>();

  for (const cat of categories) {
    for (const it of cat.items) {
      for (const f of it.fields) {
        const haystack = [
          Object.values(f.display_name || {}).join(' '),
          f.field_semantic_id ?? f.field_id ?? '',
          Object.values(f.description || {}).join(' '),
          f.expected_value_type ?? '',
          f.ontology_path ?? '',
        ]
          .join(' ')
          .toLowerCase();
        if (haystack.includes(q)) {
          if (!set.has(f.override_id)) {
            set.add(f.override_id);
            order.push(f.override_id);
          }
        }
      }
    }
  }
  return { set, order };
}
