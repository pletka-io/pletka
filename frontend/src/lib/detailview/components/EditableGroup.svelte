<script lang="ts">
  /**
   * EditableGroup renders one item (collection-group or direct-fields)
   * with inline-editable name (when collection-group), drag-and-drop
   * field reordering, group-level move/up-down/move-to-category, and
   * the row→sidebar editing flow.
   *
   * Key DnD design:
   *
   * `entries` carries ONLY the order (each entry is `{id}` for
   * svelte-dnd-action's tracker). Field content is resolved fresh from
   * `item.fields` inside EditableFieldRow on every render. This keeps
   * dnd-action's identity tracker stable even when sidebar edits update
   * a row's display name — the field reference flows through Svelte
   * reactivity without forcing the entries array to be rebuilt.
   *
   * The sync effect only re-mounts entries when the SET of override
   * IDs changes (add/remove), preserving local order during drag and
   * surviving sibling-row edits without resetting.
   */
  import type {
    OverrideEditorCategory,
    OverrideEditorItem,
    OverrideEditorField,
  } from '$lib/detailview/override-editor-types';
  import type { OverrideEditorState } from '$lib/detailview/override-editor-state.svelte.ts';
  import { tr } from '$lib/types/weave-types';
  import { pathElementsToDisplay } from '$lib/utils/ontology-path';
  import { isCollectionGroup } from '$lib/detailview/group-kind';
  import EditableFieldRow from './EditableFieldRow.svelte';
  import PathPreview from './PathPreview.svelte';

  let {
    categoryId,
    item,
    editor,
    canEdit,
    canHide,
    canReorder,
    activeOverrideID,
    activeCollectionId,
    lang,
    categories,
    availableCategories,
    entityType,
    viewState,
    oneditrow,
    oneditcollection,
  }: {
    categoryId: string;
    item: OverrideEditorItem;
    editor: OverrideEditorState;
    canEdit: boolean;
    canHide: boolean;
    canReorder: boolean;
    activeOverrideID: number | null;
    activeCollectionId: string | null;
    lang: string;
    categories: OverrideEditorCategory[];
    availableCategories: import('$lib/detailview/override-editor-types').OverrideEditorCategoryRef[];
    /** 'model' | 'collection' — drives a few render rules. */
    entityType: string;
    viewState: import('$lib/detailview/state.svelte').EntityViewState;
    oneditrow: (overrideID: number) => void;
    oneditcollection: (categoryId: string, itemId: string) => void;
  } = $props();

  // Move dropdown reads the full project category roster directly from
  // response.available_categories — backend builds it once and every
  // dropdown in the editor consumes it. No separate fetch.
  const moveTargets = $derived<Array<{ id: string; semantic_id: string; name: Record<string, string> }>>(
    availableCategories.map((c) => ({
      id: c.id,
      semantic_id: c.semantic_id ?? '',
      name: (c.name ?? {}) as Record<string, string>,
    })),
  );

  // Reorder uses ↑↓ arrows on the row (see EditableFieldRow). Stable +
  // simple — drag-and-drop with svelte-dnd-action + Svelte 5 + nested
  // animate:flip kept misbehaving (drag clone jumping to top of viewport,
  // shadow-item key collisions). Arrows match how collection groups
  // already reorder, so the UX is consistent across the editor.
  function canMoveField(overrideID: number, delta: -1 | 1): boolean {
    const idx = item.fields.findIndex((f) => f.override_id === overrideID);
    const next = idx + delta;
    return idx >= 0 && next >= 0 && next < item.fields.length;
  }

  let showMoveMenu = $state(false);
  function moveToCategory(toCategoryID: string) {
    if (!canEdit) return;
    editor.moveItemToCategory(categoryId, item.id, toCategoryID);
    showMoveMenu = false;
  }

  // Display version of the collection's shared root path. Renders as the
  // same colored class/property pills the field rows use so curators can
  // see the ontology context the collection roots itself in.
  const collectionPrefix = $derived(
    item.shared_path_prefix?.length
      ? pathElementsToDisplay(item.shared_path_prefix)
      : [],
  );
  const isCollection = $derived(isCollectionGroup(item));

  function canMoveGroup(delta: -1 | 1): boolean {
    const cat = categories.find((c) => c.category_id === categoryId);
    if (!cat) return false;
    const idx = cat.items.findIndex((it) => it.id === item.id);
    const nextIdx = idx + delta;
    return idx >= 0 && nextIdx >= 0 && nextIdx < cat.items.length;
  }
</script>

<div class="rounded-lg border border-gray-200 bg-white">
  <!-- Group header -->
  <div
    class="px-3 py-2 flex items-center gap-2 border-b border-gray-200 {activeCollectionId === item.id ? 'bg-cyan-50/40 border-l-2 border-l-pletka-primary' : 'bg-gray-50'}"
  >
    {#if isCollection}
      <span class="inline-flex items-center w-2 h-2 rounded-full bg-emerald-500 flex-shrink-0"></span>
      <!-- Click target wraps name + id only. PathPreview emits a <div>
           which can't legally nest inside <button>; placing it as a
           sibling renders correctly and keeps the click target small. -->
      <div class="flex items-baseline gap-2 flex-1 flex-wrap min-w-0">
      <button
        type="button"
        class="inline-flex items-baseline gap-2 text-left hover:text-pletka-primary cursor-pointer"
        title="Edit collection in sidebar"
        onclick={() => oneditcollection(categoryId, item.id)}
      >
          <span class="text-sm font-semibold text-gray-800">
            {tr(item.name, lang, 'Unnamed collection')}
          </span>
          {#if item.semantic_id}
            <!-- Only render when a real collection entity backs this
                 group: a name-only override (collection_name
                 set, no backing collection) has no semantic_id, and
                 falling back to item.id — the raw grouping key, which is
                 often identical to the name — duplicated the name next
                 to itself ("time time"). -->
            <span
              class="font-mono text-[11px] text-gray-500 bg-gray-100 px-1.5 py-0.5 rounded border border-gray-200"
              title="Collection semantic ID"
            >
              {item.semantic_id}
            </span>
          {/if}
        </button>
        {#if collectionPrefix.length > 0}
          <PathPreview path={collectionPrefix} />
        {/if}
        {#if item.placement}
          <!-- Collection placement badges: constraints of
               this collection within the model. Absent placement = defaults,
               no badges. -->
          {#if item.placement.is_required}
            <span class="text-[10px] font-medium text-amber-700 bg-amber-50 border border-amber-200 px-1.5 py-0.5 rounded" title="This model requires this collection">required</span>
          {/if}
          {#if item.placement.min_occurs > 0 || item.placement.max_occurs != null}
            <span class="font-mono text-[10px] text-gray-600 bg-gray-50 border border-gray-200 px-1.5 py-0.5 rounded" title="Occurrences of this collection per model">
              {item.placement.min_occurs}..{item.placement.max_occurs ?? 'n'}
            </span>
          {/if}
          {#if item.placement.is_hidden}
            <span class="text-[10px] font-medium text-gray-500 bg-gray-100 border border-gray-200 px-1.5 py-0.5 rounded line-through" title="Hidden from the public view">Hidden</span>
          {/if}
        {/if}
      </div>
    {:else}
      <span class="inline-flex items-center w-2 h-2 rounded-full bg-blue-400 flex-shrink-0"></span>
      <div class="flex items-baseline gap-2 flex-1 flex-wrap min-w-0">
        <span class="text-sm font-semibold text-gray-700">{tr(item.name, lang, 'Direct Fields')}</span>
        {#if collectionPrefix.length > 0}
          <PathPreview path={collectionPrefix} />
        {/if}
      </div>
    {/if}
    <span class="text-xs text-gray-400 font-mono">{item.field_count} field{item.field_count === 1 ? '' : 's'}</span>
    <span class="text-xs text-gray-300">·</span>
    <button
      type="button"
      class="text-xs text-gray-400 hover:text-gray-700 disabled:opacity-30"
      title="Move group up"
      disabled={!canEdit || !canReorder || !canMoveGroup(-1)}
      onclick={() => editor.moveItem(categoryId, item.id, -1)}
    >↑</button>
    <button
      type="button"
      class="text-xs text-gray-400 hover:text-gray-700 disabled:opacity-30"
      title="Move group down"
      disabled={!canEdit || !canReorder || !canMoveGroup(1)}
      onclick={() => editor.moveItem(categoryId, item.id, 1)}
    >↓</button>
    <span class="text-xs text-gray-300">·</span>
    <div class="relative">
      <button
        type="button"
        class="text-xs text-gray-500 hover:text-pletka-primary disabled:opacity-40"
        title="Move to category"
        disabled={!canEdit}
        onclick={() => (showMoveMenu = !showMoveMenu)}
      >Move…</button>
      {#if showMoveMenu}
        <div
          class="absolute right-0 top-full mt-1 w-64 bg-white border border-gray-200 rounded shadow-lg z-20 py-1 max-h-72 overflow-y-auto"
          role="menu"
        >
          <div class="px-3 py-1 text-[10px] uppercase tracking-widest text-gray-400">Move to category</div>
          {#if moveTargets.length === 0}
            <div class="px-3 py-2 text-xs text-gray-400 italic">No categories available</div>
          {/if}
          {#each moveTargets as cat (cat.id)}
            <button
              type="button"
              class="w-full text-left px-3 py-1.5 text-xs hover:bg-gray-50 disabled:opacity-40 disabled:cursor-default flex items-center gap-2"
              disabled={cat.id === categoryId}
              onclick={() => moveToCategory(cat.id)}
            >
              <span class="flex-1 truncate">{tr(cat.name, lang, cat.semantic_id || 'Unnamed category')}</span>
              {#if cat.semantic_id}
                <span class="font-mono text-[10px] text-gray-400">{cat.semantic_id}</span>
              {/if}
              {#if cat.id === categoryId}
                <span class="text-[10px] text-gray-400">current</span>
              {/if}
            </button>
          {/each}
          <div class="border-t border-gray-100 my-1"></div>
          <button
            type="button"
            class="w-full text-left px-3 py-1.5 text-xs text-gray-400 hover:bg-gray-50"
            onclick={() => (showMoveMenu = false)}
          >Cancel</button>
        </div>
      {/if}
    </div>
  </div>

  <!-- Field rows. Reorder via ↑↓ arrows on each row, same idiom as the
       collection-group header above. -->
  <div class="divide-y divide-gray-100">
        {#each item.fields as field (field.override_id)}
      <EditableFieldRow
        {categoryId}
        itemId={item.id}
        {item}
        {field}
        {editor}
        {canEdit}
        {canHide}
        isActive={activeOverrideID === field.override_id}
        canMoveUp={canReorder && canMoveField(field.override_id, -1)}
        canMoveDown={canReorder && canMoveField(field.override_id, 1)}
        {lang}
        {viewState}
        {oneditrow}
      />
    {/each}
  </div>
</div>

<svelte:window onclick={(e) => {
  if (showMoveMenu && !(e.target as HTMLElement).closest('.relative')) {
    showMoveMenu = false;
  }
}} />
