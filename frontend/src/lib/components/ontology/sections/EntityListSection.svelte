<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import ListManager from '$lib/components/list/ListManager.svelte';
  import type { OntologySectionWidgetProps } from '../section-registry';

  let { section, lang }: OntologySectionWidgetProps = $props();
</script>

{#if section.schema_url}
  <section class="rounded-lg bg-white p-6 shadow-sm ring-1 ring-gray-200">
    {#if tr(section.title, lang)}
      <h2 class="text-base font-semibold text-gray-900">{tr(section.title, lang)}</h2>
    {/if}
    {#if tr(section.description, lang)}
      <p class="mt-1 text-sm text-gray-500">{tr(section.description, lang)}</p>
    {/if}
    <div class="mt-5">
      {#key section.schema_url}
        <ListManager schemaUrl={section.schema_url} />
      {/key}
    </div>
  </section>
{/if}
