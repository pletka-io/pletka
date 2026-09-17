<script lang="ts">
  import type { EntityListSchema, FilterOption, ViewMode } from '$lib/types/entity-list-schema';
  import { tr } from '$lib/types/form-schema';
  import EntityListToolbar from './EntityListToolbar.svelte';
  import AdoptPicker from './AdoptPicker.svelte';
  import FilterDrawer from './FilterDrawer.svelte';
  import ActiveFiltersBar from './ActiveFiltersBar.svelte';
  import PaginationBar from './PaginationBar.svelte';
  import { getRowWidget } from './row-widget-registry';
  import { getEntityListEditor } from './editor-widget-registry';
  import FormRenderer from '../form/FormRenderer.svelte';
  import StatsModal from '../list/StatsModal.svelte';
  import DeleteConfirm from '../list/DeleteConfirm.svelte';
  import { addToast } from '$lib/stores/toast';
  import { confirmAction } from '$lib/stores/confirm';
  import { captureException } from '$lib/error-tracking';

  let {
    schemaUrl,
    onmutate,
    initialItemId = '',
    onitemchange,
  }: {
    schemaUrl: string;
    /** Fired after a successful create / delete / row-action so a
     *  parent (e.g. ProjectDetail) can refresh dependent counters
     *  like tab-counts. Optional — non-project consumers omit it. */
    onmutate?: () => void;
    /** Item to open in edit mode once the schema has loaded (deep link). */
    initialItemId?: string;
    /** Fired when an item is opened (id) or the editor is closed (null). */
    onitemchange?: (id: string | null) => void;
  } = $props();

  let schema = $state<EntityListSchema | null>(null);
  let items = $state<any[]>([]);
  let total = $state(0);
  let currentPage = $state(1);
  let perPage = $state(50);
  let loading = $state(true);
  let error = $state('');
  let lang = $state('en');
  let currentSort = $state('');
  let currentFilters = $state<Record<string, string>>({});
  let currentSearch = $state('');
  let currentViewMode = $state('');
  let view = $state<'list' | 'add' | 'edit'>('list');
  let editingId = $state<string | null>(null);
  let deletingItem = $state<any | null>(null);
  let statsId = $state<string | null>(null);

  // Filter drawer state + lazy options cache (shared with ActiveFiltersBar).
  let drawerOpen = $state(false);
  let optionsByParam = $state<Record<string, FilterOption[] | undefined>>({});
  let loadingByParam = $state<Record<string, boolean>>({});
  let errorByParam = $state<Record<string, string>>({});
  const releaseVersion = $derived(new URLSearchParams(window.location.search).get('version') || '');
  const readOnlyReleaseMode = $derived(releaseVersion !== '');

  $effect(() => {
    loadSchema();

    const handleRefresh = () => { loadData(); };
    window.addEventListener('entity-list:refresh', handleRefresh);
    return () => {
      window.removeEventListener('entity-list:refresh', handleRefresh);
    };
  });

  function viewModeStorageKey(entityType: string): string {
    return `entity-list:${entityType}:view_mode`;
  }

  function pickInitialViewMode(viewModes: ViewMode[] | undefined, entityType: string): string {
    if (!viewModes || viewModes.length === 0) return '';
    try {
      const stored = localStorage.getItem(viewModeStorageKey(entityType));
      if (stored && viewModes.some((m) => m.id === stored)) {
        return stored;
      }
    } catch {
      // localStorage unavailable; fall through.
    }
    const defaultMode = viewModes.find((m) => m.default);
    return defaultMode ? defaultMode.id : viewModes[0].id;
  }

  async function loadSchema() {
    loading = true;
    error = '';
    try {
      const res = await fetch(schemaUrl);
      if (!res.ok) throw new Error(`Failed to load schema: ${res.status}`);
      schema = await res.json();
      lang = schema!.ui.primary_language || 'en';
      currentSort = schema!.default_sort;
      currentViewMode = pickInitialViewMode(schema!.view_modes, schema!.entity_type);
      if (schema!.pagination) {
        perPage = schema!.pagination.page_size;
      }
      // Seed optionsByParam from inline filter options (no fetch needed).
      for (const f of schema!.filters ?? []) {
        if (f.options && f.options.length > 0) {
          optionsByParam[f.param_name] = f.options;
        }
      }
      optionsByParam = { ...optionsByParam };
      if (initialItemId) {
        handleEdit(initialItemId);
      }
      applyStateFromURL();
      await loadData();
    } catch (e: any) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  // ---- URL state sync -----------------------------------------------------
  // Reads sort/filters/search/page/per_page/view from the URL on load and
  // writes them back on every state change via history.replaceState so shared
  // links reproduce the view without polluting back/forward history.

  function applyStateFromURL() {
    if (!schema) return;
    const params = new URLSearchParams(window.location.search);

    const sortParam = params.get('sort_by');
    if (sortParam && schema.sort_options.some((o) => o.value === sortParam)) {
      currentSort = sortParam;
    }

    if (schema.search) {
      const s = params.get(schema.search.param_name);
      if (s !== null) currentSearch = s;
    }

    const nextFilters: Record<string, string> = {};
    for (const f of schema.filters ?? []) {
      const v = params.get(f.param_name);
      if (v !== null && v !== '') {
        nextFilters[f.param_name] = v;
        continue;
      }
      // URL has no value for this filter — apply schema-declared
      // defaults so the curator opens the page with the intended
      // initial selection (e.g. Origin = Owned + Adapted).
      const defaultValues = (f.options ?? []).filter((o) => o.default).map((o) => o.value);
      if (defaultValues.length > 0) {
        nextFilters[f.param_name] = defaultValues.join(',');
      }
    }
    currentFilters = nextFilters;

    if (schema.pagination) {
      const p = Number(params.get(schema.pagination.param_name));
      if (Number.isFinite(p) && p >= 1) currentPage = p;
      const pp = Number(params.get(schema.pagination.per_page_name));
      if (Number.isFinite(pp) && pp > 0) perPage = pp;
    }

    const viewParam = params.get('view');
    if (viewParam && schema.view_modes?.some((m) => m.id === viewParam)) {
      currentViewMode = viewParam;
    }
  }

  function writeStateToURL() {
    if (!schema) return;
    const url = new URL(window.location.href);
    const params = url.searchParams;

    if (currentSort && currentSort !== schema.default_sort) {
      params.set('sort_by', currentSort);
    } else {
      params.delete('sort_by');
    }

    if (schema.search) {
      if (currentSearch) {
        params.set(schema.search.param_name, currentSearch);
      } else {
        params.delete(schema.search.param_name);
      }
    }

    for (const f of schema.filters ?? []) {
      const v = currentFilters[f.param_name];
      if (v) {
        params.set(f.param_name, v);
      } else {
        params.delete(f.param_name);
      }
    }

    if (schema.pagination) {
      if (currentPage > 1) {
        params.set(schema.pagination.param_name, String(currentPage));
      } else {
        params.delete(schema.pagination.param_name);
      }
      if (perPage !== schema.pagination.page_size) {
        params.set(schema.pagination.per_page_name, String(perPage));
      } else {
        params.delete(schema.pagination.per_page_name);
      }
    }

    const defaultMode = schema.view_modes?.find((m) => m.default)?.id ?? schema.view_modes?.[0]?.id ?? '';
    if (currentViewMode && currentViewMode !== defaultMode) {
      params.set('view', currentViewMode);
    } else {
      params.delete('view');
    }

    history.replaceState(null, '', url.toString());
  }

  async function loadData() {
    if (!schema) return;
    writeStateToURL();
    try {
      const url = new URL(schema.data_url, window.location.origin);
      if (releaseVersion) {
        url.searchParams.set('version', releaseVersion);
      }
      if (currentSort) {
        url.searchParams.set('sort_by', currentSort);
      }
      if (currentSearch && schema.search) {
        url.searchParams.set(schema.search.param_name, currentSearch);
      }
      for (const [key, value] of Object.entries(currentFilters)) {
        if (value) {
          url.searchParams.set(key, value);
        }
      }
      if (schema.pagination) {
        url.searchParams.set(schema.pagination.param_name, String(currentPage));
        url.searchParams.set(schema.pagination.per_page_name, String(perPage));
      }
      const res = await fetch(url.toString());
      if (!res.ok) throw new Error(`Failed to load data: ${res.status}`);
      const data = await res.json();
      items = data[schema.data_key] || [];
      if (data.total !== undefined) {
        total = data.total;
      }
    } catch (e: any) {
      error = e.message;
    }
  }

  async function ensureLazyOptions(paramName: string, optionsUrl: string): Promise<void> {
    if (optionsByParam[paramName] !== undefined) return;
    loadingByParam[paramName] = true;
    loadingByParam = { ...loadingByParam };
    try {
      const res = await fetch(optionsUrl);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { options: FilterOption[] };
      optionsByParam[paramName] = data.options ?? [];
      optionsByParam = { ...optionsByParam };
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      errorByParam[paramName] = msg;
      errorByParam = { ...errorByParam };
      optionsByParam[paramName] = [];
      optionsByParam = { ...optionsByParam };
    } finally {
      loadingByParam[paramName] = false;
      loadingByParam = { ...loadingByParam };
    }
  }

  function handleSearch(query: string) {
    currentSearch = query;
    currentPage = 1;
    loadData();
  }

  function handleFilterChange(paramName: string, value: string) {
    currentFilters = { ...currentFilters, [paramName]: value };
    currentPage = 1;
    loadData();
  }

  function handleClearFilter(paramName: string, nextValue: string = '') {
    const next = { ...currentFilters };
    if (nextValue) {
      next[paramName] = nextValue;
    } else {
      delete next[paramName];
    }
    currentFilters = next;
    currentPage = 1;
    loadData();
  }

  function handleClearSearch() {
    currentSearch = '';
    currentPage = 1;
    loadData();
  }

  function handleClearAll() {
    currentFilters = {};
    currentSearch = '';
    currentPage = 1;
    loadData();
  }

  function handleSortChange(value: string) {
    currentSort = value;
    currentPage = 1;
    loadData();
  }

  function handlePageChange(page: number) {
    currentPage = page;
    loadData();
  }

  function handlePerPageChange(newPerPage: number) {
    perPage = newPerPage;
    currentPage = 1;
    loadData();
  }

  function handleViewModeChange(value: string) {
    currentViewMode = value;
    if (schema) {
      try {
        localStorage.setItem(viewModeStorageKey(schema.entity_type), value);
      } catch {
        // localStorage unavailable; ignore.
      }
      writeStateToURL();
    }
  }

  function handleOpenFilters() {
    drawerOpen = true;
    // Fire off fetches for any filters with options_url that haven't been resolved.
    for (const f of schema?.filters ?? []) {
      if (f.options_url && optionsByParam[f.param_name] === undefined) {
        void ensureLazyOptions(f.param_name, f.options_url);
      }
    }
  }

  function buildActionURL(raw: string): string {
    if (!releaseVersion || !raw) return raw;
    try {
      const url = new URL(raw, window.location.origin);
      url.searchParams.set('version', releaseVersion);
      return url.toString();
    } catch {
      return raw;
    }
  }

  function showAdd() { view = 'add'; }
  function backToList() { view = 'list'; editingId = null; onitemchange?.(null); }

  async function handleFormSuccess() {
    await loadData();
    onmutate?.();
    backToList();
  }

  function handleEdit(id: string) {
    if (!schema?.capabilities?.edit || readOnlyReleaseMode) return;
    if (!EditorWidget && !inlineFormUrl('edit', id)) {
      reportMisconfiguredEditor('edit');
      return;
    }
    editingId = id;
    view = 'edit';
    onitemchange?.(id);
  }

  function findRowAction(actionId: string) {
    return schema?.row_actions?.find((a) => a.id === actionId);
  }

  function handleRowAction(actionId: string, item: any) {
    switch (actionId) {
      case 'edit':
        handleEdit(item.id);
        break;
      case 'stats':
        statsId = item.id;
        break;
      case 'delete':
        void handleDelete(item);
        break;
      default:
        void handleSchemaAction(actionId, item);
    }
  }

  async function handleSchemaAction(actionId: string, item: any) {
    const action = findRowAction(actionId);
    if (!action?.url_template) return;
    const url = buildActionURL(action.url_template.replace('{id}', item.id));
    const method = (action.method || 'POST').toUpperCase();
    if (readOnlyReleaseMode && method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
      return;
    }
    try {
      const res = await fetch(url, {
        method,
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (!res.ok) throw new Error(`Action ${actionId} failed`);
      addToast('success', `${tr(action.label, lang)} completed`);
      await loadData();
      onmutate?.();
    } catch (e: any) {
      addToast('error', e.message || `Action ${actionId} failed`);
    }
  }

  async function handleDelete(item: any) {
    if (!schema?.capabilities?.delete || readOnlyReleaseMode) return;
    if (!schema.capabilities.delete.reassignment) {
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
    if (!schema?.capabilities?.delete || !deletingItem) return;
    const cap = schema.capabilities.delete;
    const url = buildActionURL(cap.url_template.replace('{id}', deletingItem.id));
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
        await loadData();
        onmutate?.();
        return;
      }
      if (res.status === 409) {
        const data = await res.json().catch(() => ({}));
        // Prefer the server's steer ("… Deprecate it instead …") over the raw
        // error code so an in-use delete explains itself rather than failing mute.
        addToast('warning', data.message || data.error || 'Cannot delete');
        return;
      }
      throw new Error('Failed to delete');
    } catch (e: any) {
      addToast('error', e.message || 'Failed to delete');
    }
  }

  function cancelDelete() {
    deletingItem = null;
  }

  // The inline add/edit form needs either a registered editor widget or a
  // non-empty form_schema_url. With neither, FormRenderer would fetch an
  // empty URL that resolves to the current page (HTML) and crash on JSON
  // parse. inlineFormUrl()/reportMisconfiguredEditor() let the handlers
  // block that and record it in weave_error_events instead.
  function inlineFormUrl(mode: 'create' | 'edit', id?: string): string {
    if (mode === 'create') return schema?.capabilities?.create?.form_schema_url ?? '';
    return schema?.capabilities?.edit?.form_schema_url_template?.replace('{id}', id ?? '') ?? '';
  }

  function reportMisconfiguredEditor(mode: 'create' | 'edit'): void {
    const widget = schema?.editor?.widget;
    addToast('error', 'This form can’t be opened — it’s misconfigured. The issue has been logged.');
    captureException(
      new Error(`entity-list ${mode} form has no editor widget and no form_schema_url`),
      {
        island: 'EntityListView',
        entity_type: schema?.entity_type,
        mode,
        editor_widget: widget ?? null,
        editor_widget_registered: widget ? Boolean(EditorWidget) : null,
      },
    );
  }

  function handleCreate() {
    if (readOnlyReleaseMode) return;
    if (!EditorWidget && !inlineFormUrl('create')) {
      reportMisconfiguredEditor('create');
      return;
    }
    if (schema?.editor?.widget || schema?.capabilities?.create?.form_schema_url) {
      showAdd();
    }
  }

  let showAdoptPicker = $state(false);

  function handleAdopt() {
    if (readOnlyReleaseMode) return;
    if (!schema?.capabilities?.adopt) return;
    showAdoptPicker = true;
  }

  function closeAdoptPicker() {
    showAdoptPicker = false;
  }

  async function handleAdopted(label?: string) {
    await loadData();
    // Schema-driven UX rule: avoid baking domain wording. The label is
    // the adopted entity's display name in the current language; the
    // verb stays generic so it survives the Project→Weave rename
    // separately.
    addToast('success', label ? `Adopted ${label}` : 'Adopted');
  }

  function filterRowActions() {
    const actions = schema?.row_actions ?? [];
    if (!readOnlyReleaseMode) return actions;
    return actions.filter((action) => {
      const method = (action.method || '').toUpperCase();
      return action.id === 'stats' || method === 'GET';
    });
  }

  const activeWidgetName = $derived(
    schema?.view_modes?.find((m) => m.id === currentViewMode)?.widget
      ?? schema?.row_widget,
  );
  const RowWidget = $derived(schema ? getRowWidget(activeWidgetName) : null);
  const showColumnHeader = $derived(
    Boolean(schema?.column_header?.show) && activeWidgetName === 'editorial',
  );
  const searchLabel = $derived(
    schema?.search ? tr(schema.search.placeholder, lang).replace(/\.\.\.$/, '') : 'Search',
  );
  const effectiveCreateAction = $derived(readOnlyReleaseMode ? undefined : schema?.capabilities?.create);
  const effectiveAdoptAction = $derived(readOnlyReleaseMode ? undefined : schema?.capabilities?.adopt);
  const effectiveEditCap = $derived(readOnlyReleaseMode ? undefined : schema?.capabilities?.edit);
  const effectiveDeleteCap = $derived(readOnlyReleaseMode ? undefined : schema?.capabilities?.delete);
  const effectiveStatsCap = $derived(schema?.capabilities?.stats);
  const effectiveRowActions = $derived(filterRowActions());
  const EditorWidget = $derived(schema ? getEntityListEditor(schema.editor?.widget) : null);
</script>

{#if loading}
  <div class="animate-pulse space-y-4 p-6">
    <div class="h-10 bg-gray-200 rounded w-full"></div>
    <div class="h-16 bg-gray-200 rounded"></div>
    <div class="h-16 bg-gray-200 rounded"></div>
    <div class="h-16 bg-gray-200 rounded"></div>
  </div>
{:else if error}
  <div class="text-center py-12">
    <svg class="mx-auto h-12 w-12 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
    </svg>
    <h3 class="mt-2 text-sm font-medium text-gray-900">Failed to load</h3>
    <p class="mt-1 text-sm text-gray-500">{error}</p>
    <button onclick={() => loadSchema()} class="mt-3 text-sm text-pletka-primary hover:underline">Try again</button>
  </div>
{:else if schema && view === 'list'}
  <EntityListToolbar
    search={schema.search}
    {currentSearch}
    filters={schema.filters}
    {currentFilters}
    viewModes={schema.view_modes}
    {currentViewMode}
    createAction={effectiveCreateAction}
    adoptAction={effectiveAdoptAction}
    entityType={schema.entity_type}
    {lang}
    onsearch={handleSearch}
    onviewmodechange={handleViewModeChange}
    onopenfilters={handleOpenFilters}
    oncreate={handleCreate}
    onadopt={handleAdopt}
  />

  <div class="bg-white shadow overflow-hidden sm:rounded-md">

    <ActiveFiltersBar
      search={currentSearch}
      searchLabel={searchLabel}
      filters={schema.filters}
      {currentFilters}
      lazyOptions={optionsByParam}
      {lang}
      onclearsearch={handleClearSearch}
      onclearfilter={handleClearFilter}
    />

    {#if items.length === 0}
      <div class="text-center py-12">
        {#if schema.empty_state}
          <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
          </svg>
          <h3 class="mt-2 text-sm font-medium text-gray-900">{tr(schema.empty_state.title, lang)}</h3>
          <p class="mt-1 text-sm text-gray-500">{tr(schema.empty_state.message, lang)}</p>
        {:else}
          <p class="text-sm text-gray-500">No items found.</p>
        {/if}
      </div>
    {:else if RowWidget}
      {#if showColumnHeader && schema.column_header}
        <div
          class="grid grid-cols-[80px_1fr_240px] gap-6 border-b border-gray-200 bg-gray-50 px-6 py-3 font-mono text-[10px] uppercase text-gray-400"
          style="letter-spacing:0.14em;"
        >
          <span>{tr(schema.column_header.id_label, lang) || 'ID'}</span>
          <span>{tr(schema.column_header.title_label, lang) || ''}</span>
          {#if schema.row_layout.count_columns && schema.row_layout.count_columns.length > 0}
            <div
              class="grid gap-3 text-right"
              style="grid-template-columns: repeat({schema.row_layout.count_columns.length}, minmax(0, 1fr));"
            >
              {#each schema.row_layout.count_columns as col}
                <span>{tr(col.label, lang)}</span>
              {/each}
            </div>
          {:else}
            <span></span>
          {/if}
        </div>
      {/if}
      <div class="divide-y divide-gray-200">
        {#each items as item (item.id || item.ID)}
          <RowWidget
            {item}
            rowLayout={schema.row_layout}
            viewMode={currentViewMode}
            detailUrlTemplate={schema.detail_url_template}
            editUrlTemplate={schema.edit_url_template}
            projectId={schema.project_id}
            {releaseVersion}
            {lang}
            onEdit={effectiveEditCap ? handleEdit : undefined}
            rowActions={effectiveRowActions}
            onAction={handleRowAction}
          />
        {/each}
      </div>
    {/if}
  </div>

  {#if schema.pagination && total > 0}
    <PaginationBar
      page={currentPage}
      {perPage}
      {total}
      pageSizeOptions={schema.pagination.page_size_options}
      onPageChange={handlePageChange}
      onPerPageChange={handlePerPageChange}
    />
  {/if}

  <FilterDrawer
    open={drawerOpen}
    filters={schema.filters}
    sortOptions={schema.sort_options}
    {currentSort}
    {currentFilters}
    {optionsByParam}
    {loadingByParam}
    {errorByParam}
    controlPrefix={`entity-list-${schema.entity_type}`}
    {perPage}
    pageSizeOptions={schema.pagination?.page_size_options}
    {lang}
    onclose={() => (drawerOpen = false)}
    onfilterchange={handleFilterChange}
    onsortchange={handleSortChange}
    onperpagechange={handlePerPageChange}
    onclearall={handleClearAll}
  />

  {#if deletingItem && effectiveDeleteCap?.reassignment}
    <DeleteConfirm
      item={deletingItem}
      {items}
      countField={effectiveDeleteCap.reassignment.count_field}
      entityLabel={effectiveDeleteCap.reassignment.entity_label}
      reassignmentEnabled={effectiveDeleteCap.reassignment.enabled}
      {lang}
      onconfirm={confirmDelete}
      oncancel={cancelDelete}
    />
  {/if}

  {#if statsId && effectiveStatsCap}
    <StatsModal
      statsUrl={buildActionURL(effectiveStatsCap.url_template.replace('{id}', statsId))}
      {lang}
      onclose={() => (statsId = null)}
    />
  {/if}
{:else if schema && (view === 'add' || view === 'edit')}
  {#if EditorWidget}
    {#key view === 'add' ? 'custom-add' : `custom-${editingId}`}
      <EditorWidget
        {schema}
        mode={view === 'add' ? 'create' : 'edit'}
        itemId={view === 'edit' ? editingId! : ''}
        {lang}
        onsuccess={handleFormSuccess}
        oncancel={backToList}
      />
    {/key}
  {:else}
    <div>
      <div class="mb-4">
        <button
          type="button"
          onclick={backToList}
          class="inline-flex items-center text-sm text-gray-600 hover:text-gray-900"
        >
          <svg class="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18"/>
          </svg>
          Back to list
        </button>
      </div>
      <h3 class="text-lg font-medium text-gray-900 mb-4">
        {#if view === 'add'}
          {tr(schema.capabilities?.create?.label, lang)}
        {:else}
          Editing <code class="mx-1 px-1.5 py-0.5 bg-gray-100 rounded text-base font-mono">{editingId}</code>
        {/if}
      </h3>
      {#key view === 'add' ? 'add' : editingId}
        <FormRenderer
          schemaUrl={view === 'add'
            ? buildActionURL(effectiveCreateAction!.form_schema_url)
            : buildActionURL(effectiveEditCap!.form_schema_url_template.replace('{id}', editingId!))}
          {lang}
          onsuccess={handleFormSuccess}
          oncancel={backToList}
        />
      {/key}
    </div>
  {/if}
{/if}

{#if showAdoptPicker && schema?.capabilities?.adopt}
  <AdoptPicker
    cap={schema.capabilities.adopt}
    entityType={schema.entity_type}
    {lang}
    onclose={closeAdoptPicker}
    onadopted={handleAdopted}
  />
{/if}
