export function controlID(prefix: string, ...parts: Array<string | number | undefined>): string {
  const safePrefix = safePart(prefix || 'entity-list');
  const safeParts = parts.map((part) => safePart(String(part ?? ''))).filter(Boolean);
  return [safePrefix, ...safeParts].join('-');
}

function safePart(value: string): string {
  return value
    .trim()
    .replace(/[^a-zA-Z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '');
}
