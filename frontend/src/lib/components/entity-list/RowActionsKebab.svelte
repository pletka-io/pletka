<!-- frontend/src/lib/components/entity-list/RowActionsKebab.svelte -->
<!--
  The per-row ⋮ actions menu shared by every row widget. Kept in one place so
  the lifecycle actions can't drift between widgets (they did before, when each
  widget re-implemented its own action rendering). Native <details> — no
  click-away listener; we close explicitly after an action fires.

  Callers pass the already-filtered visible actions; the summary stops click
  propagation so opening the menu doesn't trigger the row's navigate handler.
-->
<script lang="ts">
  import type { RowAction } from '$lib/types/entity-list-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    actions,
    item,
    lang,
    onAction,
  }: {
    actions: RowAction[];
    item: Record<string, any>;
    lang: string;
    onAction?: (actionId: string, item: Record<string, any>) => void;
  } = $props();

  let el: HTMLDetailsElement | undefined = $state();
</script>

<details bind:this={el} class="kebab-menu relative">
  <summary
    onclick={(e: MouseEvent) => e.stopPropagation()}
    class="flex cursor-pointer items-center rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600"
    title="Actions"
  >
    <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
      <path d="M10 6a2 2 0 110-4 2 2 0 010 4zM10 12a2 2 0 110-4 2 2 0 010 4zM10 18a2 2 0 110-4 2 2 0 010 4z" />
    </svg>
  </summary>
  <div class="absolute right-0 z-20 mt-1 w-40 rounded-md border border-gray-200 bg-white py-1 shadow-lg">
    {#each actions as action (action.id)}
      <button
        type="button"
        onclick={(e: MouseEvent) => {
          e.stopPropagation();
          if (el) el.open = false;
          onAction?.(action.id, item);
        }}
        class="block w-full px-3 py-1.5 text-left text-xs font-medium
          {action.style === 'danger'
            ? 'text-red-700 hover:bg-red-50'
            : 'text-gray-700 hover:bg-gray-100'}"
      >
        {tr(action.label, lang)}
      </button>
    {/each}
  </div>
</details>

<style>
  .kebab-menu > summary { list-style: none; }
  .kebab-menu > summary::-webkit-details-marker { display: none; }
</style>
