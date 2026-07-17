<script lang="ts">
  import type { PaneAvailable } from '$lib/types/ontology-pane';

  let { available, lang = 'en', onEnable }: {
    available: PaneAvailable;
    lang?: string;
    // Callback receives the full PaneAvailable; caller posts to
    // available.enable_url. No URL composition on the frontend.
    onEnable?: (avail: PaneAvailable) => void;
  } = $props();
</script>

<div class="flex items-center justify-between py-2">
  <div class="flex items-center gap-2 flex-1 min-w-0">
    <span class="text-sm text-gray-700 truncate">{available.name || available.prefix || 'Unknown'}</span>
    {#if available.prefix}
      <span class="font-mono text-xs text-gray-500 flex-shrink-0">{available.prefix}</span>
    {/if}
  </div>
  <div class="flex items-center gap-3 flex-shrink-0">
    <span class="text-sm text-gray-500">v{available.version_string || '—'}</span>
    {#if onEnable && available.enable_url}
      <button
        type="button"
        onclick={() => onEnable?.(available)}
        class="text-xs text-green-600 hover:text-green-800 font-medium inline-flex items-center"
        title="Enable extension"
      >
        <svg class="w-3.5 h-3.5 mr-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
        </svg>
        Enable
      </button>
    {/if}
  </div>
</div>
