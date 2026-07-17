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

  // Toc is rendered fixed-right; reserve gutter space so prose doesn't
  // collide with it on wide screens.
  const hasToc = $derived(schema.blocks.some((b) => b.type === 'toc'));
</script>

<div
  class="content-article max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-12"
  class:lg:pr-72={hasToc}
>
  {#each schema.blocks as block (block.type + '-' + Math.random())}
    {@const Widget = widgetRegistry[block.type]}
    {#if Widget}
      <div class="content-block content-block--{block.type}">
        <Widget block={block.data ?? {}} />
      </div>
    {:else}
      <!-- unknown block type — silent in prod -->
      <div class="text-sm text-red-500">unknown block: {block.type}</div>
    {/if}
  {/each}
</div>

<style>
  .content-article :global(.content-block + .content-block) {
    margin-top: 3rem;
  }
</style>
