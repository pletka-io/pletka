import type { Component } from 'svelte';
import { mount } from 'svelte';

import { installGlobalHandlers } from '$lib/error-tracking';

// Register error handlers as early as possible so they catch exceptions
// thrown during island bootstrap.
installGlobalHandlers();

/**
 * Mount a Svelte island component into all matching DOM elements.
 *
 * Usage in gohtml:
 *   <div data-island="entity-view" data-prop-project-id="abc123" data-prop-entity-id="xyz"></div>
 *
 * Usage in island entry:
 *   import { mountIsland } from '../mount';
 *   import DetailView from '$lib/detailview/components/DetailView.svelte';
 *   mountIsland('entity-view', DetailView);
 */
/** Parse a data attribute value, converting booleans, numbers, and
 *  JSON-encoded objects/arrays. JSON parsing is opt-in via leading `{`
 *  or `[` so plain string values that happen to start with a number
 *  digit don't get reinterpreted. */
function parseAttrValue(value: string): unknown {
  if (value === 'true') return true;
  if (value === 'false') return false;
  if (/^\d+$/.test(value)) return Number(value);
  if (value.length > 1 && (value[0] === '{' || value[0] === '[')) {
    try {
      return JSON.parse(value);
    } catch {
      // not valid JSON — fall through and treat as string
    }
  }
  return value;
}

// Registry of island components for lazy mounting
type IslandComponent = Component<any>;

const islandRegistry = new Map<string, IslandComponent>();

function mountElements(name: string, component: IslandComponent) {
  const elements = document.querySelectorAll(`[data-island="${name}"]`);

  elements.forEach((el) => {
    if (!(el instanceof HTMLElement)) return;
    // Skip already-mounted elements
    if (el.dataset.mounted) return;

    // Extract props from data-prop-* attributes
    const props: Record<string, unknown> = {};
    for (const attr of el.attributes) {
      if (attr.name.startsWith('data-prop-')) {
        // Convert kebab-case to camelCase: data-prop-model-id -> modelId
        const key = attr.name
          .slice('data-prop-'.length)
          .replace(/-([a-z])/g, (_, c) => c.toUpperCase());
        props[key] = parseAttrValue(attr.value);
      }
    }

    // Clear server-side loading placeholder before mounting
    el.textContent = '';

    // Mount the Svelte component
    mount(component, {
      target: el,
      props,
    });

    el.dataset.mounted = 'true';
  });
}

export function mountIsland(name: string, component: IslandComponent) {
  // Register component for later lazy mounting
  islandRegistry.set(name, component);

  // Mount any elements that already have the data-island attribute
  mountElements(name, component);

  // Expose a global helper so page JS can trigger lazy mounts
  if (typeof window !== 'undefined') {
    (window as any).__mountIsland = (islandName: string) => {
      const comp = islandRegistry.get(islandName);
      if (comp) {
        mountElements(islandName, comp);
      } else {
        console.warn(
          `[mountIsland] window.__mountIsland('${islandName}') called but no island registered with that name. ` +
            'Possible cause: the entry script for this island was not loaded on this page.',
        );
      }
    };
  }

  // Schedule a one-time post-load scan that warns about any
  // data-island="…" placeholder whose entry script never registered.
  // Without this, a deploy / template misalignment leaves an empty
  // div with no console signal — the user just sees a missing UI.
  scheduleUnmountedScan();
}

let unmountedScanScheduled = false;

function scheduleUnmountedScan() {
  if (unmountedScanScheduled || typeof window === 'undefined') return;
  unmountedScanScheduled = true;
  const run = () => {
    // Defer one tick past the load event so any island scripts queued
    // by the browser have a chance to call mountIsland() first.
    setTimeout(scanUnmountedIslands, 0);
  };
  if (document.readyState === 'complete') {
    run();
  } else {
    window.addEventListener('load', run, { once: true });
  }
}

function scanUnmountedIslands() {
  const seen = new Set<string>();
  document.querySelectorAll<HTMLElement>('[data-island]').forEach((el) => {
    const name = el.dataset.island;
    if (!name || seen.has(name)) return;
    seen.add(name);
    if (!islandRegistry.has(name)) {
      console.warn(
        `[mountIsland] Found <div data-island="${name}"> in the DOM, but no island bundle registered "${name}". ` +
          'Possible cause: the entry script for this island was not loaded on this page ' +
          '(check {{islandScripts "<name>"}} in the gohtml template). The placeholder will stay empty.',
      );
    } else if (!el.dataset.mounted) {
      // Registered but never mounted on this element — usually means
      // the registration ran before this element existed AND no later
      // call mounted it. Trigger one more pass.
      const comp = islandRegistry.get(name);
      if (comp) mountElements(name, comp);
    }
  });
}
