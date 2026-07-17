<script lang="ts">
  import type { PathElement as DisplayPathElement } from '$lib/utils/ontology-path';

  let {
    path = [],
  }: {
    path: DisplayPathElement[];
  } = $props();
</script>

{#if path.length > 0}
  <div class="flex flex-wrap items-center gap-1">
    {#each path as element, idx}
      {#if idx > 0}
        <svg class="h-3 w-3 text-gray-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
        </svg>
      {/if}
      <span
        class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium
          {element.isClass ? 'bg-blue-100 text-blue-800' : 'bg-green-100 text-green-800'}"
        title={element.raw}
      >
        {#if element.prefix}<span class="opacity-60 mr-0.5">{element.prefix}:</span>{/if}{element.localName}
      </span>
    {/each}
  </div>
{/if}
