<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { OntologySectionWidgetProps } from '../section-registry';

  let { section, lang }: OntologySectionWidgetProps = $props();
</script>

<nav class="flex flex-wrap gap-2">
  {#each section.tabs ?? [] as tab (tab.id)}
    <a
      href={tab.href}
      class="inline-flex items-center rounded-md px-3 py-2 text-sm font-medium ring-1 transition-colors
        {tab.active ? 'bg-sky-700 text-white ring-sky-700' : 'bg-white text-gray-700 ring-gray-300 hover:bg-gray-50'}"
    >
      {tr(tab.label, lang)}
      {#if tab.count}
        <span class="ml-2 text-xs opacity-75">{tab.count}</span>
      {/if}
    </a>
  {/each}
</nav>
