<script lang="ts">
  import { tr } from '$lib/types/weave-types';
  import type { ProjectOverviewWidgetProps } from '../overview-widget-registry';

  let { section, lang }: ProjectOverviewWidgetProps = $props();
</script>

{#if section.content_i18n}
  <div class="bg-white rounded-lg border border-gray-200 p-6">
    <p class="text-sm text-gray-700 whitespace-pre-wrap">{tr(section.content_i18n, lang)}</p>
  </div>
{/if}
