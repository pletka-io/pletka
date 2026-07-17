<script lang="ts">
  import type { FieldDef, Translations } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(''),
    formValues,
    lang,
    errors = [],
  }: {
    field: FieldDef;
    value: string | null;
    formValues: Record<string, any>;
    lang: string;
    errors: string[];
  } = $props();

  type VocabularyEntryRef = {
    id?: string;
    uri?: string;
    label?: Translations;
    external_id?: string;
  };

  type VocabularyEntry = {
    id?: string;
    uri: string;
    label?: Translations;
    scope_note?: Translations;
    broader_path?: string[];
    broader_path_items?: VocabularyEntryRef[];
    external_id?: string;
  };

  let query = $state('');
  let open = $state(false);
  let loading = $state(false);
  let pending = $state(false);
  let results = $state<VocabularyEntry[]>([]);
  let selected = $state<VocabularyEntry | null>(null);
  let completedQuery = $state('');
  let searchError = $state('');
  let searchToken = 0;
  let resolveToken = 0;

  const dependenciesSatisfied = $derived((field.depends_on ?? []).every((key) => {
    const depValue = formValues[key];
    return depValue !== undefined && depValue !== null && depValue !== '';
  }));
  const selectedLabel = $derived(selected ? entryLabel(selected) : String(value ?? ''));

  $effect(() => {
    const uri = String(value ?? '').trim();
    if (!uri) {
      selected = null;
      return;
    }
    const existing = results.find((entry) => entry.uri === uri);
    if (existing) {
      selected = existing;
      return;
    }
    void resolveSelected(uri);
  });

  $effect(() => {
    const q = query.trim();
    if (!open || q.length < 2 || !field.search_url || !dependenciesSatisfied) {
      results = [];
      pending = false;
      completedQuery = '';
      searchError = '';
      return;
    }
    pending = true;
    searchError = '';
    const handle = window.setTimeout(() => {
      void search(q);
    }, 250);
    return () => window.clearTimeout(handle);
  });

  function substituteURL(template: string): string {
    return template.replace(/\{(\w+)\}/g, (_m, key) => encodeURIComponent(String(formValues[key] ?? '')));
  }

  function normaliseEntry(item: any): VocabularyEntry | null {
    const entry = item?.entry ?? item;
    if (!entry?.uri) return null;
    return {
      id: entry.id,
      uri: entry.uri,
      label: entry.label ?? { en: entry.uri },
      scope_note: entry.scope_note,
      broader_path: entry.broader_path,
      broader_path_items: entry.broader_path_items,
      external_id: entry.external_id,
    };
  }

  async function resolveSelected(uri: string) {
    const token = ++resolveToken;
    try {
      const params = new URLSearchParams({ uri, lang });
      const res = await fetch(`/api/v2/vocabulary-entries/resolve?${params.toString()}`);
      if (!res.ok || token !== resolveToken) return;
      const entry = normaliseEntry(await res.json());
      if (entry) selected = entry;
    } catch {
      if (token === resolveToken) selected = { uri, label: { en: uri } };
    }
  }

  async function search(q: string) {
    const token = ++searchToken;
    loading = true;
    pending = true;
    searchError = '';
    try {
      const url = new URL(substituteURL(field.search_url ?? ''), window.location.origin);
      url.searchParams.set('q', q);
      url.searchParams.set('limit', '20');
      url.searchParams.set('lang', lang);
      const res = await fetch(url);
      if (!res.ok) throw new Error(`Search failed (${res.status})`);
      const data = await res.json();
      const seen = new Set<string>();
      const next: VocabularyEntry[] = [];
      for (const item of data.items ?? []) {
        const entry = normaliseEntry(item);
        if (!entry || seen.has(entry.uri)) continue;
        seen.add(entry.uri);
        next.push(entry);
      }
      if (token === searchToken) {
        results = next;
        completedQuery = q;
      }
    } catch (err) {
      if (token === searchToken) {
        searchError = err instanceof Error ? err.message : 'Vocabulary search failed';
        results = [];
      }
    } finally {
      if (token === searchToken) {
        loading = false;
        pending = false;
      }
    }
  }

  function choose(entry: VocabularyEntry) {
    selected = entry;
    value = entry.uri;
    query = '';
    open = false;
    completedQuery = '';
  }

  function clear() {
    value = '';
    selected = null;
    query = '';
    results = [];
    completedQuery = '';
    searchError = '';
  }

  function handleQueryInput(event: Event) {
    const next = (event.target as HTMLInputElement).value.trim();
    if (next.length >= 2 && completedQuery !== next) {
      pending = true;
      searchError = '';
      results = [];
    }
  }

  function entryLabel(entry: VocabularyEntry): string {
    return tr(entry.label, lang) || entry.uri || entry.id || '';
  }

  function broaderPathLabel(entry: VocabularyEntry): string {
    if (entry.broader_path_items?.length) {
      return entry.broader_path_items
        .map((item) => tr(item.label, lang) || item.uri || item.external_id || item.id || '')
        .filter(Boolean)
        .join(' > ');
    }
    return entry.broader_path?.join(' > ') ?? '';
  }
</script>

<div>
  <label class="mb-1 block text-sm font-medium text-gray-700" for={field.name}>
    {tr(field.label, lang)}
    {#if field.required}<span class="ml-1 text-red-500">*</span>{/if}
  </label>

  <div class="rounded-md border bg-white shadow-sm {errors.length ? 'border-red-300' : 'border-gray-300'}">
    {#if value}
      <div class="flex items-start justify-between gap-3 border-b border-gray-100 px-3 py-2">
        <div class="min-w-0">
          <div class="truncate text-sm font-medium text-gray-900">{selectedLabel}</div>
          <div class="mt-0.5 truncate font-mono text-xs text-gray-500">{value}</div>
        </div>
        {#if !field.readonly}
          <button type="button" class="text-xs font-medium text-gray-500 hover:text-gray-700" onclick={clear}>Clear</button>
        {/if}
      </div>
    {:else if field.readonly}
      <div class="px-3 py-2 text-sm text-gray-400">Not set</div>
    {/if}

    {#if !field.readonly}
      <div class="p-2">
        <div class="flex items-center gap-2">
          <input
            id={field.name}
            type="text"
            bind:value={query}
            oninput={handleQueryInput}
            onfocus={() => (open = true)}
            placeholder={dependenciesSatisfied ? 'Search vocabulary terms...' : 'Choose a source vocabulary first'}
            disabled={!dependenciesSatisfied}
            class="block w-full rounded-md border border-gray-200 px-3 py-2 text-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary disabled:bg-gray-50 disabled:text-gray-400"
          />
          <button
            type="button"
            class="rounded-md border border-gray-200 px-2.5 py-2 text-sm text-gray-500 hover:bg-gray-50 disabled:opacity-40"
            disabled={!dependenciesSatisfied}
            onclick={() => (open = !open)}
            aria-label={open ? 'Close vocabulary search' : 'Open vocabulary search'}
          >
            v
          </button>
        </div>
      </div>
    {/if}

    {#if open && !field.readonly}
      <div class="max-h-72 overflow-y-auto border-t border-gray-100 p-1">
        {#if !dependenciesSatisfied}
          <div class="px-3 py-2 text-sm text-gray-400">Choose a source vocabulary first.</div>
        {:else if query.trim().length < 2}
          <div class="px-3 py-2 text-sm text-gray-400">Type at least 2 characters to search vocabulary terms.</div>
        {:else if loading || pending || completedQuery !== query.trim()}
          <div class="px-3 py-2 text-sm text-gray-500">Searching...</div>
        {:else if searchError}
          <div class="px-3 py-2 text-sm text-red-600">{searchError}</div>
        {:else if results.length === 0 && completedQuery === query.trim()}
          <div class="px-3 py-2 text-sm text-gray-400">No matching terms.</div>
        {:else}
          {#each results as entry (entry.uri)}
            <button
              type="button"
              class="block w-full rounded px-3 py-2 text-left hover:bg-gray-50"
              onclick={() => choose(entry)}
            >
              <div class="truncate text-sm font-medium text-gray-900">{entryLabel(entry)}</div>
              {#if tr(entry.scope_note, lang)}
                <div class="mt-0.5 line-clamp-2 text-xs text-gray-500">{tr(entry.scope_note, lang)}</div>
              {/if}
              {#if broaderPathLabel(entry)}
                <div class="mt-1 truncate text-xs text-gray-500">{broaderPathLabel(entry)}</div>
              {/if}
              <div class="mt-1 flex flex-wrap items-center gap-2">
                {#if entry.external_id}
                  <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-600">{entry.external_id}</span>
                {/if}
                <span class="truncate font-mono text-[11px] text-gray-400">{entry.uri}</span>
              </div>
            </button>
          {/each}
        {/if}
      </div>
    {/if}
  </div>

  {#if field.help && !errors.length}
    <p class="mt-1 text-sm text-gray-500">{tr(field.help, lang)}</p>
  {/if}
  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}
</div>
