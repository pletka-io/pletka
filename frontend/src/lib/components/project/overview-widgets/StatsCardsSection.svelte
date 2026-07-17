<script lang="ts">
  import StatsCards from '../widgets/StatsCards.svelte';
  import type { ProjectOverviewWidgetProps } from '../overview-widget-registry';

  let { section }: ProjectOverviewWidgetProps = $props();
</script>

<StatsCards title={section.title} items={section.items} />
