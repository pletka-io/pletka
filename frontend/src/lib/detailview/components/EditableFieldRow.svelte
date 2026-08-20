<script lang="ts">
  /**
   * EditableFieldRow renders one override-row inline. Quick toggles
   * (required, hidden, position, remove) live on the row; deep edits
   * (display name, description, refs, occurs, visibility, set value)
   * open in OverrideSidebar via the `oneditrow` callback.
   *
   * Drives all writes through OverrideEditorState — this component
   * stays a thin presentation layer.
   */
  import type { OverrideEditorField, OverrideEditorItem } from '$lib/detailview/override-editor-types';
  import type { OverrideEditorState } from '$lib/detailview/override-editor-state.svelte.ts';
  import { tr } from '$lib/types/weave-types';
  import { pathElementsToDisplay } from '$lib/utils/ontology-path';
  import type { BadgeVariant } from '$lib/utils/field-display';
  import Badge from '$lib/components/shared/Badge.svelte';
  import PathPreview from './PathPreview.svelte';

  let {
    categoryId,
    itemId,
    item,
    field,
    editor,
    canEdit,
    canHide,
    isActive,
    canMoveUp,
    canMoveDown,
    lang,
    viewState,
    oneditrow,
  }: {
    categoryId: string;
    itemId: string;
    item: OverrideEditorItem;
    field: OverrideEditorField;
    editor: OverrideEditorState;
    canEdit: boolean;
    canHide: boolean;
    isActive: boolean;
    canMoveUp: boolean;
    canMoveDown: boolean;
    lang: string;
    /** Parent EntityViewState — supplies the sticky-header search
     *  query + match cursor. Composition editor borrows it. */
    viewState: import('$lib/detailview/state.svelte').EntityViewState;
    oneditrow: (override_id: number) => void;
  } = $props();

  const typeColors: Record<string, BadgeVariant> = {
    String: 'gray',
    URI: 'blue',
    Model: 'purple',
    Collection: 'green',
    Concept: 'indigo',
    Date: 'yellow',
    Integer: 'yellow',
    GeoJson: 'green',
    'Reference Model': 'purple',
    'Reference Collection': 'green',
  };

  const fieldName = $derived(tr(field.display_name, lang, field.field_id));
  const description = $derived(tr(field.description, lang, ''));

  // Path relative to the collection-group's shared prefix (when there
  // is one) — same trim the read-only DetailView's FieldOverride uses.
  const sharedPrefixLen = $derived(item.shared_path_prefix?.length ?? 0);
  const relativePath = $derived(
    field.path_elements?.length
      ? pathElementsToDisplay(field.path_elements).slice(sharedPrefixLen)
      : [],
  );

  // Prefer the resolved {id, semantic_id, name} refs from the response;
  // fall back to bare ID arrays when the backend hasn't surfaced names
  // (older responses or refs not in the project's catalogue).
  const refs = $derived.by(() => {
    const raw = [
      ...(field.expected_resource_model_refs?.length
        ? field.expected_resource_model_refs.map((r) => ({ kind: 'model' as const, id: r.id, semantic_id: r.semantic_id, name: r.name }))
        : (field.expected_resource_models ?? []).map((id) => ({ kind: 'model' as const, id, semantic_id: undefined, name: undefined }))),
      ...(field.expected_collection_model_refs?.length
        ? field.expected_collection_model_refs.map((r) => ({ kind: 'collection' as const, id: r.id, semantic_id: r.semantic_id, name: r.name }))
        : (field.expected_collection_models ?? []).map((id) => ({ kind: 'collection' as const, id, semantic_id: undefined, name: undefined }))),
    ];
    // Dedupe by kind:id — the same target may legitimately appear twice in the
    // resolved refs (redundant per-position override_refs), which would collide
    // the keyed {#each} below (Svelte each_key_duplicate). Keep first occurrence.
    const seen = new Set<string>();
    return raw.filter((r) => {
      const key = r.kind + ':' + r.id;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  });

  function handleEditClick(e: Event) {
    e.stopPropagation();
    oneditrow(field.override_id);
  }

  // Sticky-header search drives both views; viewState's matchSet
  // tracks whichever tree is active (read-only sections OR draft
  // categories — see OverrideEditor's setCategoriesSource).
  const isSearchActive = $derived(!!viewState.searchQuery.trim());
  const isFiltered = $derived(isSearchActive && !viewState.matchSet.has(field.override_id));
  const isActiveMatch = $derived(isSearchActive && viewState.activeOverrideID === field.override_id);
</script>

<div
  class="editable-row group flex items-start gap-2 px-3 py-2 hover:bg-gray-50 transition-colors {isActive ? 'bg-cyan-50/40 border-l-2 border-l-pletka-primary' : 'border-l-2 border-l-transparent'}"
  data-row-id={String(field.override_id)}
  data-override-id={field.override_id}
  class:hidden={isFiltered}
  class:bg-yellow-100={isActiveMatch}
>
  <!-- Position number -->
  <span class="font-mono text-[10px] text-gray-300 w-6 text-right pt-1.5 flex-shrink-0">{field.position}</span>

  <!-- Name + type + description (clickable area opens sidebar) -->
  <div
    class="flex-1 min-w-0 cursor-pointer"
    role="button"
    tabindex="0"
    onclick={handleEditClick}
    onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleEditClick(e); } }}
  >
    <div class="flex items-baseline gap-2 flex-wrap">
      <span class="font-medium text-sm text-gray-900 {field.is_hidden ? 'line-through text-gray-500' : ''}">{fieldName}</span>
      <span class="font-mono text-[10px] text-gray-400" title={field.field_id}>{field.field_id}</span>
      {#if relativePath.length > 0}
        <PathPreview path={relativePath} />
      {/if}
      {#if field.expected_value_type}
        <Badge variant={typeColors[field.expected_value_type] || 'gray'} size="xs">{field.expected_value_type}</Badge>
      {/if}
      {#if field.set_value}
        <span
          class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] bg-amber-50 text-amber-900 border border-amber-200"
          title="Fixed value (set_value)"
        >
          <span class="font-mono text-[10px] text-amber-600">=</span>
          <span class="font-mono break-all">{field.set_value}</span>
        </span>
      {/if}
      {#if refs.length > 0}
        <div class="flex flex-wrap items-center gap-1">
          {#each refs as ref (ref.kind + ':' + ref.id)}
            {@const refName = tr(ref.name, lang, '')}
            <span
              class="inline-flex items-baseline gap-1 px-1.5 py-0.5 rounded text-[11px] border break-all
                {ref.kind === 'model'
                  ? 'border-purple-200 bg-purple-50 text-purple-900'
                  : 'border-emerald-200 bg-emerald-50 text-emerald-900'}"
              title={ref.id}
            >
              <span class="font-mono">{ref.semantic_id || ref.id}</span>
              {#if refName}
                <span class="font-medium">{refName}</span>
              {/if}
            </span>
          {/each}
        </div>
      {/if}
      {#if field.is_required}
        <Badge variant="red" size="xs">Required</Badge>
      {/if}
      {#if field.is_hidden}
        <Badge variant="gray" size="xs">Hidden</Badge>
      {/if}
    </div>
    {#if description}
      <p class="text-xs text-gray-500 mt-0.5 break-words">{description}</p>
    {/if}
  </div>

  <!-- Quick toggles -->
  <div class="flex items-center gap-3 flex-shrink-0 pt-1">
    <label
      class="inline-flex items-center gap-1 text-xs text-gray-600 cursor-pointer"
      title="Required"
    >
      <input
        type="checkbox"
        class="accent-flame-500"
        checked={field.is_required}
        disabled={!canEdit}
        onclick={(e) => e.stopPropagation()}
        onchange={(e) => editor.updateField(categoryId, itemId, field.override_id, { is_required: (e.currentTarget as HTMLInputElement).checked })}
      />
      <span class="hidden sm:inline">req</span>
    </label>

    {#if canHide}
      <label
        class="inline-flex items-center gap-1 text-xs text-gray-600 cursor-pointer"
        title="Hidden"
      >
        <input
          type="checkbox"
          class="accent-flame-500"
          checked={field.is_hidden}
          disabled={!canEdit}
          onclick={(e) => e.stopPropagation()}
          onchange={(e) => editor.updateField(categoryId, itemId, field.override_id, { is_hidden: (e.currentTarget as HTMLInputElement).checked })}
        />
        <span class="hidden sm:inline">hide</span>
      </label>
    {/if}

    <!-- Reorder: ↑↓ matches the collection-group header arrows.
         Replaced the drag handle — dnd-action + Svelte 5 + nested
         animate:flip kept misbehaving. -->
    <button
      type="button"
      class="text-xs text-gray-400 hover:text-gray-700 disabled:opacity-30"
      title="Move up"
      disabled={!canEdit || !canMoveUp}
      onclick={(e) => { e.stopPropagation(); editor.moveField(categoryId, itemId, field.override_id, -1); }}
    >↑</button>
    <button
      type="button"
      class="text-xs text-gray-400 hover:text-gray-700 disabled:opacity-30"
      title="Move down"
      disabled={!canEdit || !canMoveDown}
      onclick={(e) => { e.stopPropagation(); editor.moveField(categoryId, itemId, field.override_id, 1); }}
    >↓</button>

    <!-- ✎ pencil dropped — clicking the row body already opens the
         sidebar, so the dedicated edit button was redundant. -->
    <button
      type="button"
      class="text-xs text-gray-300 hover:text-red-500 px-1"
      title="Remove override"
      disabled={!canEdit}
      onclick={(e) => { e.stopPropagation(); editor.removeField(categoryId, itemId, field.override_id); }}
    >✕</button>
  </div>
</div>
