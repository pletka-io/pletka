<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { OntologySectionWidgetProps } from '../section-registry';
  import { visibleLinks } from './shared';

  let { section, lang }: OntologySectionWidgetProps = $props();
</script>

<section class="rounded-lg bg-white p-6 shadow-sm ring-1 ring-gray-200">
  {#if tr(section.title, lang)}
    <h2 class="text-lg font-semibold text-gray-900">{tr(section.title, lang)}</h2>
  {/if}
  {#if tr(section.description, lang)}
    <p class="mt-3 max-w-4xl text-sm leading-6 text-gray-600">{tr(section.description, lang)}</p>
  {/if}
  {#if visibleLinks(section).length}
    <div class="mt-5 flex flex-wrap gap-2">
      {#each visibleLinks(section) as link (link.href)}
        <a class="inline-flex items-center rounded-md bg-white px-3 py-2 text-sm font-medium text-gray-700 ring-1 ring-gray-300 hover:bg-gray-50" href={link.href}>{tr(link.label, lang)}</a>
      {/each}
    </div>
  {/if}
</section>
