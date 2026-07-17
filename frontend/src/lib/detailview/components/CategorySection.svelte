<!-- frontend/src/lib/detailview/components/CategorySection.svelte -->
<script lang="ts">
  import type { ViewSection } from '$lib/detailview/types';
  import type { EntityViewState } from '$lib/detailview/state.svelte';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import { isCollectionGroup } from '$lib/detailview/group-kind';
  import CollectionGroup from './CollectionGroup.svelte';

  let {
    section,
    state,
    showAddButton = false,
    onAdd,
    showEditButton = false,
    onEditGroup,
    entityType = 'model',
  }: {
    section: ViewSection;
    state: {
      expandedCategories: Set<string>;
      collapsedCollections: Set<string>;
      expandedFields: Set<string>;
      viewMode: 'compact' | 'detailed';
      projectId: string;
      toggleCategory: (id: string) => void;
      toggleCollection: (key: string) => void;
      toggleField: (id: string) => void;
    };
    showAddButton?: boolean;
    onAdd?: (sectionId: string) => void;
    showEditButton?: boolean;
    onEditGroup?: (categoryId: string, itemId: string) => void;
    entityType?: string;
  } = $props();

  const lang = getUILang();
  const isExpanded = $derived(state.expandedCategories.has(section.id));
  // Empty bucket: no id at all OR no name (no keys, or every value is
  // blank). Backend returns id:'' and name:{en:''} for the un-categorised
  // bucket; either signal is enough to treat the category as empty.
  const isEmpty = $derived(
    !section.id ||
      !section.name ||
      Object.keys(section.name).length === 0 ||
      Object.values(section.name).every((v) => !v),
  );
  const categoryName = $derived(isEmpty ? 'Uncategorized' : tr(section.name, lang, 'Unnamed Category'));

  // Collections live under a single bucket by design — when the bucket
  // is empty (no category set) the outer category-card just wraps the
  // Direct Fields card with a redundant empty header. Skip the wrapper
  // and let the Direct Fields card stand alone.
  const skipCategoryWrapper = $derived(entityType === 'collection' && isEmpty);

  const totalFields = $derived(
    section.items.reduce((sum, item) => sum + item.field_count, 0),
  );

  const collectionCount = $derived(
    section.items.filter(isCollectionGroup).length,
  );

  function toggle() {
    state.toggleCategory(section.id);
  }
</script>

{#if skipCategoryWrapper}
  <div class="space-y-2 mb-3" data-category-id={section.id}>
    {#each section.items as item (item.id)}
      <CollectionGroup
        {item}
        categoryId={section.id}
        {state}
        {showEditButton}
        {onEditGroup}
      />
    {/each}
  </div>
{:else}
<div
  class="category-card bg-white border border-gray-200 rounded-lg shadow-sm mb-3
    {isEmpty ? 'border-l-4 border-l-gray-400' : 'border-l-4 border-l-blue-500'}"
  data-category-id={section.id}
  data-category-name={categoryName}
>
  <div
    class="w-full flex items-center justify-between px-5 py-3.5 cursor-pointer hover:bg-gray-50 transition-colors text-left
      {isEmpty ? 'bg-gray-50' : 'bg-blue-50/50'}"
    onclick={toggle}
    role="button"
    tabindex="0"
    onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggle(); }}
  >
    <div class="flex items-center min-w-0">
      <svg
        class="h-5 w-5 text-gray-400 mr-3 transition-transform flex-shrink-0"
        class:rotate-90={isExpanded}
        fill="none" viewBox="0 0 24 24" stroke="currentColor"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
      </svg>
      <span class="text-base font-semibold truncate {isEmpty ? 'text-gray-700' : 'text-blue-900'}">
        {categoryName}
      </span>
    </div>

    <div class="flex items-center gap-3 flex-shrink-0 ml-3">
      <span class="text-xs text-gray-400">
        {#if collectionCount > 0}{collectionCount} coll &middot; {/if}{totalFields} fields
      </span>
      {#if showAddButton}
        <button
          type="button"
          class="inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-gray-300 bg-white hover:bg-gray-50"
          onclick={(e) => { e.stopPropagation(); onAdd?.(section.id); }}
        >
          Add
        </button>
      {/if}
    </div>
  </div>

  {#if isExpanded}
    <div class="px-5 pb-4 pt-1 space-y-2">
      {#each section.items as item (item.id)}
        <CollectionGroup
          {item}
          categoryId={section.id}
          {state}
          {showEditButton}
          {onEditGroup}
        />
      {/each}
    </div>
  {/if}
</div>
{/if}
