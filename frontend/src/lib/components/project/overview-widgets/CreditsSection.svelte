<script lang="ts">
  import { tr } from '$lib/types/weave-types';
  import type { ProjectOverviewWidgetProps } from '../overview-widget-registry';

  let { section, lang }: ProjectOverviewWidgetProps = $props();
</script>

<div class="bg-white rounded-lg border border-gray-200 p-6">
  <h3 class="text-sm font-medium text-gray-900 mb-3">{tr(section.title, lang)}</h3>
  <div class="space-y-4">
    {#each section.credits || [] as group}
      <div>
        <h4 class="text-[11px] uppercase tracking-widest text-gray-500 font-medium mb-1">{tr(group.label, lang)}</h4>
        <ul class="space-y-1">
          {#each group.items as item}
            <li class="flex items-baseline gap-2 text-sm">
              <span class="font-medium text-gray-800">{item.actor_name}</span>
              {#if item.actor_type === 'organization'}
                <span class="text-[10px] uppercase tracking-wide text-gray-400">org</span>
              {/if}
              {#if item.note}
                <span class="text-gray-500">- {item.note}</span>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>
</div>
