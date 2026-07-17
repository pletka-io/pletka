export interface PathElement {
  prefix: string;
  localName: string;
  label: string;
  isClass: boolean;
  raw: string;
}

/**
 * Parse an ontology path string into structured path elements.
 * Works with both long paths (crm:P44_has_condition) and short paths (P44).
 * Class vs property is determined by position: path starts with property (index 0),
 * then alternates property → class → property → class.
 */
export function parseOntologyPath(path: string): PathElement[] {
  if (!path) return [];
  let normalized = path.trim();
  if (normalized.startsWith('->')) normalized = normalized.substring(2);

  let elements: string[];
  if (normalized.includes('\u2192')) {
    elements = normalized.split(/\s*\u2192\s*/);
  } else if (normalized.includes('->')) {
    elements = normalized.split('->');
  } else {
    elements = [normalized];
  }

  return elements.filter(el => el.trim()).map((el, idx) => {
    const raw = el.trim();
    const clean = raw.replace(/\[[^\]]+\]/g, '').trim();
    let prefix = '';
    let localName = clean;
    if (clean.includes(':')) {
      const colonIdx = clean.indexOf(':');
      prefix = clean.substring(0, colonIdx);
      localName = clean.substring(colonIdx + 1);
    }
    const isClass = idx % 2 === 1;
    const label = prefix ? `${prefix}:${localName}` : localName;
    return { prefix, localName, label, isClass, raw };
  });
}

/**
 * Convert structured weave PathElement[] to display PathElement[].
 * Prefer this over parseOntologyPath() when path_elements are available —
 * they carry the full long-form data with proper class/property types.
 */
export function pathElementsToDisplay(elements: import('$lib/types/weave-types').PathElement[]): PathElement[] {
  if (!elements || elements.length === 0) return [];
  return elements.map(pe => {
    const prefix = pe.prefix || '';
    const localName = pe.local_name || '';
    const label = prefix ? `${prefix}:${localName}` : localName;
    const isClass = pe.type === 'class' || pe.type === 'property-class';
    const raw = pe.uri || label;
    return { prefix, localName, label, isClass, raw };
  });
}

/** Format an ontology path for clipboard copy: strip leading -> and use arrow separators. */
export function formatPathForCopy(path: string): string {
  if (!path) return '';
  let normalized = path.trim();
  if (normalized.startsWith('->')) normalized = normalized.substring(2);
  return normalized.replace(/->/g, ' \u2192 ');
}
