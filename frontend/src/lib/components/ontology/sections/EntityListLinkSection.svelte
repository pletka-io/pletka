<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { OntologySectionWidgetProps } from '../section-registry';

  let { section, lang }: OntologySectionWidgetProps = $props();
</script>

<section class="rounded-lg bg-white p-6 shadow-sm ring-1 ring-gray-200">
  <h2 class="text-base font-semibold text-gray-900">{tr(section.title, lang)}</h2>
  <div class="mt-4 flex flex-wrap gap-2">
    {#each section.links ?? [] as link (link.href)}
      <a class="inline-flex items-center rounded-md bg-sky-700 px-3 py-2 text-sm font-medium text-white hover:bg-sky-800" href={link.href}>{tr(link.label, lang)}</a>
    {/each}
  </div>
</section>
