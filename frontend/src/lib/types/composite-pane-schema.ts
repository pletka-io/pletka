import type { Translations } from './form-schema';

/**
 * CompositePaneSchema is the envelope returned at a pane-schema URL when
 * a settings section needs more than one panel. Each panel points at a
 * child schema URL (form-schema or list-schema) that the dispatcher
 * mounts in order.
 */
export interface CompositePaneSchema {
  kind: 'composite-pane';
  title?: Translations;
  subtitle?: Translations;
  panels: CompositePanel[];
}

/** CompositePanel is a single named panel within a CompositePaneSchema. */
export interface CompositePanel {
  id: string;
  label?: Translations;
  schema_url: string;
  /**
   * Optional discriminator. When set, the dispatcher routes the panel
   * to a dedicated component instead of probing schema_url for a
   * form / list shape. Known kinds:
   *   - "linked-ontologies" → LinkedOntologiesPanel
   */
  kind?: string;
}

/** isCompositePaneSchema narrows an unknown response to CompositePaneSchema. */
export function isCompositePaneSchema(x: unknown): x is CompositePaneSchema {
  return (
    typeof x === 'object' &&
    x !== null &&
    (x as { kind?: string }).kind === 'composite-pane' &&
    Array.isArray((x as { panels?: unknown }).panels)
  );
}
