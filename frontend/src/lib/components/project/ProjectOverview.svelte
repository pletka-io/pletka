<!-- frontend/src/lib/components/project/ProjectOverview.svelte -->
<script lang="ts">
  import type { ProjectOverviewSchema } from '$lib/types/project-page';
  import { getProjectOverviewWidget } from './overview-widget-registry';

  let { schema }: { schema: ProjectOverviewSchema } = $props();
  const lang = $derived(schema.ui?.primary_language || 'en');

  const sidebarSections = $derived(
    schema.sections.filter((s) => getProjectOverviewWidget(s.widget)?.region === 'sidebar'),
  );
  const mainSections = $derived(
    schema.sections.filter((s) => getProjectOverviewWidget(s.widget)?.region !== 'sidebar'),
  );
</script>

<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
  <div class="lg:col-span-2 space-y-6">
    {#each mainSections as section, index (`${section.widget}:${index}`)}
      {@const entry = getProjectOverviewWidget(section.widget)}
      {@const Widget = entry?.component}
      {#if Widget}
        <Widget {section} {lang} />
      {:else}
        <div class="bg-yellow-50 border border-yellow-200 rounded p-4 text-sm text-yellow-800">
          Unknown widget: {section.widget}
        </div>
      {/if}
    {/each}
  </div>

  <div class="space-y-6">
    {#each sidebarSections as section, index (`${section.widget}:${index}`)}
      {@const entry = getProjectOverviewWidget(section.widget)}
      {@const Widget = entry?.component}
      {#if Widget}
        <Widget {section} {lang} />
      {/if}
    {/each}
  </div>
</div>
