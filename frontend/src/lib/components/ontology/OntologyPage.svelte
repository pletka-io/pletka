<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';
  import type {
    OntologyPageAction,
    OntologyPageSchema,
  } from '$lib/types/ontology-page';
  import { addToast } from '$lib/stores/toast';
  import { confirmAction } from '$lib/stores/confirm';
  import { getOntologySectionWidget } from './section-registry';

  let { schemaUrl = '' }: { schemaUrl?: string } = $props();

  let schema = $state<OntologyPageSchema | null>(null);
  let loading = $state(true);
  let errorMessage = $state('');
  let formAction = $state<OntologyPageAction | null>(null);
  let importFile = $state<File | null>(null);
  let importVersion = $state('');
  let importCompat = $state('');
  let importSetActive = $state(false);
  let importProbe = $state<any>(null);
  let importBusy = $state('');
  let importError = $state('');

  const lang = $derived(schema?.ui?.primary_language || 'en');

  $effect(() => {
    void loadSchema(schemaUrl);
  });

  async function loadSchema(url: string) {
    if (!url) return;
    loading = true;
    errorMessage = '';
    try {
      const res = await fetch(url, { headers: { Accept: 'application/json' } });
      if (!res.ok) throw new Error(`Failed to load ontology page: ${res.status}`);
      schema = await res.json();
    } catch (e: any) {
      errorMessage = e.message || 'Failed to load ontology page';
    } finally {
      loading = false;
    }
  }

  async function runAction(action: OntologyPageAction) {
    if (action.disabled) return;
    if (action.form_schema_url) {
      formAction = action;
      return;
    }
    if (action.href) {
      window.location.href = action.href;
      return;
    }
    if (!action.url) return;
    const confirmMessage = tr(action.confirm, lang);
    if (confirmMessage) {
      const ok = await confirmAction({
        title: tr(action.label, lang),
        message: confirmMessage,
        confirmLabel: tr(action.label, lang),
        danger: action.style === 'danger',
      });
      if (!ok) return;
    }
    try {
      const res = await fetch(action.url, {
        method: (action.method || 'POST').toUpperCase(),
        credentials: 'same-origin',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (!res.ok) throw new Error(`Action failed: ${res.status}`);
      addToast('success', `${tr(action.label, lang)} completed`);
      if (action.success_href) {
        window.location.href = action.success_href;
        return;
      }
      await loadSchema(schemaUrl);
    } catch (e: any) {
      addToast('error', e.message || 'Action failed');
    }
  }

  async function handleFormSuccess() {
    formAction = null;
    await loadSchema(schemaUrl);
  }

  function importPayload(setActive = importSetActive) {
    if (!importFile) {
      importError = 'Choose an RDF/XML file first.';
      return null;
    }
    const body = new FormData();
    body.append('rdf_file', importFile);
    body.append('version_string', importVersion);
    body.append('compatible_base_versions', importCompat);
    body.append('set_active', setActive ? 'true' : 'false');
    return body;
  }

  async function probeImport() {
    if (!schema?.import) return;
    const body = importPayload();
    if (!body) return;
    importBusy = 'probe';
    importError = '';
    try {
      const res = await fetch(schema.import.probe_url, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
        body,
      });
      const data = await res.json().catch(() => null);
      if (!res.ok) throw new Error(data?.message || data?.error || `Probe failed: ${res.status}`);
      importProbe = data;
      if (!importVersion && data.version_string) importVersion = data.version_string;
      if (!importCompat && data.compatible_base_versions?.length) importCompat = data.compatible_base_versions.join(', ');
    } catch (e: any) {
      importError = e.message || 'Probe failed';
    } finally {
      importBusy = '';
    }
  }

  async function commitImport(setActive: boolean) {
    if (!schema?.import) return;
    const body = importPayload(setActive);
    if (!body) return;
    importBusy = setActive ? 'commit-active' : 'commit';
    importError = '';
    try {
      const res = await fetch(schema.import.commit_url, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
        body,
      });
      const data = await res.json().catch(() => null);
      if (!res.ok) throw new Error(data?.message || data?.error || `Import failed: ${res.status}`);
      addToast('success', `${data?.probe?.version_string || importVersion || 'Version'} imported`);
      const versionID = data?.version?.id;
      if (versionID) {
        window.location.href = `/admin/ontologies/versions/${encodeURIComponent(versionID)}/page`;
        return;
      }
      await loadSchema(schemaUrl);
    } catch (e: any) {
      importError = e.message || 'Import failed';
    } finally {
      importBusy = '';
    }
  }

  function actionClass(style = '') {
    if (style === 'primary') return 'bg-sky-700 text-white hover:bg-sky-800 ring-sky-700';
    if (style === 'danger') return 'bg-red-700 text-white hover:bg-red-800 ring-red-700';
    return 'bg-white text-gray-700 hover:bg-gray-50 ring-gray-300';
  }
</script>

{#if loading && !schema}
  <div class="space-y-4 animate-pulse">
    <div class="h-24 rounded-lg bg-white ring-1 ring-gray-200"></div>
    <div class="grid gap-4 md:grid-cols-3">
      <div class="h-20 rounded-lg bg-white ring-1 ring-gray-200"></div>
      <div class="h-20 rounded-lg bg-white ring-1 ring-gray-200"></div>
      <div class="h-20 rounded-lg bg-white ring-1 ring-gray-200"></div>
    </div>
    <div class="h-40 rounded-lg bg-white ring-1 ring-gray-200"></div>
  </div>
{:else if errorMessage && !schema}
  <div class="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700">
    {errorMessage}
    <button class="ml-3 font-medium underline" onclick={() => loadSchema(schemaUrl)}>Retry</button>
  </div>
{:else if schema}
  <div class="space-y-6">
    <section class="rounded-lg bg-white p-6 shadow-sm ring-1 ring-gray-200">
      <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div class="min-w-0">
          <h1 class="text-2xl font-semibold text-gray-900">{tr(schema.title, lang)}</h1>
          {#if tr(schema.subtitle, lang)}
            <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-600">{tr(schema.subtitle, lang)}</p>
          {/if}
        </div>
        {#if schema.actions && schema.actions.length}
          <div class="flex shrink-0 flex-wrap gap-2">
            {#each schema.actions as action (action.id)}
              <button
                type="button"
                class="inline-flex items-center rounded-md px-3 py-2 text-sm font-medium ring-1 transition-colors {actionClass(action.style)}"
                disabled={action.disabled}
                onclick={() => runAction(action)}
              >
                {tr(action.label, lang)}
              </button>
            {/each}
          </div>
        {/if}
      </div>
    </section>

    {#if schema.import}
      <section class="rounded-lg bg-white p-6 shadow-sm ring-1 ring-gray-200">
        <div class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(280px,360px)]">
          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-gray-700" for="ontology-import-file">RDF/XML file</label>
              <input
                id="ontology-import-file"
                type="file"
                accept=".rdf,.rdfs,.owl,.xml,application/rdf+xml,text/xml,application/xml"
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm file:mr-3 file:rounded-md file:border-0 file:bg-gray-100 file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-gray-700 hover:file:bg-gray-200"
                onchange={(e) => {
                  importFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
                  importProbe = null;
                  importError = '';
                }}
              />
              {#if schema.import.max_bytes}
                <p class="mt-1 text-xs text-gray-500">Maximum file size: {Math.round(schema.import.max_bytes / 1024 / 1024)} MB</p>
              {/if}
            </div>

            <div class="grid gap-4 md:grid-cols-2">
              <div>
                <label class="block text-sm font-medium text-gray-700" for="ontology-import-version">Version</label>
                <input
                  id="ontology-import-version"
                  type="text"
                  bind:value={importVersion}
                  placeholder="Detected from filename when blank"
                  class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-sky-600 focus:outline-none focus:ring-sky-600"
                />
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700" for="ontology-import-compat">Compatible base versions</label>
                <input
                  id="ontology-import-compat"
                  type="text"
                  bind:value={importCompat}
                  placeholder="7.1.3, 6.2"
                  class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-sky-600 focus:outline-none focus:ring-sky-600"
                />
              </div>
            </div>

            <label class="inline-flex items-center gap-2 text-sm text-gray-700">
              <input type="checkbox" bind:checked={importSetActive} class="rounded border-gray-300 text-sky-700 focus:ring-sky-600" />
              Set imported version active
            </label>

            {#if importError}
              <div class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">{importError}</div>
            {/if}

            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                class="inline-flex items-center rounded-md bg-white px-3 py-2 text-sm font-medium text-gray-700 ring-1 ring-gray-300 hover:bg-gray-50 disabled:opacity-50"
                disabled={!!importBusy}
                onclick={probeImport}
              >
                {importBusy === 'probe' ? 'Parsing...' : 'Probe file'}
              </button>
              <button
                type="button"
                class="inline-flex items-center rounded-md bg-sky-700 px-3 py-2 text-sm font-medium text-white ring-1 ring-sky-700 hover:bg-sky-800 disabled:opacity-50"
                disabled={!!importBusy || !importProbe?.can_import}
                onclick={() => commitImport(false)}
              >
                {importBusy === 'commit' ? 'Importing...' : 'Import inactive'}
              </button>
              <button
                type="button"
                class="inline-flex items-center rounded-md bg-emerald-700 px-3 py-2 text-sm font-medium text-white ring-1 ring-emerald-700 hover:bg-emerald-800 disabled:opacity-50"
                disabled={!!importBusy || !importProbe?.can_import}
                onclick={() => commitImport(true)}
              >
                {importBusy === 'commit-active' ? 'Importing...' : 'Import and set active'}
              </button>
            </div>
          </div>

          <div class="rounded-md bg-gray-50 p-4 ring-1 ring-gray-200">
            <h2 class="text-base font-semibold text-gray-900">Probe result</h2>
            {#if importProbe}
              <dl class="mt-4 space-y-3 text-sm">
                <div><dt class="text-xs uppercase text-gray-500">Version</dt><dd class="font-medium text-gray-900">{importProbe.version_string}</dd></div>
                <div><dt class="text-xs uppercase text-gray-500">Classes</dt><dd class="font-medium text-gray-900">{importProbe.class_count}</dd></div>
                <div><dt class="text-xs uppercase text-gray-500">Properties</dt><dd class="font-medium text-gray-900">{importProbe.property_count}</dd></div>
                <div><dt class="text-xs uppercase text-gray-500">Relations</dt><dd class="font-medium text-gray-900">{importProbe.relation_count}</dd></div>
                {#if importProbe.compatible_base_versions?.length}
                  <div><dt class="text-xs uppercase text-gray-500">Compatible with</dt><dd class="font-medium text-gray-900">{importProbe.compatible_base_versions.join(', ')}</dd></div>
                {/if}
                {#if importProbe.existing_version}
                  <div class="rounded-md bg-amber-50 p-3 text-sm text-amber-800">Existing version will be replaced.</div>
                {/if}
                {#if importProbe.warnings?.length}
                  <div>
                    <dt class="text-xs uppercase text-gray-500">Warnings</dt>
                    <dd class="mt-1 space-y-1 text-amber-800">
                      {#each importProbe.warnings as warning}
                        <div>{warning}</div>
                      {/each}
                    </dd>
                  </div>
                {/if}
              </dl>
            {:else}
              <p class="mt-3 text-sm text-gray-500">Probe an RDF file to inspect counts and import impact.</p>
            {/if}
          </div>
        </div>
      </section>
    {/if}

    {#each schema.sections as section (section.id)}
      {@const SectionWidget = getOntologySectionWidget(section.widget)}
      {#if SectionWidget}
        <SectionWidget {section} {lang} {runAction} />
      {:else}
        <section class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700">
          Unknown ontology page widget: {section.widget}
        </section>
      {/if}
    {/each}
  </div>
{/if}

{#if formAction?.form_schema_url}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <button
      type="button"
      aria-label="Close"
      class="absolute inset-0 bg-black/50"
      onclick={() => (formAction = null)}
    ></button>
    <div class="relative max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-lg bg-white p-6 shadow-xl ring-1 ring-gray-200">
      <div class="mb-4 flex items-start justify-between gap-4">
        <h2 class="text-lg font-semibold text-gray-900">{tr(formAction.label, lang)}</h2>
        <button
          type="button"
          class="rounded-md px-2 py-1 text-sm text-gray-500 hover:bg-gray-100 hover:text-gray-700"
          onclick={() => (formAction = null)}
        >
          Close
        </button>
      </div>
      <FormRenderer
        schemaUrl={formAction.form_schema_url}
        {lang}
        onSuccessAction="none"
        onsuccess={handleFormSuccess}
        oncancel={() => (formAction = null)}
      />
    </div>
  </div>
{/if}
