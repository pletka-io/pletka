<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { OntologySectionWidgetProps } from '../section-registry';
  import { statTone } from './shared';

  let { section, lang }: OntologySectionWidgetProps = $props();
</script>

<section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
  {#each section.stats ?? [] as stat, i (`${section.id}-${i}`)}
    <div class="rounded-lg p-4 ring-1 {statTone(stat.tone)}">
      <div class="text-xs font-medium uppercase">{tr(stat.label, lang)}</div>
      <div class="mt-1 text-2xl font-semibold">{stat.value}</div>
      {#if tr(stat.help, lang)}
        <div class="mt-1 text-xs opacity-80">{tr(stat.help, lang)}</div>
      {/if}
    </div>
  {/each}
</section>
