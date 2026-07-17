<script lang="ts">
  import type { PaneOntology } from '$lib/types/ontology-pane';

  let { extension, lang = 'en', readonly = false, onDisable }: {
    extension: PaneOntology;
    lang?: string;
    readonly?: boolean;
    // Callback receives the full PaneOntology so the caller can fetch
    // extension.delete_url directly — schema-driven, no URL composition.
    onDisable?: (ext: PaneOntology) => void;
  } = $props();
</script>

<div class="flex items-center justify-between py-2">
  <div class="flex items-center gap-2 flex-1 min-w-0">
    <span class="text-sm text-gray-900 truncate">{extension.name || extension.prefix || 'Unknown'}</span>
    <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 flex-shrink-0">
      <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
        <path d="M11 17a1 1 0 001.447.894l4-2A1 1 0 0017 15V9.236a1 1 0 00-1.447-.894L11 10.766V17zM5.629 5.629c-.4-.4-1.057-.4-1.457 0-.4.4-.4 1.057 0 1.457l9.328 9.328c.4.4 1.057.4 1.457 0 .4-.4.4-1.057 0-1.457L5.629 5.629z"/>
      </svg>
      Ext
    </span>
    {#if extension.prefix}
      <span class="font-mono text-xs text-gray-500 flex-shrink-0">{extension.prefix}</span>
    {/if}
  </div>
  <div class="flex items-center gap-3 flex-shrink-0">
    <span class="text-sm text-gray-500">v{extension.version_string || '—'}</span>
    {#if !readonly && onDisable && extension.delete_url}
      <button
        type="button"
        onclick={() => onDisable?.(extension)}
        class="text-xs text-red-600 hover:text-red-800 inline-flex items-center"
        title="Disable extension"
      >
        <svg class="w-3.5 h-3.5 mr-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
        Disable
      </button>
    {/if}
  </div>
</div>
