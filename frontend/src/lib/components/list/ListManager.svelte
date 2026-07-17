<script lang="ts">
  import type { ListSchema, Group } from '$lib/types/list-schema';
  import { tr } from '$lib/types/form-schema';
  import { addToast } from '$lib/stores/toast';
  import { confirmAction } from '$lib/stores/confirm';
  import { dndzone } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';
  import ListRow from './ListRow.svelte';
  import FormRenderer from '../form/FormRenderer.svelte';
  import DeleteConfirm from './DeleteConfirm.svelte';
  import StatsModal from './StatsModal.svelte';

  let {
    schemaUrl,
    lang: initialLang = 'en',
    onmutate,
  }: {
    schemaUrl: string;
    lang?: string;
    onmutate?: () => void;
  } = $props();

  let schema = $state<ListSchema | null>(null);
  let items = $state<any[]>([]);
  let groups = $state<Group[] | null>(null);
  let loading = $state(true);
  let error = $state('');
  let view = $state<'list' | 'add' | 'edit'>('list');
  let editingId = $state<string | null>(null);
  let renamingId = $state<string | null>(null);
  let deletingItem = $state<any | null>(null);
  let statsId = $state<string | null>(null);
  let lang = $state('en');

  const flipDurationMs = 150;

  $effect(() => {
    lang = initialLang;
  });

  $effect(() => {
    loadSchema();
  });

  async function loadSchema() {
    loading = true;
    try {
      const res = await fetch(schemaUrl);
      if (!res.ok) throw new Error(`Failed to load schema: ${res.status}`);
      const data = await res.json();
      // Defensive: a 200 response that doesn't shape like a ListSchema must
      // not fall through to loadItems(), which would try to fetch an empty
      // or unrelated data_url and can surface misleading 404s.
      if (!data || !Array.isArray(data.columns) || typeof data.data_url !== 'string') {
        throw new Error('Server returned an unexpected list schema shape.');
      }
      schema = data;
      lang = schema!.ui.primary_language || initialLang;
      await loadItems();
    } catch (e: any) {
      error = e.message;
      schema = null;
    } finally {
      loading = false;
    }
  }

  async function loadItems() {
    if (!schema) return;
    try {
      const res = await fetch(schema.data_url);
      if (!res.ok) throw new Error(`Failed to load items: ${res.status}`);
      const data = await res.json();
      // Resolve items in this priority:
      //   1. bare JSON array → use directly
      //   2. {groups: [...]}  → grouped layout
      //   3. {[data_key]: [...]} → keyed envelope (legacy schemas)
      //   4. {items: [...]}    → generic envelope fallback
      // The bare-array path matters for slice handlers that emit
      // []rows directly (category, members, etc.); the keyed-envelope
      // path is for legacy callers still wrapping in {categories: ...}.
      if (Array.isArray(data)) {
        groups = null;
        items = data;
      } else if (Array.isArray(data?.groups)) {
        groups = data.groups as Group[];
        items = [];
      } else {
        groups = null;
        const key = schema!.data_key;
        items = (key && data[key]) || data.items || [];
      }
    } catch (e: any) {
      error = e.message;
      addToast('error', e.message);
    }
  }

  function rowIsReadonly(row: any): boolean {
    const key = schema?.per_row_readonly_field;
    return !!(key && row[key]);
  }

  // --- Reorder (optimistic) ---

  let previousOrder: any[] = [];

  function handleDndConsider(e: CustomEvent) {
    items = e.detail.items;
  }

  async function handleDndFinalize(e: CustomEvent) {
    previousOrder = [...items];
    items = e.detail.items;

    if (!schema?.capabilities.reorder) return;

    const cap = schema.capabilities.reorder;
    const ids = items.map((item) => item.id);

    try {
      const res = await fetch(cap.url, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
        body: JSON.stringify({ [cap.order_field]: ids }),
      });
      if (!res.ok) throw new Error('Failed to reorder');
      addToast('success', 'Order updated');
      onmutate?.();
    } catch {
      items = previousOrder;
      addToast('error', 'Failed to reorder');
    }
  }

  // --- Inline rename (optimistic) ---

  async function handleRename(item: any, newValue: any) {
    if (!schema?.capabilities.inline_rename) return;

    const cap = schema.capabilities.inline_rename;
    const url = cap.save_url_template.replace('{id}', item.id);
    const oldValue = item[cap.field];

    // Optimistic update
    item[cap.field] = newValue;
    items = [...items];
    renamingId = null;

    try {
      const res = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
        body: JSON.stringify({ [cap.field]: newValue }),
      });
      if (!res.ok) throw new Error('Failed to save');
      addToast('success', 'Updated');
      onmutate?.();
    } catch {
      item[cap.field] = oldValue;
      items = [...items];
      addToast('error', 'Failed to update');
    }
  }

  // --- Navigation ---

  function showAdd() { view = 'add'; }
  function showEdit(id: string) { editingId = id; view = 'edit'; }
  function backToList() { view = 'list'; editingId = null; }

  async function handleFormSuccess() {
    await loadItems();
    onmutate?.();
    backToList();
  }

  // --- Row actions ---

  function handleRowAction(actionId: string, item: any) {
    switch (actionId) {
      case 'edit': showEdit(item.id); break;
      case 'stats': handleStats(item.id); break;
      case 'delete': handleDelete(item); break;
      case 'deprecate':
      case 'activate':
        handleLifecycleAction(actionId, item);
        break;
      default:
        // Generic dispatch for actions that carry url_template + method
        // directly on the schema (e.g. future custom actions).
        handleSchemaAction(actionId, item);
    }
  }

  // Resolves the per-row action's URL template and method from the schema.
  // Used for lifecycle actions (deprecate/activate) and any future server-
  // declared row action that doesn't have a built-in handler.
  function findRowAction(actionId: string) {
    return schema?.row_actions?.find((a) => a.id === actionId);
  }

  async function handleLifecycleAction(actionId: 'deprecate' | 'activate', item: any) {
    const action = findRowAction(actionId);
    if (!action?.url_template) return;

    const url = action.url_template.replace('{id}', item.id);
    const method = (action.method || 'POST').toUpperCase();
    try {
      const res = await fetch(url, {
        method,
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (res.status === 204 || res.status === 200) {
        addToast('success', actionId === 'deprecate' ? 'Deprecated' : 'Activated');
        await loadItems();
        onmutate?.();
      } else {
        addToast('error', `Failed to ${actionId}`);
      }
    } catch {
      addToast('error', `Failed to ${actionId}`);
    }
  }

  async function handleSchemaAction(actionId: string, item: any) {
    const action = findRowAction(actionId);
    if (!action?.url_template) return;

    const url = action.url_template.replace('{id}', item.id);
    const method = (action.method || 'POST').toUpperCase();
    try {
      const res = await fetch(url, {
        method,
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (res.ok) {
        await loadItems();
        onmutate?.();
      } else {
        addToast('error', `Action ${actionId} failed`);
      }
    } catch {
      addToast('error', `Action ${actionId} failed`);
    }
  }

  function handleStats(id: string) {
    statsId = id;
  }

  // --- Delete flow ---

  async function handleDelete(item: any) {
    if (!schema?.capabilities.delete?.reassignment) {
      const ok = await confirmAction({
        title: 'Delete item',
        message: 'This action cannot be undone.',
        confirmLabel: 'Delete',
        cancelLabel: 'Cancel',
        danger: true,
      });
      if (!ok) return;
      deletingItem = item;
      await confirmDelete();
      return;
    }
    deletingItem = item;
  }

  async function confirmDelete(reassignTo?: string) {
    if (!schema?.capabilities.delete || !deletingItem) return;

    const cap = schema.capabilities.delete;
    const url = cap.url_template.replace('{id}', deletingItem.id);

    try {
      const options: RequestInit = {
        method: 'DELETE',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      };
      if (reassignTo) {
        options.headers = { ...options.headers, 'Content-Type': 'application/json' } as any;
        options.body = JSON.stringify({ reassign_fields_to: reassignTo });
      }

      const res = await fetch(url, options);

      if (res.status === 204) {
        addToast('success', reassignTo ? 'Deleted and reassigned' : 'Deleted');
        deletingItem = null;
        await loadItems();
        onmutate?.();
      } else if (res.status === 409) {
        const data = await res.json();
        // Two 409 shapes:
        //   1. Slice "in use by overrides" — entity_type + entity_id +
        //      semantic_id present. Cannot delete; user must deprecate
        //      first or pick a reassignment target.
        //   2. Legacy "has fields assigned" — field_count present, modal
        //      stays open to surface the reassignment picker.
        if (data.entity_type) {
          addToast(
            'warning',
            `Cannot delete — ${data.entity_type} is in use by overrides. Use Deprecate instead.`,
          );
        } else if (typeof data.field_count === 'number') {
          addToast('warning', `Has ${data.field_count} fields assigned`);
        } else {
          addToast('warning', data.error || 'Cannot delete');
        }
      } else {
        addToast('error', 'Failed to delete');
      }
    } catch {
      addToast('error', 'Failed to delete');
    }
  }

  function cancelDelete() {
    deletingItem = null;
  }
</script>

{#if loading}
  <div class="animate-pulse space-y-4">
    <div class="h-6 bg-gray-200 rounded w-1/4"></div>
    <div class="h-10 bg-gray-200 rounded"></div>
    <div class="h-10 bg-gray-200 rounded"></div>
    <div class="h-10 bg-gray-200 rounded"></div>
  </div>
{:else if error && !schema}
  <div class="text-red-600 p-4">{error}</div>
{:else if schema}
  {#if view === 'list'}
    <!-- Toolbar -->
    <div class="flex items-center justify-between mb-6">
      <h3 class="text-lg leading-6 font-medium text-gray-900">
        {tr(schema.title, lang)}
      </h3>
      {#if schema.capabilities.create}
        <button type="button" onclick={showAdd}
          class="inline-flex items-center px-3 py-2 border border-transparent text-sm leading-4 font-medium rounded-md text-white bg-pletka-primary hover:bg-pletka-secondary focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-pletka-primary">
          <svg class="w-4 h-4 mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/>
          </svg>
          {tr(schema.capabilities.create.label, lang)}
        </button>
      {/if}
    </div>

    <!-- Item list -->
    {#if groups}
      <div class="space-y-4">
        {#each groups as group (group.id)}
          <details
            class="group bg-white border border-gray-200 rounded-lg"
            open={!group.collapsed}
            data-group-id={group.id}
          >
            <summary class="px-4 py-3 cursor-pointer flex items-center justify-between">
              <span class="flex items-center gap-2">
                <span class="text-sm font-medium text-gray-900">{tr(group.label, lang)}</span>
                {#if group.subtitle}
                  <span class="text-xs text-gray-500">{tr(group.subtitle, lang)}</span>
                {/if}
                {#if group.badges}
                  {#each group.badges as b}
                    <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium
                      {b.tone === 'primary' ? 'bg-pletka-primary/10 text-pletka-primary' : ''}
                      {b.tone === 'warning' ? 'bg-yellow-100 text-yellow-800' : ''}
                      {!b.tone || b.tone === 'neutral' ? 'bg-gray-100 text-gray-700' : ''}">
                      {b.label}
                    </span>
                  {/each}
                {/if}
              </span>
            </summary>
            <div class="divide-y divide-gray-200 border-t border-gray-200">
              {#each group.items as row (row.id)}
                <ListRow
                  item={row}
                  columns={schema.columns}
                  actions={group.read_only ? [] : (group.actions ?? schema.row_actions)}
                  {lang}
                  languages={schema.ui.languages}
                  {renamingId}
                  reorderable={false}
                  onaction={handleRowAction}
                  onrename={handleRename}
                  onrenamestart={(id) => renamingId = id}
                  onrenamecancel={() => renamingId = null}
                  hideActions={group.read_only || rowIsReadonly(row)}
                />
              {/each}
            </div>
          </details>
        {/each}
      </div>
    {:else if items.length === 0 && schema.empty_state}
      <div class="text-center py-12">
        <h3 class="mt-2 text-sm font-medium text-gray-900">{tr(schema.empty_state.title, lang)}</h3>
        <p class="mt-1 text-sm text-gray-500">{tr(schema.empty_state.message, lang)}</p>
      </div>
    {:else if schema.capabilities?.reorder}
      <div
        class="divide-y divide-gray-200 border border-gray-200 rounded-lg"
        use:dndzone={{items, flipDurationMs, type: schema.entity_type}}
        onconsider={handleDndConsider}
        onfinalize={handleDndFinalize}
      >
        {#each items as item (item.id)}
          <div animate:flip={{duration: flipDurationMs}} data-row-id={item.id}>
            <ListRow
              {item}
              columns={schema.columns}
              actions={schema.row_actions}
              {lang}
              languages={schema.ui.languages}
              {renamingId}
              onaction={handleRowAction}
              onrename={handleRename}
              onrenamestart={(id) => renamingId = id}
              onrenamecancel={() => renamingId = null}
              hideActions={rowIsReadonly(item)}
            />
          </div>
        {/each}
      </div>

      <p class="mt-3 text-xs text-gray-400">Drag rows to reorder. Changes are saved automatically.</p>
    {:else}
      <!--
        No reorder capability → render rows without dndzone. svelte-dnd-action
        rejects items missing an `id` property and freezes the island; gating
        the directive on capabilities.reorder lets schemas opt out cleanly.
      -->
      <div class="divide-y divide-gray-200 border border-gray-200 rounded-lg">
        {#each items as item (item.id ?? `${schema.entity_type}-${items.indexOf(item)}`)}
          <div data-row-id={item.id}>
            <ListRow
              {item}
              columns={schema.columns}
              actions={schema.row_actions}
              {lang}
              languages={schema.ui.languages}
              {renamingId}
              reorderable={false}
              onaction={handleRowAction}
              onrename={handleRename}
              onrenamestart={(id) => renamingId = id}
              onrenamecancel={() => renamingId = null}
              hideActions={rowIsReadonly(item)}
            />
          </div>
        {/each}
      </div>
    {/if}
  {:else if view === 'add' || view === 'edit'}
    <div>
      <div class="mb-4">
        <button type="button" onclick={backToList}
          class="inline-flex items-center text-sm text-gray-600 hover:text-gray-900">
          <svg class="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18"/>
          </svg>
          Back to list
        </button>
      </div>
      <h3 class="text-lg font-medium text-gray-900 mb-4">
        {view === 'add' ? tr(schema.capabilities.create?.label, lang) : 'Edit'}
      </h3>
      {#key view === 'add' ? 'add' : editingId}
        <FormRenderer
          schemaUrl={view === 'add'
            ? schema.capabilities.create!.form_schema_url
            : schema.capabilities.edit!.form_schema_url_template.replace('{id}', editingId!)}
          {lang}
          onsuccess={handleFormSuccess}
          oncancel={backToList}
        />
      {/key}
    </div>
  {/if}

  {#if deletingItem && schema?.capabilities.delete?.reassignment}
    <DeleteConfirm
      item={deletingItem}
      {items}
      countField={schema.capabilities.delete.reassignment.count_field}
      entityLabel={schema.capabilities.delete.reassignment.entity_label}
      reassignmentEnabled={schema.capabilities.delete.reassignment.enabled}
      {lang}
      onconfirm={confirmDelete}
      oncancel={cancelDelete}
    />
  {/if}

  {#if statsId && schema?.capabilities.stats}
    <StatsModal
      statsUrl={schema.capabilities.stats.url_template.replace('{id}', statsId)}
      {lang}
      onclose={() => statsId = null}
    />
  {/if}
{/if}
