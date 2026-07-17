<!-- frontend/src/lib/components/project/widgets/InfoSidebar.svelte -->
<script lang="ts">
  import type { ProjectOverviewItem } from '$lib/types/project-page';
  import type { Translations } from '$lib/types/weave-types';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';

  let {
    title = undefined,
    items = [],
  }: {
    title?: Translations;
    items?: ProjectOverviewItem[];
  } = $props();

  const lang = getUILang();

  function formatValue(item: ProjectOverviewItem): string {
    if (!item.value) return '—';
    if (item.type === 'date') {
      try {
        return new Date(item.value).toLocaleDateString();
      } catch {
        return item.value;
      }
    }
    return item.value;
  }
</script>

<div class="bg-gray-50 rounded-lg p-4">
  {#if title}
    <h4 class="text-sm font-medium text-gray-900 mb-3">{tr(title, lang, '')}</h4>
  {/if}
  <dl class="space-y-3">
    {#each items as item}
      <div>
        {#if item.url}
          <a
            href={item.url}
            class="group flex items-center justify-between gap-3 rounded-md px-2 py-1 -mx-2 hover:bg-white"
          >
            <span class="text-sm font-medium text-gray-900 group-hover:text-pletka-primary">
              {tr(item.label, lang, '')}
            </span>
            {#if typeof item.count === 'number'}
              <span class="text-xs font-semibold text-gray-500 group-hover:text-pletka-primary">
                {item.count}
              </span>
            {/if}
          </a>
        {:else}
          <dt class="text-xs font-medium text-gray-500 uppercase tracking-wide">
            {tr(item.label, lang, '')}
          </dt>
          <dd class="mt-1 text-sm text-gray-900 {item.style === 'mono' ? 'font-mono' : ''}">
            {formatValue(item)}
          </dd>
        {/if}
      </div>
    {/each}
  </dl>
</div>
