<!-- frontend/src/lib/components/project/widgets/ReadmeViewer.svelte -->
<script lang="ts">
  let { content = '' }: { content?: string } = $props();
</script>

<div class="bg-white rounded-lg border border-gray-200 p-6">
  {#if content}
    <div class="markdown-body whitespace-pre-wrap text-sm text-gray-800">{content}</div>
  {:else}
    <p class="text-sm italic text-gray-500">No README available</p>
  {/if}
</div>
