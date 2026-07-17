<!-- frontend/src/lib/components/entity-list/PathBadges.svelte -->
<!--
  Renders a field's ontology path as a chain of class/property pills with
  chevron separators. A field IS an ontology path, so this is field-specific
  content — but it lives here as a reusable piece the default EntityListRow
  mounts when row_layout.path_field is set, rather than forcing a whole
  separate row widget (which is how it drifted from the shared chrome before).
-->
<script lang="ts">
  import type { PathElement } from '$lib/types/weave-types';

  let { elements = [] }: { elements?: PathElement[] } = $props();
</script>

{#if elements.length > 0}
  <div class="flex flex-wrap items-center gap-1">
    {#each elements as element, idx}
      {#if idx > 0}
        <svg class="h-3 w-3 flex-shrink-0 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
        </svg>
      {/if}
      <span
        class="inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium
          {element.type === 'class' ? 'bg-blue-100 text-blue-800' : 'bg-green-100 text-green-800'}"
        title={element.uri}
      >
        {#if element.prefix}<span class="mr-0.5 opacity-60">{element.prefix}:</span>{/if}{element.local_name}
      </span>
    {/each}
  </div>
{/if}
