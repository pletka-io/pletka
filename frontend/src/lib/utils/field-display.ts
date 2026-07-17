// frontend/src/lib/utils/field-display.ts

import type { PathElement } from '$lib/types/weave-types';

/**
 * BadgeVariant mirrors the `variant` values accepted by Badge.svelte.
 */
export type BadgeVariant =
  | 'gray'
  | 'green'
  | 'blue'
  | 'purple'
  | 'yellow'
  | 'red'
  | 'indigo'
  | 'amber';

/**
 * valueTypeColor maps an expected value type string to a badge color variant.
 * Unknown types fall back to gray.
 */
export function valueTypeColor(type: string): BadgeVariant {
  const colors: Record<string, BadgeVariant> = {
    String: 'gray',
    URI: 'blue',
    uri: 'blue',
    Model: 'purple',
    Collection: 'green',
    Concept: 'indigo',
    Date: 'yellow',
    Integer: 'yellow',
    GeoJson: 'green',
  };
  return colors[type] ?? 'gray';
}

/**
 * statusVariant maps an entity status to a badge color variant.
 */
export function statusVariant(status: string): BadgeVariant {
  switch (status) {
    case 'published':
      return 'green';
    case 'draft':
      return 'gray';
    case 'deprecated':
      return 'yellow';
    case 'In Process':
    case 'in_process':
      return 'blue';
    default:
      return 'gray';
  }
}

/**
 * parseScope extracts a prefix:localName pair from either a path element object
 * or a pre-flattened string. Returns empty strings when the input is missing.
 */
export function parseScope(scope: string | PathElement | undefined | null): {
  prefix: string;
  localName: string;
} {
  if (!scope) return { prefix: '', localName: '' };
  if (typeof scope === 'string') {
    const idx = scope.indexOf(':');
    if (idx === -1) return { prefix: '', localName: scope };
    return { prefix: scope.slice(0, idx), localName: scope.slice(idx + 1) };
  }
  return { prefix: scope.prefix ?? '', localName: scope.local_name ?? '' };
}
