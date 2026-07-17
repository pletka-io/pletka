<script lang="ts">
  import type { Component } from 'svelte';
  import type { PageSchema } from '../types';

  let {
    schema,
    widgetRegistry,
  }: {
    schema: PageSchema;
    widgetRegistry: Record<string, Component>;
  } = $props();
</script>

<div class="content-hero">
  {#each schema.blocks as block (block.type + '-' + Math.random())}
    {@const Widget = widgetRegistry[block.type]}
    {#if Widget}
      <div class="content-block content-block--{block.type}">
        <Widget block={block.data ?? {}} />
      </div>
    {:else}
      <div class="text-sm text-red-500">unknown block: {block.type}</div>
    {/if}
  {/each}
</div>

<style>
  .content-hero :global(.content-block + .content-block) {
    margin-top: 4rem;
  }
  .content-hero :global(.content-block--hero) {
    padding: 0 !important;
  }
</style>
