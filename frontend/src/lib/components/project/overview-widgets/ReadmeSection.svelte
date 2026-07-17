<script lang="ts">
  import ReadmeViewer from '../widgets/ReadmeViewer.svelte';
  import type { ProjectOverviewWidgetProps } from '../overview-widget-registry';

  let { section }: ProjectOverviewWidgetProps = $props();
</script>

<ReadmeViewer content={section.content || ''} />
