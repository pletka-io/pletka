<!-- frontend/src/lib/components/project/widgets/StatsCards.svelte -->
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

  const colorClasses: Record<string, string> = {
    purple: 'bg-purple-50 border-purple-100 text-purple-600 text-purple-700',
    green: 'bg-green-50 border-green-100 text-green-600 text-green-700',
    blue: 'bg-blue-50 border-blue-100 text-blue-600 text-blue-700',
    amber: 'bg-amber-50 border-amber-100 text-amber-600 text-amber-700',
    orange: 'bg-orange-50 border-orange-100 text-orange-600 text-orange-700',
  };

  function cardClass(color: string = 'gray'): string {
    return colorClasses[color] || 'bg-gray-50 border-gray-200 text-gray-600 text-gray-700';
  }
</script>

<div>
  {#if title}
    <h4 class="text-sm font-semibold text-gray-700 mb-3">{tr(title, lang, '')}</h4>
  {/if}
  <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
    {#each items as item}
      {@const classes = cardClass(item.color).split(' ')}
      <div class="border rounded-lg p-4 {classes[0]} {classes[1]}">
        <div class="flex items-center mb-2">
          <span class="text-sm font-medium {classes[2]}">{tr(item.label, lang, '')}</span>
        </div>
        <div class="text-3xl font-bold {classes[3]}">{item.count ?? 0}</div>
      </div>
    {/each}
  </div>
</div>
