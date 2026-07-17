<!-- frontend/src/lib/detailview/components/CollectionGroup.svelte -->
<script lang="ts">
  import type { ViewItem } from '$lib/detailview/types';
  import type { EntityViewState } from '$lib/detailview/state.svelte';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import { pathElementsToDisplay } from '$lib/utils/ontology-path';
  import { isDirectFieldGroup } from '$lib/detailview/group-kind';
  import FieldOverride from './FieldOverride.svelte';

  let {
    item,
    categoryId,
    state,
    showEditButton = false,
    onEditGroup,
  }: {
    item: ViewItem;
    categoryId: string;
    state: {
      collapsedCollections: Set<string>;
      viewMode: 'compact' | 'detailed';
      expandedFields: Set<string>;
      projectId: string;
      toggleCollection: (key: string) => void;
      toggleField: (id: string) => void;
    };
    showEditButton?: boolean;
    onEditGroup?: (categoryId: string, itemId: string) => void;
  } = $props();

  const lang = getUILang();
  const collKey = $derived(`${categoryId}-${item.id}`);
  const isExpanded = $derived(!state.collapsedCollections.has(collKey));
  const isDirectGroup = $derived(isDirectFieldGroup(item));
  const collectionName = $derived(tr(item.name, lang, isDirectGroup ? 'Direct Fields' : 'Unnamed'));

  const sharedPrefix = $derived(
    item.shared_path_prefix && item.shared_path_prefix.length > 0
      ? pathElementsToDisplay(item.shared_path_prefix)
      : [],
  );

  function toggle() {
    state.toggleCollection(collKey);
  }
</script>

{#if isDirectGroup}
  <!-- Direct-fields card: always wrapped so the visual grouping holds
       in both model and collection reads. Shared path prefix renders
       in the header so it sits next to the rows it scopes; field rows
       trim that prefix from their per-row path display. -->
  <div class="rounded-lg border border-gray-100 bg-white border-l-[3px] border-l-blue-400">
    <div class="px-4 py-2.5 flex items-center justify-between bg-blue-50/40">
      <div class="flex items-center gap-2 min-w-0">
        <span class="text-sm font-bold text-gray-800 truncate">{collectionName}</span>
        {#if sharedPrefix.length > 0}
          <div class="flex items-center gap-1 ml-2">
            {#each sharedPrefix as element, idx}
              {#if idx > 0}
                <svg class="h-3 w-3 text-gray-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                </svg>
              {/if}
              <span
                class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium
                  {element.isClass ? 'bg-blue-100 text-blue-800' : 'bg-green-100 text-green-800'}"
              >
                {element.label}
              </span>
            {/each}
          </div>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        {#if showEditButton}
          <button
            type="button"
            class="inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-gray-300 bg-white hover:bg-gray-50"
            onclick={() => onEditGroup?.(categoryId, item.id)}
          >
            Edit
          </button>
        {/if}
        <span class="inline-flex items-center justify-center w-6 h-6 rounded-full text-xs font-medium bg-gray-200 text-gray-600 flex-shrink-0">
          {item.field_count}
        </span>
      </div>
    </div>
    <div class="divide-y divide-gray-100">
      {#each item.fields as field (field.override_id)}
        <FieldOverride {field} {state} sharedPrefixLength={sharedPrefix.length} />
      {/each}
    </div>
  </div>
{:else}
  <!-- Collection group with header -->
  <div class="collection-card border border-gray-100 rounded-lg border-l-[3px] border-l-emerald-500" data-collection-name={collectionName}>
    <div
      class="w-full flex items-center justify-between px-4 py-2.5 cursor-pointer transition-colors text-left bg-green-50/40 hover:bg-green-50"
      onclick={toggle}
      role="button"
      tabindex="0"
      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggle(); }}
    >
      <div class="flex items-center min-w-0">
        <svg
          class="h-4 w-4 text-gray-400 mr-2 transition-transform flex-shrink-0"
          class:rotate-90={isExpanded}
          fill="none" viewBox="0 0 24 24" stroke="currentColor"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
        </svg>

        <span class="text-sm font-bold text-gray-800 truncate">{collectionName}</span>

        {#if item.source_url && item.id}
          <a
            href={item.source_url}
            class="ml-2 inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-emerald-50 text-emerald-800 border border-emerald-200 hover:bg-emerald-100"
            title="Open source collection"
            onclick={(e) => e.stopPropagation()}
          >
            {item.id}
            <svg class="ml-0.5 h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
          </a>
        {/if}

        {#if item.placement}
          <!-- Collection-in-model constraints. -->
          {#if item.placement.is_required}
            <span class="ml-2 inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-amber-50 text-amber-800 border border-amber-200" title="This model requires this collection">required</span>
          {/if}
          {#if item.placement.min_occurs > 0 || item.placement.max_occurs != null}
            <span class="ml-1 inline-flex items-center px-1.5 py-0.5 rounded text-xs font-mono bg-gray-50 text-gray-600 border border-gray-200" title="Occurrences of this collection per model">
              {item.placement.min_occurs}..{item.placement.max_occurs ?? 'n'}
            </span>
          {/if}
        {/if}

        {#if sharedPrefix.length > 0}
          <div class="flex items-center gap-1 ml-2">
            {#each sharedPrefix as element, idx}
              {#if idx > 0}
                <svg class="h-3 w-3 text-gray-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                </svg>
              {/if}
              <span
                class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium
                  {element.isClass ? 'bg-blue-100 text-blue-800' : 'bg-green-100 text-green-800'}"
              >
                {element.label}
              </span>
            {/each}
          </div>
        {/if}
      </div>

      <div class="flex items-center gap-2">
        {#if showEditButton}
          <button
            type="button"
            class="inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-gray-300 bg-white hover:bg-gray-50"
            onclick={(e) => { e.stopPropagation(); onEditGroup?.(categoryId, item.id); }}
          >
            Edit
          </button>
        {/if}
        <span class="inline-flex items-center justify-center w-6 h-6 rounded-full text-xs font-medium bg-gray-200 text-gray-600 flex-shrink-0 ml-2">
          {item.field_count}
        </span>
      </div>
    </div>

    {#if isExpanded}
      <div class="divide-y divide-gray-100 ml-4">
        {#each item.fields as field (field.override_id)}
          <FieldOverride {field} {state} sharedPrefixLength={sharedPrefix.length} />
        {/each}
      </div>
    {/if}
  </div>
{/if}
