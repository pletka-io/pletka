<script lang="ts">
  import type { SettingsSchema, SettingsSection } from '$lib/types/settings-schema';
  import { tr } from '$lib/types/form-schema';
  import SettingsSidebar from './SettingsSidebar.svelte';
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';
  import ListManager from '$lib/components/list/ListManager.svelte';
  import AutocompleteProbe from '$lib/components/settings/tools/AutocompleteProbe.svelte';
  import CompositePane from './CompositePane.svelte';

  let {
    projectId = '',
    schemaUrlBase,
  }: {
    projectId?: string;
    schemaUrlBase?: string;
  } = $props();

  const baseUrl = $derived(schemaUrlBase ?? `/projects/${projectId}/settings`);
  const releaseVersion = $derived(new URLSearchParams(window.location.search).get('version') || '');

  let schema = $state<SettingsSchema | null>(null);
  let activeSection = $state<SettingsSection | null>(null);
  let loading = $state(true);
  let errorMessage = $state('');
  let lang = $state('en');
  let sectionKind = $state<'form' | 'list' | 'composite' | 'error' | null>(null);
  let sectionError = $state<string>('');
  let sectionProbeId = 0;

  $effect(() => {
    fetchSettingsSchema();
    syncHashToSection();
  });

  type ProbeResult =
    | { kind: 'form' | 'list' | 'composite' }
    | { kind: 'error'; status?: number; message: string };

  async function probeSectionKind(url: string): Promise<ProbeResult> {
    try {
      const res = await fetch(url);
      if (!res.ok) {
        const msg =
          res.status === 401
            ? 'Your session has expired. Please log in again to load this section.'
            : res.status === 403
              ? "You don't have permission to view this section."
              : `Failed to load section (HTTP ${res.status}).`;
        return { kind: 'error', status: res.status, message: msg };
      }
      const data = await res.json();
      if (data?.kind === 'composite-pane') return { kind: 'composite' };
      if (data?.kind === 'list-schema' || Array.isArray(data?.columns)) return { kind: 'list' };
      return { kind: 'form' };
    } catch (e: any) {
      return { kind: 'error', message: e?.message ?? 'Network error loading section.' };
    }
  }

  $effect(() => {
    if (activeSection?.kind) {
      sectionKind = activeSection.kind;
      sectionError = '';
    } else if (
      activeSection?.schema_url &&
      activeSection.id !== 'categories' &&
      activeSection.id !== 'autocomplete'
    ) {
      const probeId = ++sectionProbeId;
      const sectionID = activeSection.id;
      const schemaURL = activeSection.schema_url;
      sectionKind = null;
      sectionError = '';
      probeSectionKind(schemaURL).then((r) => {
        if (probeId !== sectionProbeId) return;
        if (activeSection?.id !== sectionID) return;
        sectionKind = r.kind;
        if (r.kind === 'error') sectionError = r.message;
      });
    } else {
      sectionProbeId += 1;
      sectionKind = null;
      sectionError = '';
    }
  });

  async function fetchSettingsSchema(opts: { preserveSection?: boolean } = {}) {
    if (!opts.preserveSection) loading = true;
    try {
      const schemaURL = new URL(`${baseUrl}/schema`, window.location.origin);
      if (releaseVersion) {
        schemaURL.searchParams.set('version', releaseVersion);
      }
      const res = await fetch(schemaURL.toString());
      if (!res.ok) throw new Error(`Failed to load settings: ${res.status}`);
      schema = await res.json();

      if (opts.preserveSection && activeSection) {
        // Re-resolve the active section against the new schema so any
        // derived flags on it (e.g. NeedsAttention) refresh too.
        const id = activeSection.id;
        const next = schema!.sections.find(s => s.id === id);
        if (next) activeSection = next;
      } else {
        // Initial load: pick from hash or default to first.
        const hash = window.location.hash.slice(1);
        const found = schema!.sections.find(s => s.id === hash);
        activeSection = found ?? schema!.sections[0] ?? null;
      }
    } catch (e: any) {
      errorMessage = e.message;
    } finally {
      if (!opts.preserveSection) loading = false;
    }
  }

  // Called by child panes after a successful mutation. Re-fetches the
  // settings schema so warnings ('add an ontology'), sidebar attention
  // dots, and any other server-derived state refresh in place — no
  // full page reload needed.
  function handleChildMutation() {
    fetchSettingsSchema({ preserveSection: true });
  }

  function syncHashToSection() {
    const onHashChange = () => {
      if (!schema) return;
      const hash = window.location.hash.slice(1);
      const found = schema.sections.find(s => s.id === hash);
      if (found) activeSection = found;
    };
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }

  function selectSection(id: string) {
    if (!schema) return;
    const found = schema.sections.find(s => s.id === id);
    if (found) {
      activeSection = found;
      window.location.hash = id;
    }
  }
</script>

{#if loading}
  <div class="animate-pulse space-y-4 p-8">
    <div class="h-6 bg-gray-200 rounded w-1/3"></div>
    <div class="flex gap-8">
      <div class="w-56 space-y-2">
        <div class="h-8 bg-gray-200 rounded"></div>
        <div class="h-8 bg-gray-200 rounded"></div>
        <div class="h-8 bg-gray-200 rounded"></div>
      </div>
      <div class="flex-1 space-y-4">
        <div class="h-10 bg-gray-200 rounded"></div>
        <div class="h-10 bg-gray-200 rounded"></div>
      </div>
    </div>
  </div>
{:else if errorMessage && !schema}
  <div class="text-red-600 p-8">{errorMessage}</div>
{:else if schema}
  <!-- Header -->
  <div class="mb-6">
    <div class="flex items-center gap-3">
      <h1 class="text-2xl font-bold text-gray-900">
        Settings
      </h1>
      {#if schema.release}
        <span class="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800">
          {schema.release.version}
        </span>
      {/if}
    </div>
    <p class="mt-1 text-sm text-gray-500">
      {tr(schema.project_name, lang)}
    </p>
  </div>

  {#if schema.release}
    <div class="mb-6 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-blue-200 bg-blue-50 px-4 py-3">
      <div class="flex items-center gap-3">
        <span class="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800">
          release
        </span>
        <div>
          <div class="text-sm font-medium text-blue-900">{tr(schema.release.label, lang)}</div>
          <div class="text-xs text-blue-700">Settings are read-only for this release snapshot.</div>
        </div>
      </div>
      {#if schema.release.draft_url}
        <a
          href={schema.release.draft_url}
          class="inline-flex items-center rounded-md border border-blue-300 bg-white px-3 py-1.5 text-sm font-medium text-blue-800 hover:bg-blue-100"
        >
          Return to draft
        </a>
      {/if}
    </div>
  {/if}

  <!-- Warnings: required setup that's missing. Schema-driven; no client logic. -->
  {#if schema.warnings && schema.warnings.length}
    <div class="mb-6 space-y-3">
      {#each schema.warnings as w (w.id)}
        <div
          class="flex items-start gap-3 rounded-md border-l-4 p-4
            {w.severity === 'error' ? 'border-red-400 bg-red-50 text-red-800' : ''}
            {w.severity === 'warning' ? 'border-amber-400 bg-amber-50 text-amber-900' : ''}
            {w.severity === 'info' ? 'border-blue-400 bg-blue-50 text-blue-900' : ''}"
        >
          <svg class="mt-0.5 h-5 w-5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
          </svg>
          <div class="flex-1">
            <p class="text-sm">{tr(w.message, lang)}</p>
            {#if w.action_href}
              <a
                href={w.action_href}
                class="mt-2 inline-block text-sm font-medium underline hover:no-underline"
              >
                {tr(w.action_label, lang)}
              </a>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Sidebar + Content -->
  <div class="flex gap-8">
    <SettingsSidebar
      sections={schema.sections}
      activeId={activeSection?.id ?? ''}
      {lang}
      onselect={selectSection}
    />

    <div class="flex-1 min-w-0">
      {#if activeSection}
        {#key activeSection.id}
          {#if activeSection.placeholder}
            <div class="rounded-md border-2 border-dashed border-gray-300 bg-gray-50 p-12 text-center">
              <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
              </svg>
              <h3 class="mt-2 text-sm font-medium text-gray-900">{tr(activeSection.label, lang)}</h3>
              <p class="mt-1 text-sm text-gray-500">Coming soon.</p>
            </div>
          {:else if activeSection.id === 'categories'}
            <!-- Categories uses ListManager with existing schema API -->
            <ListManager schemaUrl={activeSection.schema_url ?? ''} />
          {:else if activeSection.id === 'autocomplete'}
            <!-- Diagnostic shell around a real formschema-backed ontology probe -->
            <AutocompleteProbe {projectId} probeSchemaUrl={activeSection.schema_url ?? ''} />
          {:else if activeSection.schema_url}
            {#if sectionKind === 'composite'}
              <CompositePane
                schemaUrl={activeSection.schema_url}
                {lang}
                {projectId}
                onmutate={handleChildMutation}
              />
            {:else if sectionKind === 'list'}
              <ListManager schemaUrl={activeSection.schema_url} />
            {:else if sectionKind === 'form'}
              <FormRenderer
                schemaUrl={activeSection.schema_url}
                onSuccessAction="none"
                onsuccess={() => {
                  /* Stay on page, toast already shown by FormRenderer */
                }}
              />
            {:else if sectionKind === 'error'}
              <div class="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-800">
                <p>{sectionError}</p>
                <button
                  type="button"
                  class="mt-2 inline-block underline hover:no-underline"
                  onclick={() => activeSection && (activeSection = { ...activeSection })}
                >
                  Try again
                </button>
              </div>
            {:else}
              <!-- Probing section kind — show skeleton -->
              <div class="animate-pulse space-y-4">
                <div class="h-8 bg-gray-200 rounded w-1/3"></div>
                <div class="h-24 bg-gray-200 rounded"></div>
              </div>
            {/if}
          {/if}
        {/key}
      {/if}
    </div>
  </div>
{/if}
