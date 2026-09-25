<script lang="ts">
  import type { Translations } from '$lib/types/form-schema';
  import type { ExampleConceptSource, ExampleFormField } from '$lib/types/example-form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(''),
    lang = 'en',
    errors = [],
    onchoose,
  }: {
    field: ExampleFormField;
    value: string;
    lang?: string;
    errors?: string[];
    onchoose?: (selection: { uri: string; label: string }) => void;
  } = $props();

  type ConceptOption = {
    uri: string;
    label: Translations;
    scope_note?: Translations;
    broader_path?: string[];
    broader_path_items?: Array<{ uri?: string; label?: Translations; id?: string; external_id?: string }>;
    external_id?: string;
    source?: ExampleConceptSource;
  };

  let query = $state('');
  let open = $state(false);
  let loading = $state(false);
  let pending = $state(false);
  let options = $state<ConceptOption[]>([]);
  let selected = $state<ConceptOption | null>(null);
  let completedQuery = $state('');
  let searchError = $state('');
  let searchToken = 0;
  let resolveToken = 0;

  // Once per vocabulary per page load: a keystroke-driven warning would be
  // useless noise. Nothing is shown to the curator; this is for dev tools.
  const degradedWarned = new Set<string>();

  const sources = $derived(field.concept_sources ?? []);
  const selectedLabel = $derived(selected ? tr(selected.label, lang) || selected.uri : value);

  $effect(() => {
    const uri = value?.trim();
    if (!uri) {
      selected = null;
      return;
    }
    const existing = options.find((option) => option.uri === uri);
    if (existing) {
      selected = existing;
      return;
    }
    void resolveSelected(uri);
  });

  $effect(() => {
    const q = query.trim();
    if (!open) {
      options = [];
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

  async function resolveSelected(uri: string) {
    const token = ++resolveToken;
    try {
      const params = new URLSearchParams({ uri, lang });
      const res = await fetch(`/api/v2/vocabulary-entries/resolve?${params.toString()}`);
      if (!res.ok || token !== resolveToken) return;
      const data = await res.json();
      selected = {
        uri: data.uri ?? uri,
        label: data.label ?? { en: uri },
        scope_note: data.scope_note,
        broader_path: data.broader_path,
        broader_path_items: data.broader_path_items,
        external_id: data.external_id,
      };
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
      const seen = new Set<string>();
      const next: ConceptOption[] = [];
      await Promise.all(
        sources.map(async (source) => {
          const url = new URL(source.search_url, window.location.origin);
          url.searchParams.set('q', q);
          url.searchParams.set('limit', '12');
          url.searchParams.set('lang', lang);
          const res = await fetch(url);
          if (!res.ok) return;
          const data = await res.json();
          if (data.degraded && !degradedWarned.has(source.id)) {
            degradedWarned.add(source.id);
            console.warn(
              `[pletka] vocabulary lookup degraded for ${tr(source.name, lang) || source.search_url}: ` +
                'suggestions from this vocabulary are incomplete because the vocabulary service did not answer. ' +
                'This warning appears once per vocabulary per page load, so this is logged only once even though the lookup keeps failing.',
            );
          }
          for (const item of data.items ?? []) {
            const entry = item.entry ?? item;
            const uri = entry.uri;
            if (!uri || seen.has(uri)) continue;
            seen.add(uri);
            next.push({
              uri,
              label: item.custom_label && Object.keys(item.custom_label).length ? item.custom_label : (entry.label ?? { en: uri }),
              scope_note: entry.scope_note,
              broader_path: entry.broader_path,
              broader_path_items: entry.broader_path_items,
              external_id: entry.external_id,
              source,
            });
          }
        }),
      );
      if (token === searchToken) {
        options = next;
        completedQuery = q;
      }
    } catch (err) {
      if (token === searchToken) {
        searchError = err instanceof Error ? err.message : 'Concept search failed';
        options = [];
      }
    } finally {
      if (token === searchToken) {
        loading = false;
        pending = false;
      }
    }
  }

  function select(option: ConceptOption) {
    selected = option;
    value = option.uri;
    onchoose?.({ uri: option.uri, label: tr(option.label, lang) || option.uri });
    query = '';
    open = false;
    completedQuery = '';
  }

  function clear() {
    value = '';
    selected = null;
    query = '';
    options = [];
    completedQuery = '';
    searchError = '';
  }

  function handleQueryInput(event: Event) {
    const next = (event.target as HTMLInputElement).value.trim();
    if (completedQuery !== next) {
      pending = true;
      searchError = '';
      options = [];
    }
  }

  function broaderPathLabel(option: ConceptOption): string {
    if (option.broader_path_items?.length) {
      return option.broader_path_items
        .map((item) => tr(item.label, lang) || item.uri || item.external_id || item.id || '')
        .filter(Boolean)
        .join(' > ');
    }
    return option.broader_path?.join(' > ') ?? '';
  }
</script>

<div>
  <label class="mb-1 block text-sm font-medium text-gray-700" for={`concept-${field.override_id}`}>
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
        <button type="button" class="text-xs font-medium text-gray-500 hover:text-gray-700" onclick={clear}>Clear</button>
      </div>
    {/if}

    <div class="p-2">
      <div class="flex items-center gap-2">
        <input
          id={`concept-${field.override_id}`}
          type="text"
          bind:value={query}
          oninput={handleQueryInput}
          onfocus={() => (open = true)}
          placeholder={sources.length ? 'Browse terms or type to filter…' : 'No concept list configured'}
          disabled={!sources.length}
          class="block w-full rounded-md border border-gray-200 px-3 py-2 text-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary disabled:bg-gray-50 disabled:text-gray-400"
        />
        <button
          type="button"
          class="rounded-md border border-gray-200 px-2.5 py-2 text-sm text-gray-500 hover:bg-gray-50 disabled:opacity-40"
          disabled={!sources.length}
          onclick={() => (open = !open)}
          aria-label={open ? 'Close concept search' : 'Open concept search'}
        >
          v
        </button>
      </div>
    </div>

    {#if open}
      <div class="max-h-72 overflow-y-auto border-t border-gray-100 p-1">
        {#if !sources.length}
          <div class="px-3 py-2 text-sm text-gray-400">No concept list is configured for this field.</div>
        {:else if loading || pending || completedQuery !== query.trim()}
          <div class="px-3 py-2 text-sm text-gray-500">Searching…</div>
        {:else if searchError}
          <div class="px-3 py-2 text-sm text-red-600">{searchError}</div>
        {:else if options.length === 0 && completedQuery === query.trim()}
          <div class="px-3 py-2 text-sm text-gray-400">No matching concepts.</div>
        {:else}
          {#each options as option (option.uri)}
            <button
              type="button"
              class="block w-full rounded px-3 py-2 text-left hover:bg-gray-50"
              onclick={() => select(option)}
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium text-gray-900">{tr(option.label, lang) || option.uri}</div>
                  {#if tr(option.scope_note, lang)}
                    <div class="mt-0.5 line-clamp-2 text-xs text-gray-500">{tr(option.scope_note, lang)}</div>
                  {/if}
                  {#if broaderPathLabel(option)}
                    <div class="mt-1 truncate text-xs text-gray-500">{broaderPathLabel(option)}</div>
                  {/if}
                  <div class="mt-1 flex flex-wrap items-center gap-2">
                    {#if option.external_id}
                      <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-600">{option.external_id}</span>
                    {/if}
                    {#if option.source}
                      <span class="rounded bg-blue-50 px-1.5 py-0.5 text-[11px] text-blue-700">{tr(option.source.name, lang) || option.source.semantic_id || option.source.id}</span>
                    {/if}
                  </div>
                </div>
              </div>
            </button>
          {/each}
        {/if}
      </div>
    {/if}
  </div>

  {#if field.help && !errors.length}
    <p class="mt-1 text-xs text-gray-500">{tr(field.help, lang)}</p>
  {/if}
  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}
</div>
