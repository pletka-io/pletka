<script lang="ts">
  let {
    title = 'No items yet',
    description = '',
    actionLabel = '',
    onaction = undefined,
  }: {
    title?: string;
    description?: string;
    actionLabel?: string;
    onaction?: (() => void) | undefined;
  } = $props();
</script>

<div class="text-center py-12">
  <svg class="mx-auto h-16 w-16 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z" />
  </svg>
  <h3 class="mt-4 text-lg font-medium text-gray-900">{title}</h3>
  {#if description}
    <p class="mt-2 text-sm text-gray-500">{description}</p>
  {/if}
  {#if actionLabel && onaction}
    <button
      type="button"
      onclick={onaction}
      class="mt-4 inline-flex items-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
    >
      <svg class="-ml-1 mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
      </svg>
      {actionLabel}
    </button>
  {/if}
</div>
