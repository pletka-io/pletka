// frontend/src/lib/components/admin/admin-panel-registry.ts
//
// Registry of bespoke admin-section panels, keyed by AdminSection.kind. Mirrors
// the entity-list row-widget registry: AdminShell looks a kind up here instead
// of a hardcoded if/else chain, so a new panel drops in with one register call
// and zero AdminShell edits.
//
// This is only for CUSTOM panels (git-restore, materialization, and any
// platform-shipped panel). The generic schema-driven kinds
// (entity-list / list / form / composite) are NOT registered here — AdminShell
// renders those from the section's schema_url.
//
// Platform builds add their own panels by shipping their own `admin-shell`
// island entry that imports registerAdminPanel, registers its components, then
// imports the core admin-shell island (which mounts AdminShell with the
// registry already populated). The frontend island resolver serves the
// platform's admin-shell over core's when present — the same override model
// platform already uses for its other islands.

import type { Component } from 'svelte';
import AdminGitRestore from './AdminGitRestore.svelte';
import MaterializationPanel from './MaterializationPanel.svelte';

const registry = new Map<string, Component<Record<string, never>>>();

/** registerAdminPanel associates an AdminSection.kind with a Svelte component. */
export function registerAdminPanel(kind: string, component: Component<any>): void {
  registry.set(kind, component as Component<Record<string, never>>);
}

/** getAdminPanel returns the registered panel for a kind, or null if none. */
export function getAdminPanel(kind?: string): Component<Record<string, never>> | null {
  if (!kind) return null;
  return registry.get(kind) ?? null;
}

// Built-in panels shipped by the core application.
registerAdminPanel('git-restore', AdminGitRestore as Component<any>);
registerAdminPanel('materialization', MaterializationPanel as Component<any>);
