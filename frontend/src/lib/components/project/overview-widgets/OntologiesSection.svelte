<script lang="ts">
  import { tr } from '$lib/types/weave-types';
  import type { ProjectOverviewItem } from '$lib/types/project-page';
  import type { ProjectOverviewWidgetProps } from '../overview-widget-registry';

  let { section, lang }: ProjectOverviewWidgetProps = $props();

  function inheritedProjectID(item: ProjectOverviewItem): string | undefined {
    return item.origin?.kind === 'inherited' ? item.origin.source_project_id : undefined;
  }
</script>

<div class="bg-white rounded-lg border border-gray-200 p-6">
  <h3 class="text-sm font-medium text-gray-900 mb-3">{tr(section.title, lang)}</h3>
  <div class="space-y-3">
    {#each section.items || [] as onto}
      {@const inheritedFrom = inheritedProjectID(onto)}
      <div class="py-2 border-b border-gray-100 last:border-0">
        <div class="flex items-baseline justify-between gap-3">
          <div class="min-w-0 flex-1">
            <div class="flex items-baseline gap-2 flex-wrap">
              <span class="text-sm font-medium text-gray-800">{tr(onto.label, lang)}</span>
              {#if onto.value}
                <span class="text-xs text-gray-500">v{onto.value}</span>
              {/if}
              {#if inheritedFrom}
                <a
                  href={`/projects/${inheritedFrom}`}
                  class="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium uppercase tracking-wide bg-amber-50 text-amber-800 border border-amber-200 hover:bg-amber-100"
                  title="Inherited from {inheritedFrom}"
                >
                  inherited - {inheritedFrom}
                </a>
              {/if}
            </div>
            {#if onto.subline}
              <div class="text-xs text-gray-500 mt-0.5 break-all font-mono">{onto.subline}</div>
            {/if}
          </div>
          <div class="flex-shrink-0 flex items-baseline gap-3 text-xs text-gray-500">
            {#if onto.classes_total !== undefined && onto.classes_total > 0}
              <span><span class="font-medium text-gray-700">{onto.classes_used ?? 0}/{onto.classes_total}</span> classes</span>
            {/if}
            {#if onto.properties_total !== undefined && onto.properties_total > 0}
              <span><span class="font-medium text-gray-700">{onto.properties_used ?? 0}/{onto.properties_total}</span> properties</span>
            {/if}
          </div>
        </div>
      </div>
    {/each}
  </div>
</div>
