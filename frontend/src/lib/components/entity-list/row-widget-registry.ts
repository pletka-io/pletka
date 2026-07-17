// frontend/src/lib/components/entity-list/row-widget-registry.ts

import type { Component } from 'svelte';
import type { EntityRowLayout, RowAction } from '$lib/types/entity-list-schema';
import EntityListRow from './EntityListRow.svelte';
import EditorialListRow from './widgets/EditorialListRow.svelte';

/**
 * RowWidgetProps is the contract every row widget implements.
 * Widgets receive the flattened item from the API, the row layout config from
 * the schema, the current view mode (e.g., "compact"/"detailed"), and the URL
 * templates and context needed to render links.
 */
export interface RowWidgetProps {
  item: Record<string, any>;
  rowLayout: EntityRowLayout;
  viewMode: string;
  detailUrlTemplate: string;
  editUrlTemplate?: string;
  projectId: string;
  releaseVersion?: string;
  lang: string;
  onEdit?: (id: string) => void;
  rowActions?: RowAction[];
  onAction?: (actionId: string, item: Record<string, any>) => void;
}

const registry = new Map<string, Component<RowWidgetProps>>();

/**
 * registerRowWidget associates a widget name with a Svelte component.
 * Call this at module load time; SaaS builds can add their own registrations
 * in a separate entry file without touching core code.
 */
export function registerRowWidget(
  name: string,
  component: Component<RowWidgetProps>,
): void {
  registry.set(name, component);
}

/**
 * getRowWidget returns the component registered under the given name, falling
 * back to the "default" widget (EntityListRow) when the name is empty or
 * unknown. There is intentionally no role parameter — the backend decides
 * which widget to put in the schema.
 */
export function getRowWidget(name?: string): Component<RowWidgetProps> {
  if (name) {
    const found = registry.get(name);
    if (found) return found;
  }
  return registry.get('default')!;
}

// Built-in widgets (shipped with the open-source build).
registerRowWidget('default', EntityListRow as Component<RowWidgetProps>);
registerRowWidget('editorial', EditorialListRow as Component<RowWidgetProps>);
