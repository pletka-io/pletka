<script lang="ts">
  import type { FieldDef, Translations } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import { warnVocabularyDegraded } from '$lib/utils/degraded-warning';

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
    /** Direct children, when the source reported it. 0 means this term cannot
     *  usefully be a parent; undefined means the count is not known. */
    narrower_total?: number;
    /** Whole-subtree size — the real size of a list scoped to this term. */
    descendants_total?: number;
    /** On a children listing: whether this child itself has children. */
    has_children?: boolean;
    /** Curated root only: browse hint + operator note. */
    browse?: string;
    note?: string;
  };

  let query = $state('');
  let open = $state(false);
  // A browse picker (roots_url present) opens its panel once on render so the
  // curated roots are visible immediately; a plain search picker stays closed
  // until focused. One-time, so the curator can still close it afterwards.
  let autoOpened = $state(false);
  let loading = $state(false);
  let pending = $state(false);
  let results = $state<VocabularyEntry[]>([]);
  let selected = $state<VocabularyEntry | null>(null);
  let completedQuery = $state('');
  let searchError = $state('');
  let searchToken = 0;
  let resolveToken = 0;

  // --- roots-first browse (#3625), active only when the schema provides a
  // roots_url. A plain value-entry picker has none and behaves exactly as
  // before. ---
  type BrowseMode = 'roots' | 'parents' | 'all';
  const browseEnabled = $derived(Boolean(field.roots_url) && !field.readonly);
  let mode = $state<BrowseMode>('roots');
  // A parent can never be a leaf, so the roots tree hides leaves by default;
  // "Show leaves" reveals them for curators who want to see the full level.
  let showLeaves = $state(false);
  let roots = $state<VocabularyEntry[]>([]);
  let rootsLoaded = $state(false);
  let rootsLoading = $state(false);
  let rootsEmpty = $state(false);
  let rootsError = $state('');
  // Breadcrumb drill: path is the ancestry the curator clicked into; level is
  // the nodes currently shown (roots when path is empty, else the last path
  // node's children).
  let path = $state<VocabularyEntry[]>([]);
  let level = $state<VocabularyEntry[]>([]);
  let levelLoading = $state(false);
  let levelToken = 0;

  function conceptIdOf(entry: VocabularyEntry): string {
    if (entry.external_id) return entry.external_id;
    const uri = String(entry.uri ?? '');
    const i = uri.lastIndexOf('/');
    return i >= 0 ? uri.slice(i + 1) : uri;
  }

  // fillTemplate substitutes {vocabulary_id} (and any other form field) from
  // formValues, plus explicit extras like {conceptId}. Unlike substituteURL it
  // is used for the browse endpoints.
  function fillTemplate(tpl: string, extra: Record<string, string> = {}): string {
    return tpl.replace(/\{(\w+)\}/g, (_m, key) =>
      encodeURIComponent(String(extra[key] ?? formValues[key] ?? '')),
    );
  }

  async function loadRoots() {
    if (!field.roots_url || !dependenciesSatisfied) return;
    rootsLoading = true;
    rootsError = '';
    try {
      const res = await fetch(fillTemplate(field.roots_url), { headers: { 'X-Requested-With': 'XMLHttpRequest' } });
      if (!res.ok) throw new Error(`Roots failed (${res.status})`);
      const data = await res.json();
      if (data.degraded) warnVocabularyDegraded(field.roots_url, tr(field.label, lang) || field.roots_url);
      const next: VocabularyEntry[] = [];
      for (const item of data.items ?? []) {
        const entry = normaliseEntry(item);
        if (entry) next.push(entry);
      }
      roots = next;
      rootsEmpty = next.length === 0;
      // No curated roots for this vocabulary (local vocab, most service mounts,
      // or a degraded fetch) → the roots tab has nothing to offer; fall back to
      // free search with parents-only filtering, and hide the Standard-roots tab.
      if (rootsEmpty && mode === 'roots') mode = 'parents';
      else resetLevel();
    } catch (err) {
      rootsError = err instanceof Error ? err.message : 'Could not load roots';
      roots = [];
      rootsEmpty = true;
      if (mode === 'roots') mode = 'parents';
    } finally {
      rootsLoading = false;
      rootsLoaded = true;
    }
  }

  function resetLevel() {
    path = [];
    level = roots;
    query = '';
  }

  async function drillInto(node: VocabularyEntry) {
    if (!field.children_url_template) return;
    const token = ++levelToken;
    levelLoading = true;
    try {
      const url = fillTemplate(field.children_url_template, { conceptId: conceptIdOf(node) });
      const res = await fetch(url, { headers: { 'X-Requested-With': 'XMLHttpRequest' } });
      if (!res.ok) throw new Error(`Children failed (${res.status})`);
      const data = await res.json();
      if (token !== levelToken) return;
      const next: VocabularyEntry[] = [];
      for (const item of data.items ?? []) {
        const entry = normaliseEntry(item);
        if (entry) next.push(entry);
      }
      path = [...path, node];
      level = next;
      query = '';
    } catch {
      // A failed drill leaves the current level in place.
    } finally {
      if (token === levelToken) levelLoading = false;
    }
  }

  function crumbTo(index: number) {
    // index -1 = back to roots; otherwise re-show the children of path[index]
    // by popping everything after it and keeping that node's already-loaded
    // children is not cached, so re-drill from the node at index.
    if (index < 0) {
      resetLevel();
      return;
    }
    const target = path[index];
    path = path.slice(0, index);
    void drillInto(target);
  }

  function expandable(entry: VocabularyEntry): boolean {
    if (entry.has_children !== undefined) return entry.has_children;
    return (entry.narrower_total ?? 0) > 0;
  }

  function scopedCount(entry: VocabularyEntry): number | undefined {
    return entry.descendants_total ?? entry.narrower_total;
  }

  // Open a browse picker's panel once, so its curated roots show on render
  // without the curator first focusing a search box (which roots mode hides).
  $effect(() => {
    if (browseEnabled && !autoOpened) {
      open = true;
      autoOpened = true;
    }
  });

  // Load roots the first time the browse picker is opened with its dependency
  // satisfied; reload when the source vocabulary changes.
  let lastVocabKey = $state('');
  $effect(() => {
    if (!browseEnabled || !open || !dependenciesSatisfied) return;
    const key = String(formValues['vocabulary_id'] ?? '');
    if (key !== lastVocabKey) {
      lastVocabKey = key;
      rootsLoaded = false;
      mode = 'roots';
      void loadRoots();
    } else if (!rootsLoaded && !rootsLoading) {
      void loadRoots();
    }
  });

  // Parents-only filters the live search results to nodes that can be a parent.
  const visibleResults = $derived(
    mode === 'parents' ? results.filter((e) => (e.narrower_total ?? 0) > 0) : results,
  );

  // Roots mode: the query filters the current browsed level by label,
  // client-side (roots and a drilled level are small, already-loaded sets).
  const visibleLevel = $derived.by(() => {
    const term = query.trim().toLowerCase();
    return level.filter((e) => {
      if (!showLeaves && !expandable(e)) return false;
      return !term || entryLabel(e).toLowerCase().includes(term);
    });
  });

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
    // In roots mode the query filters the browsed level client-side; it must
    // not fire a remote search.
    if (!open || q.length < 2 || !field.search_url || !dependenciesSatisfied || (browseEnabled && mode === 'roots')) {
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
      narrower_total: entry.narrower_total,
      descendants_total: entry.descendants_total,
      has_children: entry.has_children,
      browse: entry.browse,
      note: entry.note,
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
      const searchURL = substituteURL(field.search_url ?? '');
      const url = new URL(searchURL, window.location.origin);
      url.searchParams.set('q', q);
      url.searchParams.set('limit', '20');
      url.searchParams.set('lang', lang);
      // Parents-only drops leaves server-side (before paging), so the page
      // fills with real parents instead of being thinned by a client filter.
      if (mode === 'parents') url.searchParams.set('has_children', '1');
      const res = await fetch(url);
      if (!res.ok) throw new Error(`Search failed (${res.status})`);
      const data = await res.json();
      // Console only: the picker still works, it just has fewer suggestions,
      // and a curator has no action to take.
      if (data.degraded) warnVocabularyDegraded(searchURL, tr(field.label, lang) || searchURL);
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
      <div class="space-y-2 p-2">
        {#if browseEnabled}
          <div class="flex gap-1" role="tablist" aria-label="Browse mode">
            {#if !rootsEmpty}
              <button
                type="button"
                role="tab"
                aria-selected={mode === 'roots'}
                class="rounded px-2.5 py-1 text-xs font-medium {mode === 'roots' ? 'bg-pletka-primary text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}"
                disabled={!dependenciesSatisfied}
                onclick={() => { mode = 'roots'; open = true; }}
              >Standard roots</button>
            {/if}
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'parents'}
              class="rounded px-2.5 py-1 text-xs font-medium {mode === 'parents' ? 'bg-pletka-primary text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}"
              disabled={!dependenciesSatisfied}
              onclick={() => { mode = 'parents'; open = true; }}
            >Parents only</button>
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'all'}
              class="rounded px-2.5 py-1 text-xs font-medium {mode === 'all' ? 'bg-pletka-primary text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}"
              disabled={!dependenciesSatisfied}
              onclick={() => { mode = 'all'; open = true; }}
            >All</button>
          </div>
        {/if}
        {#if browseEnabled && mode === 'roots'}
          <label class="flex items-center gap-1.5 text-xs text-gray-600">
            <input type="checkbox" bind:checked={showLeaves} class="rounded border-gray-300 text-pletka-primary focus:ring-pletka-primary" />
            Show leaves (terms with nothing below them)
          </label>
        {/if}
        <div class="flex items-center gap-2">
          <input
            id={field.name}
            type="text"
            bind:value={query}
            oninput={handleQueryInput}
            onfocus={() => (open = true)}
            placeholder={!dependenciesSatisfied
              ? 'Choose a source vocabulary first'
              : browseEnabled && mode === 'roots'
                ? 'Filter this list...'
                : 'Search vocabulary terms...'}
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
        {:else if browseEnabled && mode === 'roots'}
          {#if path.length}
            <div class="flex flex-wrap items-center gap-1 px-2 py-1 text-xs text-gray-500">
              <button type="button" class="font-medium text-pletka-primary hover:underline" onclick={() => crumbTo(-1)}>Roots</button>
              {#each path as crumb, i}
                <span>›</span>
                <button type="button" class="truncate hover:underline" onclick={() => crumbTo(i)}>{entryLabel(crumb)}</button>
              {/each}
            </div>
          {/if}
          {#if rootsLoading && !rootsLoaded}
            <div class="px-3 py-2 text-sm text-gray-500">Loading roots...</div>
          {:else if rootsError}
            <div class="px-3 py-2 text-sm text-red-600">{rootsError}</div>
          {:else if levelLoading}
            <div class="px-3 py-2 text-sm text-gray-500">Loading...</div>
          {:else if level.length === 0}
            <div class="px-3 py-2 text-sm text-gray-400">Nothing to browse here.</div>
          {:else if visibleLevel.length === 0 && query.trim()}
            <div class="px-3 py-2 text-sm text-gray-400">No terms here match "{query.trim()}".</div>
          {:else if visibleLevel.length === 0}
            <div class="px-3 py-2 text-sm text-gray-400">Only leaf terms here — enable "Show leaves" to see them.</div>
          {:else}
            {#each visibleLevel as entry (entry.uri)}
              <div class="flex items-stretch gap-1">
                {#if expandable(entry) && field.children_url_template}
                  <button
                    type="button"
                    class="rounded px-2 text-gray-500 hover:bg-gray-100"
                    aria-label="Browse children of {entryLabel(entry)}"
                    onclick={() => drillInto(entry)}
                  >›</button>
                {:else}
                  <span class="w-7"></span>
                {/if}
                <button
                  type="button"
                  class="block min-w-0 flex-1 rounded px-2 py-2 text-left hover:bg-gray-50"
                  onclick={() => choose(entry)}
                >
                  <div class="truncate text-sm font-medium text-gray-900">{entryLabel(entry)}</div>
                  {#if entry.note}
                    <div class="mt-0.5 line-clamp-2 text-xs text-gray-500">{entry.note}</div>
                  {/if}
                  <div class="mt-1 flex flex-wrap items-center gap-2">
                    {#if scopedCount(entry) !== undefined}
                      {#if scopedCount(entry) === 0}
                        <span class="rounded bg-amber-50 px-1.5 py-0.5 text-[11px] font-medium text-amber-700" title="A list scoped to this term would be empty">empty</span>
                      {:else}
                        <span class="rounded bg-green-50 px-1.5 py-0.5 text-[11px] font-medium text-green-700">{scopedCount(entry)} terms</span>
                      {/if}
                    {/if}
                    {#if entry.external_id}
                      <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-600">{entry.external_id}</span>
                    {/if}
                  </div>
                </button>
              </div>
            {/each}
          {/if}
        {:else if query.trim().length < 2}
          <div class="px-3 py-2 text-sm text-gray-400">Type at least 2 characters to search vocabulary terms.</div>
        {:else if loading || pending || completedQuery !== query.trim()}
          <div class="px-3 py-2 text-sm text-gray-500">Searching...</div>
        {:else if searchError}
          <div class="px-3 py-2 text-sm text-red-600">{searchError}</div>
        {:else if visibleResults.length === 0 && completedQuery === query.trim()}
          <div class="px-3 py-2 text-sm text-gray-400">{mode === 'parents' ? 'No matching terms that can be a parent.' : 'No matching terms.'}</div>
        {:else}
          {#each visibleResults as entry (entry.uri)}
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
                {#if entry.narrower_total === 0}
                  <span
                    class="rounded bg-amber-50 px-1.5 py-0.5 text-[11px] font-medium text-amber-700"
                    title="Nothing sits under this term, so a list scoped to it would be empty"
                  >no narrower terms</span>
                {:else if entry.narrower_total}
                  <span class="rounded bg-green-50 px-1.5 py-0.5 text-[11px] font-medium text-green-700"
                  >{entry.narrower_total} narrower</span>
                {/if}
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
