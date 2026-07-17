<script lang="ts">
  import type { AdminSchema, AdminSection } from '$lib/types/admin-schema';
  import { tr } from '$lib/types/form-schema';
  import SettingsSidebar from '$lib/components/settings/SettingsSidebar.svelte';
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';
  import ListManager from '$lib/components/list/ListManager.svelte';
  import EntityListView from '$lib/components/entity-list/EntityListView.svelte';
  import CompositePane from '$lib/components/settings/CompositePane.svelte';
  import { getAdminPanel } from './admin-panel-registry';

  let { lang: initialLang = 'en' }: { lang?: string } = $props();

  let schema = $state<AdminSchema | null>(null);
  let activeSection = $state<AdminSection | null>(null);
  let loading = $state(true);
  let errorMessage = $state('');
  let lang = $state('en');

  $effect(() => {
    lang = initialLang;
    fetchAdminSchema();
    syncHashToSection();
  });

  async function fetchAdminSchema() {
    loading = true;
    errorMessage = '';
    try {
      const res = await fetch('/admin/schema');
      if (!res.ok) throw new Error(`Failed to load admin: ${res.status}`);
      schema = await res.json();
      const hash = window.location.hash.slice(1);
      const found = schema!.sections.find(s => s.id === hash);
      activeSection = found ?? schema!.sections[0] ?? null;
    } catch (e: any) {
      errorMessage = e.message;
    } finally {
      loading = false;
    }
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
        <div class="h-24 bg-gray-200 rounded"></div>
      </div>
    </div>
  </div>
{:else if errorMessage && !schema}
  <div class="text-red-600 p-8">{errorMessage}</div>
{:else if schema}
  <div class="mb-6">
    <h1 class="text-2xl font-bold text-gray-900">
      {tr(schema.title, lang)}
    </h1>
    <p class="mt-1 text-sm text-gray-500">
      Global administration
    </p>
  </div>

  {#if schema.warnings && schema.warnings.length}
    <div class="mb-6 space-y-3">
      {#each schema.warnings as w (w.id)}
        <div
          class="flex items-start gap-3 rounded-md border-l-4 p-4
            {w.severity === 'error' ? 'border-red-400 bg-red-50 text-red-800' : ''}
            {w.severity === 'warning' ? 'border-amber-400 bg-amber-50 text-amber-900' : ''}
            {w.severity === 'info' ? 'border-blue-400 bg-blue-50 text-blue-900' : ''}"
        >
          <div class="flex-1">
            <p class="text-sm">{tr(w.message, lang)}</p>
            {#if w.action_href}
              <a href={w.action_href} class="mt-2 inline-block text-sm font-medium underline hover:no-underline">
                {tr(w.action_label, lang)}
              </a>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}

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
              <h3 class="mt-2 text-sm font-medium text-gray-900">{tr(activeSection.label, lang)}</h3>
              <p class="mt-1 text-sm text-gray-500">Coming soon.</p>
            </div>
          {:else if getAdminPanel(activeSection.kind)}
            {@const Panel = getAdminPanel(activeSection.kind)}
            <Panel />
          {:else if activeSection.schema_url}
            {#if activeSection.kind === 'entity-list'}
              <EntityListView schemaUrl={activeSection.schema_url} />
            {:else if activeSection.kind === 'list'}
              <ListManager schemaUrl={activeSection.schema_url} />
            {:else if activeSection.kind === 'form'}
              <FormRenderer
                schemaUrl={activeSection.schema_url}
                onSuccessAction="none"
                onsuccess={() => {
                  /* stay on page */
                }}
              />
            {:else if activeSection.kind === 'composite'}
              <CompositePane schemaUrl={activeSection.schema_url} {lang} projectId="" />
            {:else}
              <div class="rounded-md border border-gray-200 bg-white p-4 text-sm text-gray-600">
                Unsupported admin section kind.
              </div>
            {/if}
          {/if}
        {/key}
      {/if}
    </div>
  </div>
{/if}
