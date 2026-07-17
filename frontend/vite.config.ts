import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { delimiter, dirname, relative, resolve } from 'path';
import { existsSync, readFileSync } from 'fs';

type ContributionKind =
  | 'globalEntries'
  | 'islands'
  | 'formWidgets'
  | 'entityListRowWidgets'
  | 'entityListEditorWidgets'
  | 'contentWidgets';

type ContributionEntry = {
  name: string;
  source: string;
  description?: string;
  overrides?: 'core';
};

type ResolvedContributionEntry = ContributionEntry & {
  sourcePath: string;
  manifestPath: string;
  core: boolean;
};

type FrontendContributions = Record<ContributionKind, ResolvedContributionEntry[]>;

const contributionKinds: ContributionKind[] = [
  'globalEntries',
  'islands',
  'formWidgets',
  'entityListRowWidgets',
  'entityListEditorWidgets',
  'contentWidgets',
];

const virtualContributionsModule = 'virtual:pletka-frontend-contributions';
const resolvedVirtualContributionsModule = `\0${virtualContributionsModule}`;
const coreManifestPath = resolve(__dirname, 'core.frontend.json');

function emptyContributions(): FrontendContributions {
  return {
    globalEntries: [],
    islands: [],
    formWidgets: [],
    entityListRowWidgets: [],
    entityListEditorWidgets: [],
    contentWidgets: [],
  };
}

function envPaths(name: string): string[] {
  return (process.env[name] ?? '')
    .split(delimiter)
    .map((path) => path.trim())
    .filter(Boolean);
}

function validateName(kind: ContributionKind, name: unknown, manifestPath: string): string {
  if (typeof name !== 'string' || name.trim() === '') {
    throw new Error(`${manifestPath}: ${kind} entry requires a non-empty name`);
  }
  const trimmed = name.trim();
  if (!/^[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(trimmed)) {
    throw new Error(`${manifestPath}: invalid ${kind} name "${trimmed}"`);
  }
  return trimmed;
}

function validateOptionalText(
  field: string,
  value: unknown,
  manifestPath: string,
): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  if (typeof value !== 'string') {
    throw new Error(`${manifestPath}: ${field} must be a string when set`);
  }
  return value;
}

function validateSource(
  kind: ContributionKind,
  source: unknown,
  manifestPath: string,
): string {
  if (typeof source !== 'string' || source.trim() === '') {
    throw new Error(`${manifestPath}: ${kind} entry requires a non-empty source`);
  }
  const trimmed = source.trim();
  const validSuffixes = kind === 'globalEntries'
    ? ['.ts', '.css']
    : kind === 'islands'
      ? ['.ts']
      : ['.svelte'];
  if (!validSuffixes.some((suffix) => trimmed.endsWith(suffix))) {
    throw new Error(`${manifestPath}: ${kind} source "${trimmed}" must end with ${validSuffixes.join(' or ')}`);
  }
  const sourcePath = resolve(dirname(manifestPath), trimmed);
  if (!existsSync(sourcePath)) {
    throw new Error(`${manifestPath}: ${kind} source does not exist: ${sourcePath}`);
  }
  return sourcePath;
}

function manifestPaths(): { path: string; core: boolean }[] {
  const extraPaths = [
    ...envPaths('PLETKA_FRONTEND_MANIFESTS'),
    ...envPaths('PLETKA_FRONTEND_CONTRIBUTIONS'),
  ].map((path) => ({ path: resolve(__dirname, path), core: false }));

  return [{ path: coreManifestPath, core: true }, ...extraPaths];
}

function readFrontendManifests(): FrontendContributions {
  const contributions = emptyContributions();

  for (const manifest of manifestPaths()) {
    const manifestPath = manifest.path;
    const raw = JSON.parse(readFileSync(manifestPath, 'utf8')) as Record<string, unknown>;
    validateOptionalText('id', raw.id, manifestPath);
    validateOptionalText('name', raw.name, manifestPath);
    validateOptionalText('description', raw.description, manifestPath);

    for (const kind of contributionKinds) {
      const entries = raw[kind];
      if (entries === undefined) {
        continue;
      }
      if (!Array.isArray(entries)) {
        throw new Error(`${manifestPath}: ${kind} must be an array`);
      }
      for (const entry of entries) {
        if (entry === null || typeof entry !== 'object') {
          throw new Error(`${manifestPath}: ${kind} entries must be objects`);
        }
        const rawEntry = entry as Record<string, unknown>;
        const name = validateName(kind, rawEntry.name, manifestPath);
        const sourcePath = validateSource(kind, rawEntry.source, manifestPath);
        const description = validateOptionalText(`${kind} "${name}" description`, rawEntry.description, manifestPath);
        const overrides = rawEntry.overrides;
        if (overrides !== undefined && overrides !== 'core') {
          throw new Error(`${manifestPath}: ${kind} "${name}" overrides must be "core" when set`);
        }
        contributions[kind].push({
          name,
          source: String(rawEntry.source),
          description,
          sourcePath,
          manifestPath,
          core: manifest.core,
          overrides: overrides as 'core' | undefined,
        });
      }
    }
  }

  validateContributionDuplicates(contributions);
  return contributions;
}

function validateContributionDuplicates(contributions: FrontendContributions): void {
  for (const kind of contributionKinds) {
    const seen = new Map<string, ResolvedContributionEntry>();
    const resolved: ResolvedContributionEntry[] = [];
    for (const entry of contributions[kind]) {
      const existing = seen.get(entry.name);
      if (existing) {
        if (existing.core && !entry.core && entry.overrides === 'core') {
          const index = resolved.findIndex((candidate) => candidate.name === entry.name);
          if (index >= 0) {
            resolved[index] = entry;
          }
          seen.set(entry.name, entry);
          continue;
        }
        throw new Error(
          `${entry.manifestPath}: duplicate ${kind} "${entry.name}" from ${entry.sourcePath}; already registered by ${existing.sourcePath}` +
            (existing.core && !entry.core ? '; add "overrides": "core" to replace the core entry intentionally' : ''),
        );
      }
      seen.set(entry.name, entry);
      resolved.push(entry);
    }
    contributions[kind] = resolved;
  }
}

function getViteEntries(contributions: FrontendContributions): Record<string, string> {
  const entries: Record<string, string> = {};
  for (const entry of [...contributions.globalEntries, ...contributions.islands]) {
    entries[entry.name] = entry.sourcePath;
  }
  return entries;
}

function importPathFor(sourcePath: string): string {
  let path = relative(__dirname, sourcePath);
  if (!path.startsWith('.')) {
    path = `./${path}`;
  }
  return path.replaceAll('\\', '/');
}

function generatedRegistrationModule(contributions: FrontendContributions): string {
  const lines: string[] = [];

  let index = 0;
  const addRegistrations = (
    kind: Exclude<ContributionKind, 'islands'>,
    registerFn: string,
    importLine: string,
  ) => {
    if (contributions[kind].length === 0) {
      return;
    }
    const registrations = contributions[kind].filter((entry) => !entry.core || entry.overrides === 'core');
    if (registrations.length === 0) {
      return;
    }
    lines.push(importLine);
    for (const entry of registrations) {
      const symbol = `Contribution${index++}`;
      lines.push(`import ${symbol} from ${JSON.stringify(importPathFor(entry.sourcePath))};`);
      lines.push(`${registerFn}(${JSON.stringify(entry.name)}, ${symbol});`);
    }
  };

  addRegistrations(
    'formWidgets',
    'registerWidget',
    'import { registerWidget } from "$lib/components/form/widget-registry";',
  );
  addRegistrations(
    'entityListRowWidgets',
    'registerRowWidget',
    'import { registerRowWidget } from "$lib/components/entity-list/row-widget-registry";',
  );
  addRegistrations(
    'entityListEditorWidgets',
    'registerEntityListEditor',
    'import { registerEntityListEditor } from "$lib/components/entity-list/editor-widget-registry";',
  );
  addRegistrations(
    'contentWidgets',
    'registerContentWidget',
    'import { registerContentWidget } from "$lib/components/content/widget-registry";',
  );

  lines.push('export {};');
  return `${lines.join('\n')}\n`;
}

function frontendContributionsPlugin(contributions: FrontendContributions) {
  return {
    name: 'pletka-frontend-contributions',
    resolveId(id: string) {
      if (id === virtualContributionsModule) {
        return resolvedVirtualContributionsModule;
      }
      return null;
    },
    load(id: string) {
      if (id === resolvedVirtualContributionsModule) {
        return generatedRegistrationModule(contributions);
      }
      return null;
    },
  };
}

const frontendContributions = readFrontendManifests();

export default defineConfig({
  // Assets are served at /static/dist/* by the Go static file handler.
  // Without `base`, Vite emits dynamic-import preload <link href="chunks/…">
  // entries as document-relative URLs. Browsers then resolve those
  // against the current page (e.g. /projects/AME/models/AMEM.1) instead
  // of the asset root and the preload 404s. The actual import still
  // works (it resolves relative to the entry script URL which IS under
  // /static/dist/), so content renders fine — but the console fills
  // with "Unable to preload CSS for /assets/EntityListView-…" warnings
  // on the first dynamic-chunk fetch of a page. Setting `base` makes
  // Vite emit absolute /static/dist/… paths everywhere, so the preload
  // URLs are correct.
  base: '/static/dist/',
  plugins: [frontendContributionsPlugin(frontendContributions), svelte()],
  resolve: {
    alias: {
      $lib: resolve(__dirname, 'src/lib'),
    },
  },
  build: {
    outDir: resolve(__dirname, '../pkg/assets/static/dist'),
    emptyOutDir: true,
    manifest: true,
    rollupOptions: {
      input: {
        ...getViteEntries(frontendContributions),
      },
      output: {
        entryFileNames: '[name]-[hash].js',
        chunkFileNames: 'chunks/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
  },
});
