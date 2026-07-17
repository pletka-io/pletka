<!-- frontend/src/lib/components/entity-list/PaginationBar.svelte -->
<script lang="ts">
  let {
    page,
    perPage,
    total,
    pageSizeOptions = [],
    onPageChange,
    onPerPageChange,
  }: {
    page: number;
    perPage: number;
    total: number;
    pageSizeOptions?: number[];
    onPageChange: (page: number) => void;
    onPerPageChange?: (perPage: number) => void;
  } = $props();

  const totalPages = $derived(Math.max(1, Math.ceil(total / perPage)));
  const rangeStart = $derived((page - 1) * perPage + 1);
  const rangeEnd = $derived(Math.min(page * perPage, total));

  /** Build a truncated page number list: 1 ... 4 5 [6] 7 8 ... 13 */
  const pageNumbers = $derived.by(() => {
    const pages: (number | '...')[] = [];
    if (totalPages <= 7) {
      for (let i = 1; i <= totalPages; i++) pages.push(i);
      return pages;
    }

    pages.push(1);
    if (page > 3) pages.push('...');

    const start = Math.max(2, page - 1);
    const end = Math.min(totalPages - 1, page + 1);
    for (let i = start; i <= end; i++) pages.push(i);

    if (page < totalPages - 2) pages.push('...');
    pages.push(totalPages);
    return pages;
  });
</script>

{#if total > perPage}
  <div class="flex items-center justify-between border-t border-gray-200 bg-white px-4 py-3 sm:px-6 mt-4">
    <div class="flex items-center text-sm text-gray-700">
      <span>
        Showing <span class="font-medium">{rangeStart}</span>–<span class="font-medium">{rangeEnd}</span> of <span class="font-medium">{total}</span>
      </span>

      {#if pageSizeOptions.length > 0 && onPerPageChange}
        <span class="ml-4 flex items-center gap-1.5">
          <span class="text-gray-500">Show:</span>
          {#each pageSizeOptions as size}
            <button
              type="button"
              class="px-1.5 py-0.5 text-xs rounded {size === perPage
                ? 'bg-blue-100 text-blue-700 font-medium'
                : 'text-gray-500 hover:text-gray-700 hover:bg-gray-100'}"
              onclick={() => onPerPageChange?.(size)}
            >
              {size}
            </button>
          {/each}
        </span>
      {/if}
    </div>

    <nav class="flex items-center gap-1" aria-label="Pagination">
      <button
        type="button"
        class="px-2 py-1 text-sm rounded hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed"
        disabled={page <= 1}
        onclick={() => onPageChange(page - 1)}
        aria-label="Previous page"
      >
        &lsaquo;
      </button>

      {#each pageNumbers as p}
        {#if p === '...'}
          <span class="px-2 py-1 text-sm text-gray-400">...</span>
        {:else}
          <button
            type="button"
            class="px-2.5 py-1 text-sm rounded {p === page
              ? 'bg-blue-600 text-white font-medium'
              : 'text-gray-700 hover:bg-gray-100'}"
            onclick={() => onPageChange(p)}
            aria-current={p === page ? 'page' : undefined}
          >
            {p}
          </button>
        {/if}
      {/each}

      <button
        type="button"
        class="px-2 py-1 text-sm rounded hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed"
        disabled={page >= totalPages}
        onclick={() => onPageChange(page + 1)}
        aria-label="Next page"
      >
        &rsaquo;
      </button>
    </nav>
  </div>
{/if}
