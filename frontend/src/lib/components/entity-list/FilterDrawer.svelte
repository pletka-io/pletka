<!--
  FilterDrawer - slide-over panel that hosts filters, sort and page size for
  a schema-driven entity list. Filter widget names are resolved through
  filter-widget-registry.ts.
-->
<script lang="ts">
  import type {
    FilterConfig,
    FilterOption,
    SortOption,
  } from '$lib/types/entity-list-schema';
  import { tr } from '$lib/types/form-schema';
  import { filterWidgetName, getFilterWidget } from './filter-widget-registry';
  import { controlID } from './control-id';

  let {
    open,
    filters = [],
    sortOptions = [],
    currentSort = '',
    currentFilters = {},
    optionsByParam = {},
    loadingByParam = {},
    errorByParam = {},
    controlPrefix = 'entity-list',
    perPage,
    pageSizeOptions,
    lang,
    onclose,
    onfilterchange,
    onsortchange,
    onperpagechange,
    onclearall,
  }: {
    open: boolean;
    filters?: FilterConfig[];
    sortOptions?: SortOption[];
    currentSort?: string;
    currentFilters?: Record<string, string>;
    optionsByParam?: Record<string, FilterOption[] | undefined>;
    loadingByParam?: Record<string, boolean>;
    errorByParam?: Record<string, string>;
    controlPrefix?: string;
    perPage?: number;
    pageSizeOptions?: number[];
    lang: string;
    onclose: () => void;
    onfilterchange: (paramName: string, value: string) => void;
    onsortchange: (value: string) => void;
    onperpagechange?: (value: number) => void;
    onclearall: () => void;
  } = $props();

  // Per-filter typeahead query (local, not sent over the wire).
  let typeaheadQuery = $state<Record<string, string>>({});

  function optionsFor(filter: FilterConfig): FilterOption[] {
    if (filter.options && filter.options.length > 0) return filter.options;
    return optionsByParam[filter.param_name] ?? [];
  }

  function selectedValues(filter: FilterConfig): string[] {
    const raw = currentFilters[filter.param_name] ?? '';
    if (raw === '') return [];
    return filter.multi ? raw.split(',').map((s) => s.trim()).filter(Boolean) : [raw];
  }

  function toggleMultiValue(filter: FilterConfig, value: string) {
    const current = selectedValues(filter);
    const next = current.includes(value)
      ? current.filter((v) => v !== value)
      : [...current, value];
    onfilterchange(filter.param_name, next.join(','));
  }

  function filteredOptions(filter: FilterConfig): FilterOption[] {
    const q = (typeaheadQuery[filter.param_name] ?? '').trim().toLowerCase();
    const opts = optionsFor(filter);
    if (!q) return opts;
    return opts.filter((o) => {
      const label = tr(o.label, lang).toLowerCase();
      return label.includes(q) || o.value.toLowerCase().includes(q);
    });
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && open) onclose();
  }

  function setTypeaheadQuery(paramName: string, value: string) {
    typeaheadQuery[paramName] = value;
    typeaheadQuery = { ...typeaheadQuery };
  }

  const sortID = $derived(controlID(controlPrefix, 'sort'));
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-40 bg-gray-900/30 transition-opacity"
    aria-hidden="true"
    onclick={onclose}
  ></div>

  <aside
    class="fixed inset-y-0 right-0 z-50 flex w-full max-w-md transform flex-col overflow-hidden bg-white shadow-xl transition-transform"
    aria-label="Filters"
  >
    <header class="flex items-center justify-between border-b border-gray-200 px-5 py-4">
      <h2 class="text-base font-semibold text-gray-900">Filters &amp; display</h2>
      <div class="flex items-center gap-3">
        <button
          type="button"
          class="text-xs font-medium text-gray-500 hover:text-gray-900"
          onclick={onclearall}
        >Clear all</button>
        <button
          type="button"
          aria-label="Close filters"
          class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600"
          onclick={onclose}
        >
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </header>

    <div class="flex-1 overflow-y-auto px-5 py-4 space-y-6">

      {#if sortOptions.length > 0}
        <section>
          <label for={sortID} class="mb-2 block text-xs font-semibold uppercase tracking-wide text-gray-500">Sort by</label>
          <select
            id={sortID}
            name="sort"
            value={currentSort}
            onchange={(e) => onsortchange((e.target as HTMLSelectElement).value)}
            class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
          >
            {#each sortOptions as opt}
              <option value={opt.value}>{tr(opt.label, lang)}</option>
            {/each}
          </select>
        </section>
      {/if}

      {#each filters as filter}
        {@const widget = filterWidgetName(filter)}
        {@const FilterWidget = getFilterWidget(widget)}
        {@const sel = selectedValues(filter)}
        <section>
          <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">
            {tr(filter.label, lang)}
          </h3>

          {#if loadingByParam[filter.param_name]}
            <div class="py-2 text-sm text-gray-400">Loading...</div>
          {:else if errorByParam[filter.param_name]}
            <div class="py-2 text-sm text-red-600">Failed to load: {errorByParam[filter.param_name]}</div>
          {:else if FilterWidget}
            <FilterWidget
              {filter}
              {lang}
              {controlPrefix}
              currentValue={currentFilters[filter.param_name] ?? ''}
              selectedValues={sel}
              options={optionsFor(filter)}
              filteredOptions={filteredOptions(filter)}
              typeaheadQuery={typeaheadQuery[filter.param_name] ?? ''}
              {onfilterchange}
              ontogglevalue={toggleMultiValue}
              onquerychange={setTypeaheadQuery}
            />
          {:else}
            <div class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
              Unknown filter widget: {widget}
            </div>
          {/if}
        </section>
      {/each}

      {#if pageSizeOptions && pageSizeOptions.length > 0 && onperpagechange}
        <section>
          <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Results per page</h3>
          <div class="flex gap-2">
            {#each pageSizeOptions as size}
              <button
                type="button"
                class="rounded-md border px-3 py-1.5 text-sm transition-colors
                  {perPage === size
                    ? 'border-pletka-primary bg-pletka-primary text-white'
                    : 'border-gray-300 text-gray-700 hover:bg-gray-50'}"
                onclick={() => onperpagechange?.(size)}
              >{size}</button>
            {/each}
          </div>
        </section>
      {/if}

    </div>

    <footer class="border-t border-gray-200 px-5 py-3">
      <button
        type="button"
        class="w-full rounded-md bg-pletka-primary px-4 py-2 text-sm font-medium text-white hover:bg-pletka-secondary"
        onclick={onclose}
      >Done</button>
    </footer>
  </aside>
{/if}
