<script lang="ts">
  import FormSection from '$lib/components/form/FormSection.svelte';
  import type { OverrideEditorItem } from '$lib/detailview/override-editor-types';
  import type { OverrideEditorState } from '$lib/detailview/override-editor-state.svelte';
  import { UNCATEGORIZED_ID } from '$lib/detailview/override-editor-state.svelte';
  import type { FormSchema } from '$lib/types/form-schema';
  import { tr } from '$lib/types/weave-types';
  import { confirmAction } from '$lib/stores/confirm';

  const schemaCache = new Map<string, FormSchema>();

  let {
    categoryId,
    item,
    editor,
    schemaUrl,
    lang,
    onclose,
  }: {
    categoryId: string;
    item: OverrideEditorItem;
    editor: OverrideEditorState;
    schemaUrl: string;
    lang: string;
    onclose: () => void;
  } = $props();

  let schema = $state<FormSchema | null>(null);
  let loading = $state(true);
  let errorMessage = $state('');
  let formValues = $state<Record<string, any>>({});
  let lastCommitted = $state('');

  function buildFormValues(nextItem: OverrideEditorItem, nextCategoryID: string): Record<string, any> {
    return {
      name: { ...(nextItem.name ?? {}) },
      category_id:
        nextCategoryID && nextCategoryID !== UNCATEGORIZED_ID
          ? nextCategoryID
          : '',
    };
  }

  function formSignature(values: Record<string, any>): string {
    return JSON.stringify(values);
  }

  async function loadSchema(url: string) {
    loading = true;
    errorMessage = '';
    try {
      const cached = schemaCache.get(url);
      if (cached) {
        schema = cached;
        return;
      }
      const res = await fetch(url);
      if (!res.ok) throw new Error(`Failed to load sidebar schema: ${res.status}`);
      const data = (await res.json()) as FormSchema;
      schemaCache.set(url, data);
      schema = data;
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : String(err);
      schema = null;
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void loadSchema(schemaUrl);
  });

  $effect(() => {
    const next = buildFormValues(item, categoryId);
    formValues = next;
    lastCommitted = formSignature(next);
  });

  $effect(() => {
    if (!schema) return;
    const signature = formSignature(formValues);
    if (signature === lastCommitted) return;

    const targetCategoryID = formValues.category_id ? String(formValues.category_id) : UNCATEGORIZED_ID;
    editor.updateItem(categoryId, item.id, { name: formValues.name ?? {} });
    if (targetCategoryID !== categoryId) {
      editor.moveItemToCategory(categoryId, item.id, targetCategoryID);
      onclose();
    }
    lastCommitted = signature;
  });

  // Gated by the same capability that gates every other mutation in
  // this editor (see OverrideEditor.svelte's canEditOverrides()).
  const canRemove = $derived(editor.response?.capabilities.can_edit_overrides ?? false);

  // Collection placement constraints. Local mirror of
  // item.placement; every change flows through updateItem so it rides the
  // normal draft/Save cycle. Absent placement = defaults.
  function placementOf(it: OverrideEditorItem) {
    return {
      is_required: it.placement?.is_required ?? false,
      min_occurs: it.placement?.min_occurs ?? 0,
      max_occurs: it.placement?.max_occurs ?? null,
      is_hidden: it.placement?.is_hidden ?? false,
    };
  }
  // Initialized empty; the effect below syncs from `item` on mount and on
  // every item change, so the initializer never reads reactive state.
  let constraints = $state({
    is_required: false,
    min_occurs: 0,
    max_occurs: null as number | null,
    is_hidden: false,
  });
  $effect(() => {
    constraints = placementOf(item);
  });

  function commitConstraints() {
    editor.updateItem(categoryId, item.id, {
      placement: {
        ...(item.placement ?? {}),
        is_required: constraints.is_required,
        min_occurs: Math.max(0, Math.floor(Number(constraints.min_occurs) || 0)),
        max_occurs:
          constraints.max_occurs === null || String(constraints.max_occurs) === ''
            ? null
            : Math.max(0, Math.floor(Number(constraints.max_occurs))),
        is_hidden: constraints.is_hidden,
      },
    });
  }

  async function removeCollection() {
    if (!canRemove) return;
    const name = tr(item.name, lang, item.semantic_id || item.id);
    const ok = await confirmAction({
      title: 'Remove collection',
      message: `Remove ${name} and its ${item.field_count} field${item.field_count === 1 ? '' : 's'} from this model? Base fields are not deleted — only this model's use of them.`,
      confirmLabel: 'Remove',
      danger: true,
    });
    if (!ok) return;
    editor.removeCollectionGroup(categoryId, item.id);
    onclose();
  }
</script>

<aside class="collection-sidebar fixed top-0 right-0 h-full w-[420px] bg-white border-l border-gray-200 shadow-xl z-50 flex flex-col">
  <header class="px-5 py-4 border-b border-gray-200 flex items-center gap-3">
    <div class="flex-1 min-w-0">
      <div class="text-[10px] uppercase tracking-widest text-pletka-primary font-medium">
        Collection · {item.semantic_id || item.id}
      </div>
      <h3 class="text-base font-semibold text-gray-900 truncate">{tr(item.name, lang, 'Unnamed collection')}</h3>
    </div>
    <button
      type="button"
      class="text-gray-400 hover:text-gray-700 text-lg"
      title="Close (Esc)"
      onclick={onclose}
    >✕</button>
  </header>

  <div class="flex-1 overflow-y-auto px-5 py-4 space-y-5">
    {#if loading}
      <div class="animate-pulse space-y-4 pt-2">
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
      </div>
    {:else if errorMessage}
      <div class="rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{errorMessage}</div>
    {:else if schema}
      {#each schema.sections as section (section.id)}
        <FormSection
          {section}
          bind:formValues
          {lang}
          languages={schema.ui.languages}
          errors={{}}
          dynamicOptions={{}}
          hiddenFields={new Set()}
        />
      {/each}
    {/if}

    {#if canRemove}
      <section class="border-t border-gray-100 pt-3 space-y-3">
        <h4 class="text-xs font-medium text-gray-500 uppercase tracking-wider">Constraints in this model</h4>
        <label class="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            bind:checked={constraints.is_required}
            onchange={commitConstraints}
          />
          Required — the model must have this collection
        </label>
        <div class="flex items-center gap-3 text-sm text-gray-700">
          <label class="flex items-center gap-2">
            Min
            <input
              type="number"
              min="0"
              class="w-20 rounded border-gray-300 text-sm"
              bind:value={constraints.min_occurs}
              onchange={commitConstraints}
            />
          </label>
          <label class="flex items-center gap-2">
            Max
            <input
              type="number"
              min="0"
              placeholder="∞"
              class="w-20 rounded border-gray-300 text-sm"
              bind:value={constraints.max_occurs}
              onchange={commitConstraints}
            />
          </label>
          <span class="text-xs text-gray-400">occurrences per model; empty max = unbounded</span>
        </div>
        <label class="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            bind:checked={constraints.is_hidden}
            onchange={commitConstraints}
          />
          Hide this collection in the public view
        </label>
        <p class="text-xs text-gray-400">
          Field-level min/max inside this collection apply per collection
          instance; the values above apply to the collection within this model.
        </p>
      </section>
    {/if}

    <details class="border-t border-gray-100 pt-3">
      <summary class="text-xs font-medium text-gray-500 uppercase tracking-wider cursor-pointer">Source</summary>
      <div class="mt-2 text-xs text-gray-600 space-y-1">
        <div><span class="text-gray-400">Collection ID:</span> <span class="font-mono">{item.id}</span></div>
        {#if item.semantic_id}
          <div><span class="text-gray-400">Semantic ID:</span> <span class="font-mono">{item.semantic_id}</span></div>
        {/if}
        <div><span class="text-gray-400">Position:</span> <span class="font-mono">{item.position}</span></div>
        <div><span class="text-gray-400">Field count:</span> <span class="font-mono">{item.field_count}</span></div>
        {#if item.shared_path_prefix && item.shared_path_prefix.length > 0}
          <div>
            <span class="text-gray-400">Shared root:</span>
            <span class="font-mono break-all">{item.shared_path_prefix.map((p) => p.local_name).join(' → ')}</span>
          </div>
        {/if}
      </div>
    </details>
  </div>

  <footer class="px-5 py-3 border-t border-gray-200 bg-gray-50 flex items-center justify-between gap-3">
    {#if canRemove}
      <button
        type="button"
        class="px-4 py-2 rounded-md bg-red-600 text-white text-sm font-medium hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-600 focus:ring-offset-2"
        title="Remove this collection from the model"
        onclick={removeCollection}
      >Remove</button>
    {:else}
      <span></span>
    {/if}
    <button
      type="button"
      class="px-4 py-2 rounded-md bg-pletka-primary text-white text-sm font-medium hover:bg-pletka-secondary focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
      onclick={onclose}
    >Done</button>
  </footer>
</aside>

<button
  type="button"
  class="collection-sidebar-backdrop fixed inset-0 bg-black/20 z-40"
  aria-label="Close collection sidebar"
  onclick={onclose}
></button>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') onclose(); }} />
