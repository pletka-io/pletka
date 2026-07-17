import type { Component } from 'svelte';
import type { EntityListSchema } from '$lib/types/entity-list-schema';

export type EntityListEditorProps = {
  schema: EntityListSchema;
  mode: 'create' | 'edit';
  itemId?: string;
  lang: string;
  onsuccess?: () => void | Promise<void>;
  oncancel?: () => void;
};

export type EntityListEditorComponent = Component<EntityListEditorProps>;

const registry = new Map<string, EntityListEditorComponent>();

export function registerEntityListEditor(kind: string, component: EntityListEditorComponent) {
  if (!kind) return;
  registry.set(kind, component);
}

export function getEntityListEditor(kind: string | undefined): EntityListEditorComponent | null {
  if (!kind) return null;
  return registry.get(kind) ?? null;
}
