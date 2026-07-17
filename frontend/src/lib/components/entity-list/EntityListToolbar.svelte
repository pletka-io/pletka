<script lang="ts">
  import type {
    SearchConfig,
    FilterConfig,
    ViewMode,
    EntityListCapabilities,
  } from '$lib/types/entity-list-schema';
  import { tr } from '$lib/types/form-schema';
  import { controlID } from './control-id';

  let {
    search,
    currentSearch = '',
    filters,
    currentFilters = {},
    viewModes,
    currentViewMode,
    createAction,
    adoptAction,
    entityType,
    lang,
    onsearch,
    onviewmodechange,
    onopenfilters,
    oncreate,
    onadopt,
  }: {
    search?: SearchConfig;
    currentSearch?: string;
    filters?: FilterConfig[];
    currentFilters?: Record<string, string>;
    viewModes?: ViewMode[];
    currentViewMode?: string;
    createAction?: EntityListCapabilities['create'];
    adoptAction?: EntityListCapabilities['adopt'];
    entityType: string;
    lang: string;
    onsearch: (query: string) => void;
    onviewmodechange?: (value: string) => void;
    onopenfilters: () => void;
    oncreate: () => void;
    onadopt?: () => void;
  } = $props();

  let searchQuery = $state('');
  let debounceTimer: ReturnType<typeof setTimeout>;

  $effect(() => {
    searchQuery = currentSearch;
  });

  function handleSearchInput(e: Event) {
    const target = e.target as HTMLInputElement;
    searchQuery = target.value;
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      onsearch(searchQuery);
    }, 300);
  }

  function handleViewModeClick(modeId: string) {
    if (onviewmodechange) onviewmodechange(modeId);
  }

  // Count of active filters (exclude empty values). Sort + search not counted.
  const activeFilterCount = $derived(
    (filters ?? []).reduce((n, f) => {
      const v = currentFilters[f.param_name];
      return n + (v ? 1 : 0);
    }, 0),
  );
  const controlPrefix = $derived(controlID('entity-list', entityType));
  const searchID = $derived(search ? controlID(controlPrefix, search.param_name) : '');
</script>

<div class="mb-6 flex items-center justify-between gap-3">
  <!-- Left: Search (primary) -->
  <div class="flex flex-1 items-center gap-3 min-w-0">
    {#if search}
      <div class="relative flex-1 max-w-md">
        <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
          <svg class="h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </div>
        <input
          id={searchID}
          name={search?.param_name}
          type="text"
          aria-label={tr(search.placeholder, lang)}
          placeholder={tr(search.placeholder, lang)}
          value={searchQuery}
          oninput={handleSearchInput}
          class="block w-full rounded-md border border-gray-300 py-2 pl-9 pr-3 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
        />
      </div>
    {/if}
  </div>

  <!-- Right: Filters funnel, View toggle, Create -->
  <div class="flex items-center gap-3">
    <!-- Filter drawer trigger (funnel icon) -->
    {#if (filters && filters.length > 0)}
      <button
        type="button"
        onclick={onopenfilters}
        class="relative inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-1"
        aria-label="Open filters"
      >
        <!-- funnel icon -->
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4a1 1 0 011-1h16a1 1 0 01.8 1.6L14 14v5a1 1 0 01-1.45.9l-3-1.5A1 1 0 019 17.5V14L3.2 4.6A1 1 0 013 4z" />
        </svg>
        <span>Filters</span>
        {#if activeFilterCount > 0}
          <span class="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-pletka-primary px-1.5 text-[11px] font-semibold text-white">
            {activeFilterCount}
          </span>
        {/if}
      </button>
    {/if}

    <!-- View mode toggle -->
    {#if viewModes && viewModes.length >= 2}
      <div class="flex items-center gap-1 rounded bg-gray-100 p-0.5">
        {#each viewModes as mode (mode.id)}
          <button
            type="button"
            class="rounded px-2.5 py-1 text-xs transition-colors
              {currentViewMode === mode.id
                ? 'bg-white text-gray-900 shadow-sm'
                : 'text-gray-500 hover:text-gray-700'}"
            onclick={() => handleViewModeClick(mode.id)}
          >
            {tr(mode.label, lang)}
          </button>
        {/each}
      </div>
    {/if}

    <!-- Adopt button (peer to Create) -->
    {#if adoptAction && onadopt}
      <button
        type="button"
        onclick={onadopt}
        class="inline-flex items-center rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
      >
        <svg class="-ml-1 mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        {tr(adoptAction.label, lang)}
      </button>
    {/if}

    <!-- Create button -->
    {#if createAction}
      <button
        type="button"
        onclick={oncreate}
        class="inline-flex items-center rounded-md border border-transparent bg-pletka-primary px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-pletka-primary-dark focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
      >
        <svg class="-ml-1 mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
        </svg>
        {tr(createAction.label, lang)}
      </button>
    {/if}
  </div>
</div>
