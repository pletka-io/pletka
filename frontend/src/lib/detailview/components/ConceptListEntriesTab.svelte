<script lang="ts">
  import type { EntityViewState } from '$lib/detailview/state.svelte';
  import type { ConceptListSearchResult } from '$lib/detailview/types';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import { addToast } from '$lib/stores/toast';

  let {
    state: viewState,
  }: {
    state: EntityViewState;
  } = $props();

  type BroaderEdge = { id: string; concept_id: string; broader_id: string; scheme_id?: string; position?: number };

  const entries = $derived(viewState.response?.entries ?? []);
  const caps = $derived(viewState.response?.capabilities?.concept_list);
  const isClosed = $derived(Boolean(caps?.is_closed));
  const hasRemoteSource = $derived(Boolean(caps?.has_remote_source));
  const sourceName = $derived(caps?.source_name ?? '');
  // Source search is only meaningful when a remote authority backs the list;
  // a local-only list goes straight to term creation.
  const canAdd = $derived(Boolean(caps?.add_entry_url && caps?.search_entries_url) && hasRemoteSource && !isClosed);
  const updateTemplate = $derived(caps?.update_entry_url ?? '');
  const removeTemplate = $derived(caps?.remove_entry_url ?? '');
  const reorderURL = $derived(caps?.reorder_url ?? '');
  const createTermURL = $derived(caps?.create_term_url ?? '');
  const sealURL = $derived(caps?.seal_url ?? '');
  const skosURL = $derived(caps?.skos_url ?? '');
  const broaderTemplate = $derived(caps?.broader_url_template ?? '');
  const selectedEntryKeys = $derived(new Set(entries.flatMap((entry) => [entry.vocabulary_entry_id, entry.uri].filter(Boolean))));

  const jsonHeaders = { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' };

  let termLabel = $state('');
  let creatingTerm = $state(false);
  let sealing = $state(false);

  // Hierarchy (broader/narrower) editing, lazy per term.
  let hierarchyOpenID = $state('');
  let broaderCache = $state<Record<string, BroaderEdge[]>>({});
  let broaderLoading = $state('');
  let broaderChoice = $state('');
  let broaderBusy = $state('');

  let query = $state('');
  let open = $state(false);
  let searching = $state(false);
  let pending = $state(false);
  let mutatingID = $state('');
  let editingLabelID = $state('');
  let labelDraft = $state('');
  let searchError = $state('');
  let completedQuery = $state('');
  let results = $state<ConceptListSearchResult[]>([]);
  let searchToken = 0;

  function labelFor(entry: (typeof entries)[number]): string {
    return tr(entry.custom_label, getUILang(), '') || tr(entry.label, getUILang(), entry.uri || entry.vocabulary_entry_id);
  }

  function resultLabel(result: ConceptListSearchResult): string {
    return tr(result.entry.label, getUILang(), result.entry.uri || result.vocabulary_entry_id);
  }

  function resultKey(result: ConceptListSearchResult): string {
    return result.vocabulary_entry_id || result.entry.id || result.entry.uri;
  }

  function resultSelected(result: ConceptListSearchResult): boolean {
    return selectedEntryKeys.has(result.entry.uri) || selectedEntryKeys.has(result.vocabulary_entry_id ?? '') || selectedEntryKeys.has(result.entry.id ?? '');
  }

  function broaderPathLabel(entry: { broader_path?: string[]; broader_path_items?: Array<{ uri?: string; label?: Record<string, string>; id?: string; external_id?: string }> }): string {
    const lang = getUILang();
    if (entry.broader_path_items?.length) {
      return entry.broader_path_items
        .map((item) => tr(item.label, lang, item.uri || item.external_id || item.id || ''))
        .filter(Boolean)
        .join(' > ');
    }
    return entry.broader_path?.join(' > ') ?? '';
  }

  $effect(() => {
    const q = query.trim();
    if (!open || !caps?.search_entries_url) {
      results = [];
      pending = false;
      completedQuery = '';
      searchError = '';
      return;
    }
    pending = true;
    searchError = '';
    const handle = window.setTimeout(() => {
      void searchEntries(q);
    }, 250);
    return () => window.clearTimeout(handle);
  });

  async function searchEntries(q: string) {
    if (!caps?.search_entries_url) return;
    const token = ++searchToken;
    searching = true;
    pending = true;
    searchError = '';
    try {
      const url = new URL(caps.search_entries_url, window.location.origin);
      url.searchParams.set('q', q);
      url.searchParams.set('limit', '20');
      const res = await fetch(url.toString());
      if (!res.ok) throw new Error(`Search failed (${res.status})`);
      const data = await res.json();
      if (token === searchToken) {
        results = data.items ?? [];
        completedQuery = q;
      }
    } catch (err) {
      if (token === searchToken) {
        searchError = err instanceof Error ? err.message : String(err);
        results = [];
      }
    } finally {
      if (token === searchToken) {
        searching = false;
        pending = false;
      }
    }
  }

  function handleQueryInput(event: Event) {
    const next = (event.target as HTMLInputElement).value.trim();
    if (completedQuery !== next) {
      pending = true;
      searchError = '';
      results = [];
    }
  }

  async function addEntry(result: ConceptListSearchResult) {
    if (!caps?.add_entry_url) return;
    if (resultSelected(result)) return;
    const mutationKey = resultKey(result);
    mutatingID = mutationKey;
    try {
      const res = await fetch(caps.add_entry_url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Requested-With': 'XMLHttpRequest',
        },
        body: JSON.stringify({
          vocabulary_entry_id: result.vocabulary_entry_id || result.entry.id || '',
          vocabulary_entry_uri: result.entry.uri,
        }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `Add failed (${res.status})`);
      }
      addToast('success', 'Entry added');
      query = '';
      open = false;
      results = [];
      completedQuery = '';
      await viewState.init();
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to add entry');
    } finally {
      mutatingID = '';
    }
  }

  function startEditLabel(entry: (typeof entries)[number]) {
    editingLabelID = entry.id;
    labelDraft = tr(entry.custom_label, getUILang(), '');
  }

  async function saveCustomLabel(entry: (typeof entries)[number]) {
    if (!updateTemplate) return;
    const lang = getUILang();
    const customLabel = { ...(entry.custom_label ?? {}) };
    const next = labelDraft.trim();
    if (next) {
      customLabel[lang] = next;
    } else {
      delete customLabel[lang];
    }
    mutatingID = entry.id;
    try {
      const url = updateTemplate.replace('{id}', encodeURIComponent(entry.id));
      const res = await fetch(url, {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          'X-Requested-With': 'XMLHttpRequest',
        },
        body: JSON.stringify({ custom_label: customLabel }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `Update failed (${res.status})`);
      }
      addToast('success', 'Label updated');
      editingLabelID = '';
      labelDraft = '';
      await viewState.init();
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to update label');
    } finally {
      mutatingID = '';
    }
  }

  async function clearCustomLabel(entry: (typeof entries)[number]) {
    labelDraft = '';
    await saveCustomLabel(entry);
  }

  async function moveEntry(entryID: string, delta: number) {
    if (!reorderURL) return;
    const ids = entries.map((entry) => entry.id);
    const index = ids.indexOf(entryID);
    const nextIndex = index + delta;
    if (index < 0 || nextIndex < 0 || nextIndex >= ids.length) return;
    const [moved] = ids.splice(index, 1);
    ids.splice(nextIndex, 0, moved);
    mutatingID = entryID;
    try {
      const res = await fetch(reorderURL, {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          'X-Requested-With': 'XMLHttpRequest',
        },
        body: JSON.stringify({ entry_ids: ids }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `Reorder failed (${res.status})`);
      }
      await viewState.init();
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to reorder entries');
    } finally {
      mutatingID = '';
    }
  }

  async function removeEntry(entryID: string) {
    if (!removeTemplate) return;
    mutatingID = entryID;
    try {
      const url = removeTemplate.replace('{id}', encodeURIComponent(entryID));
      const res = await fetch(url, {
        method: 'DELETE',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (!res.ok && res.status !== 204) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `Remove failed (${res.status})`);
      }
      addToast('success', 'Entry removed');
      await viewState.init();
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to remove entry');
    } finally {
      mutatingID = '';
    }
  }

  async function createTerm(labelText?: string) {
    const label = (labelText ?? termLabel).trim();
    if (!createTermURL || !label) return;
    creatingTerm = true;
    try {
      const res = await fetch(createTermURL, {
        method: 'POST',
        headers: jsonHeaders,
        body: JSON.stringify({ label: { [getUILang()]: label } }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `Create failed (${res.status})`);
      }
      addToast('success', 'Term added');
      termLabel = '';
      query = '';
      open = false;
      results = [];
      completedQuery = '';
      await viewState.init();
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to add term');
    } finally {
      creatingTerm = false;
    }
  }

  async function toggleSealed() {
    if (!sealURL) return;
    sealing = true;
    try {
      const res = await fetch(sealURL, {
        method: 'PATCH',
        headers: jsonHeaders,
        body: JSON.stringify({ is_closed: !isClosed }),
      });
      if (!res.ok && res.status !== 204) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `Update failed (${res.status})`);
      }
      addToast('success', isClosed ? 'List reopened' : 'List sealed');
      await viewState.init();
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to update list');
    } finally {
      sealing = false;
    }
  }

  function entryLabelForVocabID(vocabID: string): string {
    const match = entries.find((entry) => entry.vocabulary_entry_id === vocabID);
    return match ? labelFor(match) : vocabID;
  }

  function broaderURLFor(conceptID: string): string {
    return broaderTemplate.replace('{conceptID}', encodeURIComponent(conceptID));
  }

  async function toggleHierarchy(conceptID: string) {
    if (hierarchyOpenID === conceptID) {
      hierarchyOpenID = '';
      return;
    }
    hierarchyOpenID = conceptID;
    broaderChoice = '';
    if (!broaderCache[conceptID] && broaderTemplate) await loadBroader(conceptID);
  }

  async function loadBroader(conceptID: string) {
    if (!broaderTemplate) return;
    broaderLoading = conceptID;
    try {
      const res = await fetch(broaderURLFor(conceptID));
      if (!res.ok) throw new Error(`Load failed (${res.status})`);
      const data = await res.json();
      broaderCache = { ...broaderCache, [conceptID]: Array.isArray(data) ? data : [] };
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to load hierarchy');
    } finally {
      broaderLoading = '';
    }
  }

  async function addBroader(conceptID: string) {
    if (!broaderTemplate || !broaderChoice) return;
    broaderBusy = conceptID;
    try {
      const res = await fetch(broaderURLFor(conceptID), {
        method: 'POST',
        headers: jsonHeaders,
        body: JSON.stringify({ broader_id: broaderChoice }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `Add failed (${res.status})`);
      }
      broaderChoice = '';
      await loadBroader(conceptID);
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to add broader term');
    } finally {
      broaderBusy = '';
    }
  }

  async function removeBroader(conceptID: string, edgeID: string) {
    if (!broaderTemplate) return;
    broaderBusy = conceptID;
    try {
      const res = await fetch(`${broaderURLFor(conceptID)}/${encodeURIComponent(edgeID)}`, {
        method: 'DELETE',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (!res.ok && res.status !== 204) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `Remove failed (${res.status})`);
      }
      await loadBroader(conceptID);
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Failed to remove broader term');
    } finally {
      broaderBusy = '';
    }
  }
</script>

<div class="bg-white shadow-sm rounded-lg border border-gray-200">
  <div class="border-b border-gray-200 px-6 py-4 space-y-4">
    <div class="flex items-start justify-between gap-4">
      <div>
        <div class="flex items-center gap-2">
          <h2 class="text-lg font-semibold text-gray-900">Entries</h2>
          {#if isClosed}
            <span class="rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">Sealed</span>
          {/if}
        </div>
        <p class="mt-1 text-sm text-gray-500">
          Curated concepts pinned into this controlled list. These are the values examples and future content forms can offer as dropdown choices.
        </p>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        {#if skosURL}
          <a
            href={skosURL}
            class="rounded-md border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50"
            title="Download this list as a SKOS concept scheme (Turtle)"
            download
          >
            SKOS
          </a>
        {/if}
        {#if sealURL}
          <button
            type="button"
            class="rounded-md border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50 disabled:opacity-60"
            disabled={sealing}
            onclick={toggleSealed}
            title={isClosed ? 'Reopen this list to allow adding terms' : 'Seal this list: its membership is complete and locked'}
          >
            {#if sealing}
              Working...
            {:else if isClosed}
              Reopen list
            {:else}
              Mark complete (seal)
            {/if}
          </button>
        {/if}
      </div>
    </div>

    {#if !isClosed && hasRemoteSource}
      <div class="rounded-lg border border-gray-200 bg-gray-50 p-4">
        <label class="block text-sm font-medium text-gray-700" for="concept-list-entry-search">
          Add from {sourceName || 'source vocabulary'}
        </label>
        <p class="mt-0.5 text-xs text-gray-500">Search {sourceName || 'the source vocabulary'}; if it has no match you can add the term to this project instead.</p>
        <div class="relative mt-2">
          <input
            id="concept-list-entry-search"
            class="block w-full rounded-md border-gray-300 shadow-sm focus:border-pletka-primary focus:ring-pletka-primary sm:text-sm"
            placeholder={`Search ${sourceName || 'the source vocabulary'}…`}
            bind:value={query}
            oninput={handleQueryInput}
            onfocus={() => (open = true)}
          />
        </div>
        {#if open}
          <div class="mt-3 max-h-72 divide-y divide-gray-200 overflow-y-auto rounded-md border border-gray-200 bg-white">
            {#if searching || pending || completedQuery !== query.trim()}
              <div class="px-4 py-3 text-sm text-gray-500">Searching {sourceName || 'source'}…</div>
            {:else if searchError}
              <div class="px-4 py-3 text-sm text-red-600">{searchError}</div>
            {:else if results.length === 0 && completedQuery === query.trim()}
              <div class="px-4 py-3 text-sm">
                {#if query.trim() && createTermURL}
                  <span class="text-gray-500">No match in {sourceName || 'the source'}.</span>
                  <button
                    type="button"
                    class="ml-1 font-medium text-pletka-primary hover:underline disabled:opacity-60"
                    disabled={creatingTerm}
                    onclick={() => createTerm(query)}
                  >
                    {creatingTerm ? 'Adding…' : `Add “${query.trim()}” as a new term`}
                  </button>
                {:else}
                  <span class="text-gray-400">No concepts found.</span>
                {/if}
              </div>
            {:else}
              {#each results as result (result.vocabulary_entry_id || result.entry.uri)}
                <div class="flex items-start justify-between gap-4 px-4 py-3">
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <p class="text-sm font-medium text-gray-900">{resultLabel(result)}</p>
                      {#if resultSelected(result)}
                        <span class="rounded bg-green-50 px-2 py-0.5 text-xs font-medium text-green-700">Added</span>
                      {/if}
                      {#if result.entry.external_id}
                        <span class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-500">{result.entry.external_id}</span>
                      {/if}
                    </div>
                    {#if result.entry.scope_note}
                      <p class="mt-1 line-clamp-2 text-sm text-gray-600">{tr(result.entry.scope_note, getUILang(), '')}</p>
                    {/if}
                    {#if broaderPathLabel(result.entry)}
                      <p class="mt-1 line-clamp-2 text-xs text-gray-500">
                        <span class="font-medium text-gray-600">Context:</span>
                        {broaderPathLabel(result.entry)}
                      </p>
                    {/if}
                    <p class="mt-1 truncate font-mono text-xs text-gray-500">{result.entry.uri}</p>
                  </div>
                  <button
                    type="button"
                    class="shrink-0 rounded-md bg-pletka-primary px-3 py-1.5 text-sm font-medium text-white hover:bg-pletka-primary/90 disabled:cursor-not-allowed disabled:opacity-60"
                    disabled={resultSelected(result) || mutatingID === resultKey(result)}
                    onclick={() => addEntry(result)}
                  >
                    {#if resultSelected(result)}
                      Added
                    {:else if mutatingID === resultKey(result)}
                      Adding...
                    {:else}
                      Add
                    {/if}
                  </button>
                </div>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    {#if !isClosed && !hasRemoteSource && createTermURL}
      <div class="rounded-lg border border-gray-200 bg-gray-50 p-4">
        <label class="block text-sm font-medium text-gray-700" for="concept-list-new-term">
          Add a term
        </label>
        <p class="mt-0.5 text-xs text-gray-500">This list has no external source vocabulary — terms are created directly in this project.</p>
        <div class="mt-2 flex items-center gap-2">
          <input
            id="concept-list-new-term"
            class="block w-full rounded-md border-gray-300 shadow-sm focus:border-pletka-primary focus:ring-pletka-primary sm:text-sm"
            placeholder="e.g. Female"
            bind:value={termLabel}
            onkeydown={(e) => e.key === 'Enter' && createTerm()}
          />
          <button
            type="button"
            class="shrink-0 rounded-md bg-pletka-primary px-3 py-1.5 text-sm font-medium text-white hover:bg-pletka-primary/90 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={creatingTerm || !termLabel.trim()}
            onclick={() => createTerm()}
          >
            {creatingTerm ? 'Adding…' : 'Add term'}
          </button>
        </div>
      </div>
    {/if}
  </div>

  {#if entries.length === 0}
    <div class="px-6 py-10 text-center">
      <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 text-gray-400">
        <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
        </svg>
      </div>
      <h3 class="mt-3 text-sm font-medium text-gray-900">No pinned entries</h3>
      <p class="mt-1 text-sm text-gray-500">
        This list currently relies on vocabulary search rather than a curated set of stored entries.
      </p>
    </div>
  {:else}
    <div class="divide-y divide-gray-100">
      {#each entries as entry, index (entry.id)}
        <div class="px-6 py-4">
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-sm font-semibold text-gray-900">{labelFor(entry)}</h3>
                {#if entry.external_id}
                  <span class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-500">{entry.external_id}</span>
                {/if}
              </div>
              {#if entry.scope_note}
                <p class="mt-1 text-sm text-gray-600">{tr(entry.scope_note, getUILang(), '')}</p>
              {/if}
              {#if broaderPathLabel(entry)}
                <p class="mt-1 truncate text-xs text-gray-500">{broaderPathLabel(entry)}</p>
              {/if}
              {#if entry.uri}
                <a class="mt-2 block truncate font-mono text-xs text-blue-700 hover:text-blue-900" href={entry.uri} target="_blank" rel="noreferrer">
                  {entry.uri}
                </a>
              {/if}
              {#if updateTemplate}
                <div class="mt-3">
                  {#if editingLabelID === entry.id}
                    <div class="flex max-w-xl items-center gap-2">
                      <input
                        class="block w-full rounded-md border-gray-300 text-sm shadow-sm focus:border-pletka-primary focus:ring-pletka-primary"
                        placeholder="Optional display label"
                        bind:value={labelDraft}
                      />
                      <button
                        type="button"
                        class="rounded-md bg-pletka-primary px-3 py-1.5 text-xs font-medium text-white hover:bg-pletka-primary/90 disabled:opacity-60"
                        disabled={mutatingID === entry.id}
                        onclick={() => saveCustomLabel(entry)}
                      >
                        Save
                      </button>
                      <button
                        type="button"
                        class="rounded-md border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50"
                        onclick={() => {
                          editingLabelID = '';
                          labelDraft = '';
                        }}
                      >
                        Cancel
                      </button>
                    </div>
                  {:else}
                    <div class="flex flex-wrap items-center gap-2">
                      <button
                        type="button"
                        class="text-xs font-medium text-gray-500 hover:text-gray-800"
                        onclick={() => startEditLabel(entry)}
                      >
                        {tr(entry.custom_label, getUILang(), '') ? 'Edit custom label' : 'Add custom label'}
                      </button>
                      {#if tr(entry.custom_label, getUILang(), '')}
                        <button
                          type="button"
                          class="text-xs font-medium text-gray-400 hover:text-red-600 disabled:opacity-60"
                          disabled={mutatingID === entry.id}
                          onclick={() => clearCustomLabel(entry)}
                        >
                          Use source label
                        </button>
                      {/if}
                    </div>
                  {/if}
                </div>
              {/if}
              {#if broaderTemplate && entry.vocabulary_entry_id}
                <div class="mt-3">
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md border border-gray-200 bg-white px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-50"
                    onclick={() => toggleHierarchy(entry.vocabulary_entry_id)}
                    aria-expanded={hierarchyOpenID === entry.vocabulary_entry_id}
                  >
                    <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M3 7h6l2 2h10M3 12h4M7 17h4" /></svg>
                    {hierarchyOpenID === entry.vocabulary_entry_id ? 'Hide broader / narrower' : 'Broader / narrower'}
                  </button>
                  {#if hierarchyOpenID === entry.vocabulary_entry_id}
                    <div class="mt-2 max-w-xl rounded-md border border-gray-200 bg-gray-50 p-3">
                      {#if broaderLoading === entry.vocabulary_entry_id}
                        <p class="text-xs text-gray-500">Loading...</p>
                      {:else}
                        <p class="text-xs font-medium text-gray-600">Broader than</p>
                        {#if (broaderCache[entry.vocabulary_entry_id] ?? []).length === 0}
                          <p class="mt-1 text-xs text-gray-400">No broader terms yet.</p>
                        {:else}
                          <ul class="mt-1 space-y-1">
                            {#each broaderCache[entry.vocabulary_entry_id] as edge (edge.id)}
                              <li class="flex items-center justify-between gap-2 text-xs text-gray-700">
                                <span class="truncate">{entryLabelForVocabID(edge.broader_id)}</span>
                                <button
                                  type="button"
                                  class="shrink-0 text-gray-400 hover:text-red-600 disabled:opacity-60"
                                  disabled={broaderBusy === entry.vocabulary_entry_id}
                                  onclick={() => removeBroader(entry.vocabulary_entry_id, edge.id)}
                                >
                                  Remove
                                </button>
                              </li>
                            {/each}
                          </ul>
                        {/if}
                        {#if !isClosed}
                          <div class="mt-2 flex items-center gap-2">
                            <select
                              class="block w-full rounded-md border-gray-300 text-xs shadow-sm focus:border-pletka-primary focus:ring-pletka-primary"
                              bind:value={broaderChoice}
                            >
                              <option value="">Choose a broader term...</option>
                              {#each entries.filter((other) => other.vocabulary_entry_id && other.vocabulary_entry_id !== entry.vocabulary_entry_id) as other (other.id)}
                                <option value={other.vocabulary_entry_id}>{labelFor(other)}</option>
                              {/each}
                            </select>
                            <button
                              type="button"
                              class="shrink-0 rounded-md bg-pletka-primary px-3 py-1.5 text-xs font-medium text-white hover:bg-pletka-primary/90 disabled:cursor-not-allowed disabled:opacity-60"
                              disabled={!broaderChoice || broaderBusy === entry.vocabulary_entry_id}
                              onclick={() => addBroader(entry.vocabulary_entry_id)}
                            >
                              Add
                            </button>
                          </div>
                        {/if}
                      {/if}
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <span class="rounded-full bg-gray-50 px-2 py-0.5 text-xs text-gray-500">
                #{entry.position}
              </span>
              {#if reorderURL}
                <button
                  type="button"
                  class="rounded-md border border-gray-200 px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
                  disabled={index === 0 || mutatingID === entry.id}
                  onclick={() => moveEntry(entry.id, -1)}
                >
                  Up
                </button>
                <button
                  type="button"
                  class="rounded-md border border-gray-200 px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
                  disabled={index === entries.length - 1 || mutatingID === entry.id}
                  onclick={() => moveEntry(entry.id, 1)}
                >
                  Down
                </button>
              {/if}
              {#if removeTemplate}
                <button
                  type="button"
                  class="rounded-md border border-gray-200 px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={mutatingID === entry.id}
                  onclick={() => removeEntry(entry.id)}
                >
                  {mutatingID === entry.id ? 'Removing...' : 'Remove'}
                </button>
              {/if}
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
