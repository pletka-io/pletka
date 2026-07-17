<script lang="ts">
  import type { Translations } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import { fade } from 'svelte/transition';

  let {
    item,
    items,
    countField,
    entityLabel,
    reassignmentEnabled = false,
    lang,
    onconfirm,
    oncancel,
  }: {
    item: any;
    items: any[];
    countField: string;
    entityLabel: Translations;
    reassignmentEnabled: boolean;
    lang: string;
    onconfirm: (reassignTo?: string) => Promise<void> | void;
    oncancel: () => void;
  } = $props();

  let reassignTo = $state('');
  let submitting = $state(false);

  let count = $derived(Number(item[countField]) || 0);
  let needsReassignment = $derived(reassignmentEnabled && count > 0);
  let siblings = $derived(items.filter((i) => i.id !== item.id));

  async function handleConfirm() {
    submitting = true;
    try {
      await onconfirm(needsReassignment ? reassignTo : undefined);
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') oncancel(); }} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="fixed inset-0 z-50 flex items-center justify-center" transition:fade={{duration: 150}}>
  <div class="fixed inset-0 bg-gray-500 bg-opacity-75" onclick={oncancel}></div>
  <div class="relative bg-white rounded-lg shadow-xl max-w-md w-full p-6">
    <h3 class="text-lg font-medium text-gray-900 mb-2">
      {needsReassignment ? 'Reassign & Delete' : 'Delete'}
    </h3>

    {#if needsReassignment}
      <p class="text-sm text-gray-600 mb-4">
        This item has {count} {tr(entityLabel, lang)} assigned. Reassign them before deleting.
      </p>
      <div class="mb-4">
        <label for="reassign-select" class="block text-sm font-medium text-gray-700 mb-1">
          Reassign {tr(entityLabel, lang)} to:
        </label>
        <select id="reassign-select" bind:value={reassignTo}
          class="block w-full rounded-md border-gray-300 shadow-sm focus:ring-pletka-primary focus:border-pletka-primary sm:text-sm">
          <option value="" disabled>Select...</option>
          {#each siblings as sib}
            <option value={sib.id}>{tr(sib.ui_name, lang) || sib.system_name || sib.id}</option>
          {/each}
        </select>
      </div>
      <p class="text-xs text-gray-500 mb-4">Model/collection overrides will also be cleared.</p>
    {:else}
      <p class="text-sm text-gray-600 mb-4">
        This will also clear any model/collection overrides using this item. This action cannot be undone.
      </p>
    {/if}

    <div class="flex justify-end space-x-3">
      <button type="button" onclick={oncancel}
        class="px-3 py-2 border border-gray-300 text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50">
        Cancel
      </button>
      <button type="button" onclick={handleConfirm}
        disabled={submitting || (needsReassignment && !reassignTo)}
        class="px-3 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 disabled:opacity-50">
        {#if submitting}
          Deleting...
        {:else}
          {needsReassignment ? 'Reassign & Delete' : 'Delete'}
        {/if}
      </button>
    </div>
  </div>
</div>
