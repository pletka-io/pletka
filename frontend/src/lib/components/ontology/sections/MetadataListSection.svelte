<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { OntologySectionWidgetProps } from '../section-registry';

  let { section, lang }: OntologySectionWidgetProps = $props();
</script>

<section class="rounded-lg bg-white p-6 shadow-sm ring-1 ring-gray-200">
  {#if tr(section.title, lang)}
    <h2 class="text-base font-semibold text-gray-900">{tr(section.title, lang)}</h2>
  {/if}
  <dl class="mt-4 grid gap-4 md:grid-cols-2">
    {#each (section.metadata ?? []).filter((m) => m.value || m.href) as item, i (`${section.id}-${i}`)}
      <div>
        <dt class="text-xs font-medium uppercase text-gray-500">{tr(item.label, lang)}</dt>
        <dd class="mt-1 text-sm text-gray-900 {item.code ? 'break-all font-mono' : ''}">
          {#if item.href}
            <a href={item.href} class="text-sky-700 hover:text-sky-800">{item.value || item.href}</a>
          {:else}
            {item.value}
          {/if}
        </dd>
      </div>
    {/each}
  </dl>
</section>
