<script lang="ts">
  import InfoSidebar from '../widgets/InfoSidebar.svelte';
  import type { ProjectOverviewWidgetProps } from '../overview-widget-registry';

  let { section }: ProjectOverviewWidgetProps = $props();
</script>

<InfoSidebar title={section.title} items={section.items} />
