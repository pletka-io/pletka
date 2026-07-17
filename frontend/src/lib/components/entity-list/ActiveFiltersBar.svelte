<!--
  ActiveFiltersBar — chips row that surfaces the current search query and
  any active filter values. Each chip has an "×" that dismisses that chip
  (search or a single filter), and the bar renders nothing when no chips
  are active. Reusable by any entity-list instance.
-->
<script lang="ts">
  import type { FilterConfig, FilterOption } from '$lib/types/entity-list-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    search = '',
    searchLabel = 'Search',
    filters = [],
    currentFilters = {},
    lazyOptions = {},
    lang,
    onclearsearch,
    onclearfilter,
  }: {
    search?: string;
    searchLabel?: string;
    filters?: FilterConfig[];
    currentFilters?: Record<string, string>;
    /** Optional map of filter.param_name -> lazily fetched options, for chip labels. */
    lazyOptions?: Record<string, FilterOption[] | null | undefined>;
    lang: string;
    onclearsearch: () => void;
    /** Called with the next joined value for the filter (empty string = clear). */
    onclearfilter: (paramName: string, nextValue: string) => void;
  } = $props();

  function filterLabel(filter: FilterConfig, value: string): string {
    if (!value) return '';
    const pool = (filter.options && filter.options.length > 0)
      ? filter.options
      : (lazyOptions[filter.param_name] ?? []);
    const match = pool.find((o: FilterOption) => o.value === value);
    return match ? tr(match.label, lang) : value;
  }

  function splitValues(filter: FilterConfig, raw: string): string[] {
    if (!raw) return [];
    return filter.multi ? raw.split(',').map((s) => s.trim()).filter(Boolean) : [raw];
  }

  type Chip = { filter: FilterConfig; value: string };
  const activeChips = $derived(
    filters.flatMap((f) =>
      splitValues(f, currentFilters[f.param_name] ?? '').map((value) => ({ filter: f, value }) as Chip),
    ),
  );

  function removeChip(chip: Chip) {
    const current = splitValues(chip.filter, currentFilters[chip.filter.param_name] ?? '');
    const next = current.filter((v) => v !== chip.value);
    onclearfilter(chip.filter.param_name, next.join(','));
  }

  const hasAny = $derived(Boolean(search) || activeChips.length > 0);
</script>

{#if hasAny}
  <div class="flex flex-wrap items-center gap-2 px-4 sm:px-6 py-2 text-xs bg-gray-50 border-b border-gray-200">
    <span class="text-gray-400 uppercase font-mono tracking-[0.14em]" style="font-size:10px;">Active</span>

    {#if search}
      <span class="inline-flex items-center gap-1.5 rounded-full bg-white border border-gray-200 pl-2.5 pr-1 py-0.5">
        <span class="text-gray-500">{searchLabel}:</span>
        <span class="font-medium text-gray-800">{search}</span>
        <button
          type="button"
          class="ml-0.5 rounded-full p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700"
          aria-label="Clear {searchLabel}"
          onclick={onclearsearch}
        >
          <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </span>
    {/if}

    {#each activeChips as chip (chip.filter.param_name + ':' + chip.value)}
      <span class="inline-flex items-center gap-1.5 rounded-full bg-white border border-gray-200 pl-2.5 pr-1 py-0.5">
        <span class="text-gray-500">{tr(chip.filter.label, lang)}:</span>
        <span class="font-medium text-gray-800">{filterLabel(chip.filter, chip.value)}</span>
        <button
          type="button"
          class="ml-0.5 rounded-full p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700"
          aria-label="Clear {tr(chip.filter.label, lang)} — {filterLabel(chip.filter, chip.value)}"
          onclick={() => removeChip(chip)}
        >
          <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </span>
    {/each}
  </div>
{/if}
