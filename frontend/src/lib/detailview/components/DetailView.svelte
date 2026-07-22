<!-- frontend/src/lib/detailview/components/DetailView.svelte -->
<script lang="ts">
  import { EntityViewState, type TabId } from '$lib/detailview/state.svelte';
  import { tr, type OriginInfo } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import { addToast } from '$lib/stores/toast';
  import { confirmAction } from '$lib/stores/confirm';
  import type { BadgeVariant } from '$lib/utils/field-display';
  import Badge from '$lib/components/shared/Badge.svelte';
  import LoadingState from '$lib/components/shared/LoadingState.svelte';
  import CategorySection from './CategorySection.svelte';
  import StatsTab from './StatsTab.svelte';
  import MetadataTab from './MetadataTab.svelte';
  import DiagramTab from './DiagramTab.svelte';
  import ReuseTab from './ReuseTab.svelte';
  import ConceptListEntriesTab from './ConceptListEntriesTab.svelte';
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';
  import OverrideEditor from './OverrideEditor.svelte';
  import EntityListView from '$lib/components/entity-list/EntityListView.svelte';

  let {
    projectId,
    entityType,
    entityId,
    routeBase = '/projects',
  }: {
    projectId: string;
    entityType: string;
    entityId: string;
    routeBase?: string;
  } = $props();

  // svelte-ignore state_referenced_locally
  const viewState = new EntityViewState(projectId, entityType, entityId, routeBase);

  let showHeaderMenu = $state(false);
  let editMode = $state<'none' | 'metadata' | 'composition'>('none');
  let adopting = $state(false);
  let forking = $state(false);
  let lifecycleBusy = $state(false);

  function startMetadataEdit() { editMode = 'metadata'; }
  function startCompositionEdit() { editMode = 'composition'; }
  function backToView() { editMode = 'none'; }
  async function handleEditSuccess() {
    await viewState.init();
    editMode = 'none';
  }
  async function adoptToProject() {
    const url = viewState.response?.capabilities?.adopt_url;
    if (!url || adopting) return;
    adopting = true;
    try {
      const res = await fetch(url, { method: 'POST' });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(body?.error || `Adopt failed: ${res.status}`);
      }
      await viewState.init();
    } catch (err) {
      viewState.error = err instanceof Error ? err.message : String(err);
    } finally {
      adopting = false;
    }
  }
  async function forkToEdit() {
    const url = viewState.response?.capabilities?.fork_url;
    if (!url || forking) return;
    forking = true;
    try {
      const res = await fetch(url, { method: 'POST' });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(body?.error || `Adapt failed: ${res.status}`);
      }
      const sourceName = viewState.response?.entity?.name
        ? tr(viewState.response.entity.name, getUILang())
        : viewState.response?.entity?.id || '';
      if (body?.url) {
        // Persist the toast across the navigation so the curator sees
        // confirmation on the new entity's page. Cleared on read.
        try {
          sessionStorage.setItem(
            '__pendingToast',
            JSON.stringify({ tone: 'success', message: sourceName ? `Adapted ${sourceName}` : 'Adapted' }),
          );
        } catch { /* sessionStorage unavailable; non-fatal */ }
        window.location.assign(body.url);
        return;
      }
      await viewState.init();
      addToast('success', sourceName ? `Adapted ${sourceName}` : 'Adapted');
    } catch (err) {
      viewState.error = err instanceof Error ? err.message : String(err);
    } finally {
      forking = false;
    }
  }

  // Deprecate / activate: POST the URL the schema gave us, then refresh
  // in place. Exactly one of deprecate_url / activate_url is present.
  async function toggleDeprecate() {
    const caps = viewState.response?.capabilities;
    const deprecate = !!caps?.deprecate_url;
    const url = caps?.deprecate_url || caps?.activate_url;
    if (!url || lifecycleBusy) return;
    lifecycleBusy = true;
    showHeaderMenu = false;
    try {
      const res = await fetch(url, { method: 'POST' });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body?.error || `Request failed: ${res.status}`);
      }
      await viewState.init();
      addToast('success', deprecate ? 'Deprecated' : 'Activated');
    } catch (err) {
      viewState.error = err instanceof Error ? err.message : String(err);
    } finally {
      lifecycleBusy = false;
    }
  }

  // Delete: confirm (destructive), DELETE, then redirect to the project
  // page — the entity no longer exists so we can't stay on its detail view.
  async function deleteEntity() {
    const url = viewState.response?.capabilities?.delete_url;
    if (!url || lifecycleBusy) return;
    showHeaderMenu = false;
    const name = tr(viewState.response?.entity?.name, getUILang(), entityId);
    const ok = await confirmAction({
      title: `Delete ${name}?`,
      message: 'This removes it from the project and cannot be undone.',
      confirmLabel: 'Delete',
      danger: true,
    });
    if (!ok) return;
    lifecycleBusy = true;
    try {
      // X-Requested-With marks this as an XHR mutation so the CSRF
      // middleware exempts it (see pkg/session/middleware.go). Without it
      // nosurf rejects the delete with a bare 400 before the handler runs.
      const res = await fetch(url, {
        method: 'DELETE',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body?.message || body?.error || `Delete failed: ${res.status}`);
      }
      try {
        sessionStorage.setItem(
          '__pendingToast',
          JSON.stringify({ tone: 'success', message: `Deleted ${name}` }),
        );
      } catch { /* sessionStorage unavailable; non-fatal */ }
      window.location.assign(`${routeBase}/${projectId}`);
    } catch (err) {
      viewState.error = err instanceof Error ? err.message : String(err);
      lifecycleBusy = false;
    }
  }

  // ─── Sentinel for sticky header ───
  let sentinelEl: HTMLDivElement | undefined = $state();

  // ─── Tab definitions ───
  // The Reuse tab is appended only when capabilities.reuse_url is
  // present (today: field views only). Schema-driven gating, same idiom
  // used for the Graph sub-tab in DiagramTab — server decides what's
  // visible by emitting (or omitting) the URL.
  const tabs = $derived.by<{ id: TabId; label: string; icon: string }[]>(() => {
    if (entityType === 'concept-list') {
      return [
        {
          id: 'entries',
          label: 'Entries',
          icon: 'M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z',
        },
        {
          id: 'metadata',
          label: 'Metadata',
          icon: 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
        },
      ];
    }
    const base: { id: TabId; label: string; icon: string }[] = [
      {
        id: 'fields',
        label: entityType === 'field' ? 'Pattern' : 'Patterns',
        icon: 'M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z',
      },
      {
        id: 'stats',
        label: 'Statistics',
        icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z',
      },
      {
        id: 'metadata',
        label: 'Metadata',
        icon: 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
      },
      ...(entityType === 'model' && viewState.response?.capabilities?.examples_schema_url
        ? [{
            id: 'examples' as TabId,
            label: 'Examples',
            icon: 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
          }]
        : []),
      {
        id: 'derivatives',
        label: 'Derivatives',
        icon: 'M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z',
      },
    ];
    if (viewState.response?.capabilities?.reuse_url) {
      base.push({
        id: 'reuse',
        label: 'Reuse',
        icon: 'M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15',
      });
    }
    return base;
  });

  // ─── Derived data ───
  const entityName = $derived(
    tr(viewState.response?.entity?.name, getUILang(), viewState.response?.entity?.id || ''),
  );

  const entityDescription = $derived(
    tr(viewState.response?.entity?.description, getUILang(), ''),
  );

  const entityIdentifier = $derived(viewState.response?.entity?.id || '');

  const entityStatus = $derived(viewState.response?.entity?.status || '');
  // Prefer the derived publication state (draft/published/modified/new) for the
  // header badge; fall back to raw status (release-view sends no publication
  // state, and its archived status already reads published).
  const displayStatus = $derived(
    viewState.response?.entity?.publication_state || entityStatus,
  );
  const entityDeprecated = $derived(!!viewState.response?.entity?.deprecated);
  const entityModelType = $derived(viewState.response?.entity?.model_type || '');
  const entityOrigin = $derived(viewState.response?.entity?.origin);

  function modelTypeLabel(t: string): string {
    switch (t) {
      case 'core': return 'Core';
      case 'auxiliary': return 'Auxiliary';
      case 'example': return 'Example';
      default: return t;
    }
  }
  function modelTypeVariant(t: string): 'blue' | 'gray' | 'yellow' {
    switch (t) {
      case 'core': return 'blue';
      case 'auxiliary': return 'gray';
      case 'example': return 'yellow';
      default: return 'gray';
    }
  }
  const release = $derived(viewState.response?.release);
  const releaseLabel = $derived(release ? tr(release.label, getUILang(), release.version) : '');

  const scopeLabel = $derived.by(() => {
    const scope = viewState.response?.entity?.ontology_scope;
    if (!scope) return '';
    return scope.prefix ? `${scope.prefix}:${scope.local_name}` : scope.local_name || '';
  });

  const sections = $derived(viewState.response?.sections ?? []);

  const showFieldSearch = $derived(viewState.activeTab === 'fields' && entityType !== 'concept-list');
  const entryCount = $derived(viewState.response?.entity?.entry_count ?? viewState.response?.entries?.length ?? 0);

  function statusVariant(s: string): BadgeVariant {
    switch (s) {
      case 'published': return 'green';
      case 'modified': return 'amber';
      case 'new': return 'blue';
      case 'draft': return 'gray';
      case 'deprecated': return 'yellow';
      case 'In Process':
      case 'in_process': return 'blue';
      default: return 'gray';
    }
  }

  // Badge variant kept hard-coded by origin.kind in the markup (color
  // discriminator only). The label text comes from the server-resolved
  // origin_label off the entity meta so the badge stays in sync with
  // the provenance card and respects locale.
  function originBadgeText(_origin?: OriginInfo): string {
    const meta = viewState.response?.entity as Record<string, any> | undefined;
    const label = meta?.origin_label;
    if (label && typeof label === 'object') return tr(label, getUILang());
    return '';
  }

  // provenanceTitle + provenanceHelp now read pre-resolved Translations
  // maps off the entity meta. The server fills origin_label / origin_help
  // via LocalizedText (see pkg/weave/detailview/handler.go originDetail*)
  // and the i18n walker substitutes bundle values per language before
  // encode. No domain wording lives in this component — schema-driven
  // UI rule (.claude/rules/ui-patterns.md).
  function provenanceTitle(): string {
    const meta = viewState.response?.entity as Record<string, any> | undefined;
    const label = meta?.origin_label;
    if (label && typeof label === 'object') return tr(label, getUILang());
    return '';
  }

  function provenanceHelp(): string {
    const meta = viewState.response?.entity as Record<string, any> | undefined;
    const help = meta?.origin_help;
    if (help && typeof help === 'object') return tr(help, getUILang());
    return '';
  }

  function entityViewURL(): string {
    const base = `${routeBase}/${projectId}/entity-view/${entityType}/${entityId}`;
    if (!viewState.releaseVersion) return base;
    return `${base}?version=${encodeURIComponent(viewState.releaseVersion)}`;
  }

  function fieldSearchKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      viewState.fieldSearchNav(e.shiftKey ? -1 : 1);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      viewState.fieldSearchClear();
    }
  }

  // ─── Init ───
  $effect(() => {
    viewState.init();
  });

  // ─── Sticky header: IntersectionObserver ───
  // Only compact header on the fields tab (long scrollable content).
  // Other tabs (stats, metadata, derivatives) are short — compaction just
  // flickers the header for no benefit.
  $effect(() => {
    if (!sentinelEl) return;
    if (viewState.activeTab !== 'fields') {
      viewState.headerCompacted = false;
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        viewState.headerCompacted = !entries[0].isIntersecting;
      },
      { rootMargin: '-80px 0px 0px 0px', threshold: 0 },
    );
    observer.observe(sentinelEl);
    return () => observer.disconnect();
  });

  // ─── Scroll context: category/collection detection ───
  $effect(() => {
    if (!viewState.headerCompacted) {
      viewState.scrollCategoryName = '';
      viewState.scrollCollectionName = '';
      return;
    }

    let rafId = 0;

    function updateContext() {
      const headerBottom = 140;
      let catName = '';
      let collName = '';

      for (const el of document.querySelectorAll<HTMLElement>('[data-category-id]')) {
        const rect = el.getBoundingClientRect();
        if (rect.top <= headerBottom && rect.bottom > headerBottom) {
          catName = el.dataset.categoryName || '';
        }
      }

      for (const el of document.querySelectorAll<HTMLElement>('[data-collection-name]')) {
        const rect = el.getBoundingClientRect();
        if (rect.top <= headerBottom && rect.bottom > headerBottom) {
          collName = el.dataset.collectionName || '';
        }
      }

      viewState.scrollCategoryName = catName;
      viewState.scrollCollectionName = collName;
    }

    function onScroll() {
      cancelAnimationFrame(rafId);
      rafId = requestAnimationFrame(updateContext);
    }

    window.addEventListener('scroll', onScroll, { passive: true });
    updateContext();
    return () => {
      window.removeEventListener('scroll', onScroll);
      cancelAnimationFrame(rafId);
    };
  });
</script>

{#snippet tabBar()}
  <div class="border-b border-gray-200 mb-6 bg-white">
    <nav class="flex items-center" aria-label="Tabs">
      <div class="flex space-x-4">
        {#each tabs as tab (tab.id)}
          <button
            type="button"
            class="px-4 py-3 font-medium text-sm border-b-2 transition-colors
              {viewState.activeTab === tab.id
                ? 'text-blue-600 border-blue-600'
                : 'text-gray-500 border-transparent hover:text-gray-700'}"
            onclick={() => viewState.setTab(tab.id)}
          >
            <svg class="inline-block w-5 h-5 mr-2 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={tab.icon} />
            </svg>
            {tab.label}
            {#if tab.id === 'fields' && entityType !== 'field'}
              <span class="ml-2 px-2 py-0.5 rounded-full text-xs bg-blue-100 text-blue-800">
                {viewState.totalFieldCount}
              </span>
            {/if}
          </button>
        {/each}
      </div>

      <!-- Field search (visible on patterns tab) -->
      {#if showFieldSearch}
        <div class="ml-auto flex items-center gap-2 pr-2">
          <svg class="w-4 h-4 text-gray-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            bind:value={viewState.searchQuery}
            oninput={() => viewState.fieldSearchUpdate()}
            onkeydown={fieldSearchKeydown}
            placeholder="Search fields..."
            class="w-48 px-2 py-1 text-sm border border-gray-300 rounded-md focus:w-72 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all outline-none"
          />
          {#if viewState.matchOrder.length > 0}
            <span class="text-xs text-gray-500 whitespace-nowrap">
              {viewState.searchIndex + 1} of {viewState.matchOrder.length}
            </span>
            <button
              type="button"
              class="px-1.5 py-0.5 text-xs border border-gray-300 rounded bg-white text-gray-500 hover:bg-gray-50"
              onclick={() => viewState.fieldSearchNav(-1)}
              title="Previous (Shift+Enter)"
            >&#9650;</button>
            <button
              type="button"
              class="px-1.5 py-0.5 text-xs border border-gray-300 rounded bg-white text-gray-500 hover:bg-gray-50"
              onclick={() => viewState.fieldSearchNav(1)}
              title="Next (Enter)"
            >&#9660;</button>
            <button
              type="button"
              class="px-1.5 py-0.5 text-xs border border-gray-300 rounded bg-white text-gray-500 hover:bg-gray-50"
              onclick={() => viewState.fieldSearchClear()}
              title="Clear (Escape)"
            >&times;</button>
          {:else if viewState.searchQuery.trim()}
            <span class="text-xs text-gray-400">No matches</span>
          {/if}
        </div>
      {/if}
    </nav>
  </div>
{/snippet}

<div class="max-w-7xl mx-auto">
  {#if viewState.loading && !viewState.response}
    <LoadingState message="Loading entity view..." />
  {:else if viewState.error && !viewState.response}
    <div class="bg-red-50 border border-red-200 rounded-lg p-6 text-center">
      <svg class="mx-auto h-12 w-12 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
      <h3 class="mt-2 text-sm font-medium text-red-800">Failed to load {entityType}</h3>
      <p class="mt-1 text-sm text-red-600">{viewState.error}</p>
      <button
        type="button"
        class="mt-4 inline-flex items-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-red-600 hover:bg-red-700"
        onclick={() => viewState.init()}
      >
        Retry
      </button>
    </div>
  {:else if viewState.response?.entity}
    {#if release}
      <div class="mb-6 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-blue-200 bg-blue-50 px-4 py-3">
        <div class="flex items-center gap-3">
          <Badge variant="blue" size="sm">release</Badge>
          <div>
            <div class="text-sm font-medium text-blue-900">{releaseLabel}</div>
            <div class="text-xs text-blue-700">This page is showing a read-only snapshot.</div>
          </div>
        </div>
        <a
          href={release.draft_url}
          class="inline-flex items-center rounded-md border border-blue-300 bg-white px-3 py-1.5 text-sm font-medium text-blue-800 hover:bg-blue-100"
        >
          Return to draft
        </a>
      </div>
    {/if}

    <!-- Sentinel: observed to trigger header compaction when scrolled past -->
    <div bind:this={sentinelEl} class="h-1 -mb-1"></div>

    <!-- Header (always sticky -- only content changes between full/compact) -->
    <div
      class="bg-white shadow-sm sticky top-16 z-30
        {viewState.headerCompacted ? 'rounded-none shadow-md' : 'rounded-lg p-6 mb-0'}"
    >
      {#if viewState.headerCompacted}
        <!-- Compact header + tabs in one sticky block -->
        <div class="px-6 py-2">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3 min-w-0">
              <h1 class="text-lg font-bold text-gray-900 truncate">{entityName}</h1>
              <span class="text-sm text-gray-500 font-mono flex-shrink-0">{entityIdentifier}</span>
              {#if entityStatus}
                <Badge variant={statusVariant(displayStatus)} size="xs">{displayStatus}</Badge>
              {/if}
              {#if entityDeprecated}
                <Badge variant="red" size="xs">⊘ Deprecated</Badge>
              {/if}
              {#if entityModelType}
                <Badge variant={modelTypeVariant(entityModelType)} size="xs">{modelTypeLabel(entityModelType)}</Badge>
              {/if}
              {#if release}
                <Badge variant="blue" size="xs">{release.version}</Badge>
              {/if}
              {#if entityOrigin?.kind === 'inherited' || entityOrigin?.kind === 'adopted' || entityOrigin?.kind === 'forked'}
                <Badge variant={entityOrigin?.kind === 'forked' ? 'blue' : 'amber'} size="xs">{originBadgeText(entityOrigin)}</Badge>
              {/if}
              {#if scopeLabel}
                <Badge variant="purple" size="xs">{scopeLabel}</Badge>
              {/if}
            </div>
          </div>
        </div>
        {#if viewState.activeTab === 'fields'}
          {@const catName = viewState.scrollCategoryName || ''}
          {@const collName = viewState.scrollCollectionName || ''}
          {#if catName}
            <div class="px-6 pb-1 flex items-center text-xs text-gray-400">
              <svg class="h-3 w-3 mr-1 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
              </svg>
              <span class="truncate">{catName}</span>
              {#if collName}
                <svg class="h-3 w-3 mx-1 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                </svg>
                <span class="truncate">{collName}</span>
              {/if}
            </div>
          {/if}
        {/if}
        {@render tabBar()}
      {:else}
        <!-- Full header -->
        <div class="flex items-start justify-between">
          <div class="flex-1">
            <div class="flex items-center">
              <h1 class="text-3xl font-bold text-gray-900">{entityName}</h1>
              {#if entityStatus}
                <span class="ml-3">
                  <Badge variant={statusVariant(displayStatus)} size="sm">{displayStatus}</Badge>
                </span>
              {/if}
              {#if entityDeprecated}
                <span class="ml-1">
                  <Badge variant="red" size="sm">⊘ Deprecated</Badge>
                </span>
              {/if}
              {#if entityModelType}
                <span class="ml-1">
                  <Badge variant={modelTypeVariant(entityModelType)} size="sm">{modelTypeLabel(entityModelType)}</Badge>
                </span>
              {/if}
              {#if release}
                <span class="ml-1">
                  <Badge variant="blue" size="sm">{release.version}</Badge>
                </span>
              {/if}
            </div>
            {#if entityDescription}
              <p class="mt-2 text-gray-600">{entityDescription}</p>
            {/if}
            <!-- Provenance card: merges the title-line origin badge and the
                 amber-card help text into one surface, with the Adapt CTA
                 inline so curators don't have to hunt for it (UX review
                 items 5 + 6 + 7). Colors mirror the list-row badge
                 palette so the legend reads consistently. -->
            {#if entityOrigin?.kind === 'inherited' || entityOrigin?.kind === 'adopted' || entityOrigin?.kind === 'adopted_reference' || entityOrigin?.kind === 'forked'}
              {@const palette =
                entityOrigin?.kind === 'forked'
                  ? { bg: 'bg-amber-50', border: 'border-amber-200', title: 'text-amber-950', help: 'text-amber-900' }
                  : entityOrigin?.kind === 'adopted'
                    ? { bg: 'bg-indigo-50', border: 'border-indigo-200', title: 'text-indigo-950', help: 'text-indigo-900' }
                    : entityOrigin?.kind === 'adopted_reference'
                      ? { bg: 'bg-slate-50', border: 'border-slate-300', title: 'text-slate-900', help: 'text-slate-700' }
                      : { bg: 'bg-blue-50', border: 'border-blue-200', title: 'text-blue-950', help: 'text-blue-900' }}
              <div class="mt-3 max-w-3xl rounded-lg border {palette.border} {palette.bg} px-4 py-3">
                <div class="flex flex-wrap items-start gap-3">
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium {palette.title}">{provenanceTitle()}</div>
                    {#if provenanceHelp()}
                      <div class="mt-1 text-sm {palette.help}">{provenanceHelp()}</div>
                    {/if}
                  </div>
                  {#if !viewState.response?.capabilities?.editable && viewState.response?.capabilities?.fork_url}
                    <button
                      type="button"
                      onclick={forkToEdit}
                      disabled={forking}
                      class="shrink-0 inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-blue-300 bg-white text-blue-900 hover:bg-blue-50 focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      {forking
                        ? (tr((viewState.response?.capabilities as any)?.fork_pending, getUILang()) || 'Adapting…')
                        : (tr((viewState.response?.capabilities as any)?.fork_label, getUILang()) || 'Adapt to edit')}
                    </button>
                  {/if}
                </div>
              </div>
            {/if}
            <div class="mt-3 flex items-center space-x-4 text-sm text-gray-500">
              {#if entityIdentifier}
                <span class="flex items-center">
                  <svg class="mr-1.5 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                  </svg>
                  {entityIdentifier}
                </span>
              {/if}
              {#if scopeLabel}
                <span class="flex items-center">
                  <svg class="mr-1.5 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
                  </svg>
                  <Badge variant="purple" size="xs">{scopeLabel}</Badge>
                </span>
              {/if}
              {#if entityType === 'concept-list'}
                <span class="flex items-center">
                  <svg class="mr-1.5 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
                  </svg>
                  {entryCount} entries
                </span>
              {:else}
                <span class="flex items-center">
                  <svg class="mr-1.5 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                  </svg>
                  {viewState.totalFieldCount} fields
                </span>
              {/if}
            </div>
          </div>

          <!-- Action buttons (read-only view) -->
          <div class="flex items-center space-x-3 ml-4">
            {#if viewState.response?.capabilities?.editable && viewState.response?.capabilities?.metadata_url}
              <button
                type="button"
                onclick={startMetadataEdit}
                class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-gray-300 bg-white hover:bg-gray-50 focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
              >
                Edit Metadata
              </button>
            {/if}
            {#if entityType !== 'field' && entityType !== 'concept-list' && viewState.response?.capabilities?.editable && viewState.response?.capabilities?.save_url}
              <button
                type="button"
                onclick={startCompositionEdit}
                class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-gray-300 bg-white hover:bg-gray-50 focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
              >
                Edit Pattern
              </button>
            {/if}
            {#if !viewState.response?.capabilities?.editable && viewState.response?.capabilities?.adopt_url}
              <button
                type="button"
                onclick={adoptToProject}
                disabled={adopting}
                class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-emerald-300 bg-emerald-50 text-emerald-900 hover:bg-emerald-100 focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {adopting ? 'Adopting…' : (tr((viewState.response?.capabilities as any)?.adopt_label, getUILang()) || 'Adopt')}
              </button>
            {/if}
            <!-- Adapt CTA moved into the provenance card inline (UX
                 review item 6). Action strip keeps Adopt + Edit only. -->

            <div class="relative">
              <button
                type="button"
                class="p-2 text-gray-400 hover:text-gray-600 rounded-md hover:bg-gray-100"
                onclick={(e: MouseEvent) => { e.stopPropagation(); showHeaderMenu = !showHeaderMenu; }}
                title="More actions"
              >
                <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M10 6a2 2 0 110-4 2 2 0 010 4zM10 12a2 2 0 110-4 2 2 0 010 4zM10 18a2 2 0 110-4 2 2 0 010 4z" />
                </svg>
              </button>
              {#if showHeaderMenu}
                <!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <div
                  class="absolute right-0 mt-1 w-56 rounded-md shadow-lg bg-white ring-1 ring-black ring-opacity-5 z-50"
                  onclick={(e) => e.stopPropagation()}
                >
                  <div class="py-1" role="menu">
                    <button
                      type="button"
                      class="w-full text-left px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 flex items-center"
                      onclick={() => { navigator.clipboard.writeText(entityId); showHeaderMenu = false; }}
                    >
                      <svg class="mr-3 h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                      </svg>
                      Copy ID
                    </button>
                    <button
                      type="button"
                      class="w-full text-left px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 flex items-center"
                      onclick={() => { window.open(entityViewURL(), '_blank'); showHeaderMenu = false; }}
                    >
                      <svg class="mr-3 h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
                      </svg>
                      View as JSON
                    </button>
                    {#if viewState.response?.capabilities?.deprecate_url || viewState.response?.capabilities?.activate_url || viewState.response?.capabilities?.delete_url}
                      <div class="my-1 border-t border-gray-100"></div>
                    {/if}
                    {#if viewState.response?.capabilities?.deprecate_url || viewState.response?.capabilities?.activate_url}
                      <button
                        type="button"
                        disabled={lifecycleBusy}
                        class="w-full text-left px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 flex items-center disabled:opacity-50 disabled:cursor-not-allowed"
                        onclick={toggleDeprecate}
                      >
                        <svg class="mr-3 h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
                        </svg>
                        {viewState.response?.capabilities?.deprecate_url ? 'Deprecate' : 'Activate'}
                      </button>
                    {/if}
                    {#if viewState.response?.capabilities?.delete_url}
                      <button
                        type="button"
                        disabled={lifecycleBusy}
                        class="w-full text-left px-4 py-2 text-sm text-red-600 hover:bg-red-50 flex items-center disabled:opacity-50 disabled:cursor-not-allowed"
                        onclick={deleteEntity}
                      >
                        <svg class="mr-3 h-4 w-4 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                        </svg>
                        Delete
                      </button>
                    {/if}
                  </div>
                </div>
              {/if}
            </div>
          </div>
        </div>
      {/if}
    </div>

    <!-- Error toast -->
    {#if viewState.error}
      <div class="mb-4 bg-red-50 border border-red-200 rounded-lg p-4 flex items-center justify-between">
        <div class="flex items-center">
          <svg class="h-5 w-5 text-red-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span class="text-sm text-red-700">{viewState.error}</span>
        </div>
        <button
          type="button"
          class="text-red-400 hover:text-red-600"
          onclick={() => { viewState.error = null; }}
          aria-label="Dismiss error"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    {/if}

    <!-- Edit mode: replace tab content with inline FormRenderer -->
    {#if editMode === 'metadata'}
      <div class="mt-4">
        <div class="mb-4 flex items-center justify-between">
          <button
            type="button"
            onclick={backToView}
            class="inline-flex items-center text-sm text-gray-600 hover:text-gray-900"
          >
            <svg class="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18"/>
            </svg>
            Back
          </button>
          <div class="text-sm text-gray-600">
            Editing
            <code class="mx-1 px-1.5 py-0.5 bg-gray-100 rounded text-xs font-mono">{entityId}</code>
            {#if viewState.response?.entity?.name}
              <span class="text-gray-700 font-medium">{tr(viewState.response.entity.name, getUILang())}</span>
            {/if}
          </div>
        </div>
        <FormRenderer
          schemaUrl={viewState.response!.capabilities.metadata_url!}
          onsuccess={handleEditSuccess}
          oncancel={backToView}
        />
      </div>
    {:else if editMode === 'composition'}
      <div class="mt-4">
        <OverrideEditor
          saveUrl={viewState.response!.capabilities.save_url!}
          {viewState}
          onback={backToView}
          onsuccess={handleEditSuccess}
        />
      </div>
    {:else}

    <!-- Tab Navigation (only shown standalone when NOT compacted; otherwise rendered inside compact header) -->
    {#if !viewState.headerCompacted}
      {@render tabBar()}
    {/if}

    <!-- Tab Content -->
    {#if viewState.activeTab === 'entries'}
      <ConceptListEntriesTab state={viewState} />
    {:else if viewState.activeTab === 'fields'}
      <div data-tab-content="fields">
        <!-- Toolbar (only for models/collections with sections) -->
        {#if entityType !== 'field'}
          <div class="flex items-center justify-between mb-4">
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="px-2.5 py-1 text-xs text-gray-600 hover:text-gray-900 bg-gray-100 hover:bg-gray-200 rounded"
                onclick={() => viewState.expandAllCategories()}
              >
                Expand all
              </button>
              <button
                type="button"
                class="px-2.5 py-1 text-xs text-gray-600 hover:text-gray-900 bg-gray-100 hover:bg-gray-200 rounded"
                onclick={() => viewState.collapseAllCategories()}
              >
                Collapse all
              </button>
            </div>

            <div class="flex items-center gap-1 bg-gray-100 rounded p-0.5">
              <button
                type="button"
                class="px-2.5 py-1 text-xs rounded transition-colors
                  {viewState.viewMode === 'compact' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'}"
                onclick={() => viewState.setViewMode('compact')}
              >
                Compact
              </button>
              <button
                type="button"
                class="px-2.5 py-1 text-xs rounded transition-colors
                  {viewState.viewMode === 'detailed' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'}"
                onclick={() => viewState.setViewMode('detailed')}
              >
                Detailed
              </button>
            </div>
          </div>
        {/if}

        <!-- Sections (models/collections) or field detail -->
        {#if entityType === 'field'}
          {@const entity = viewState.response?.entity}
          {#if entity}
            <div class="space-y-6">
              <!-- Ontology Path -->
              {#if entity.path_elements?.length}
                <div class="bg-white border border-gray-200 rounded-lg p-5">
                  <h3 class="text-sm font-semibold text-gray-700 mb-3">Ontology Path</h3>
                  <div class="flex flex-wrap items-center gap-1.5">
                    {#if entity.ontology_scope}
                      <span class="inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium bg-purple-100 text-purple-800 border border-purple-200">
                        {entity.ontology_scope.prefix}:{entity.ontology_scope.local_name}
                      </span>
                    {/if}
                    {#each entity.path_elements as pe, i}
                      <svg class="w-3 h-3 text-gray-300 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clip-rule="evenodd"></path>
                      </svg>
                      <span class="inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium
                        {pe.type === 'class' ? 'bg-blue-50 text-blue-700 border border-blue-200' :
                         pe.type === 'property' ? 'bg-green-50 text-green-700 border border-green-200' :
                         'bg-gray-50 text-gray-700 border border-gray-200'}">
                        {pe.prefix}:{pe.local_name}
                      </span>
                    {/each}
                  </div>
                  {#if entity.ontology_path}
                    <details class="mt-3">
                      <summary class="text-xs text-gray-400 cursor-pointer hover:text-gray-600">Raw path</summary>
                      <code class="block mt-1 text-xs text-gray-500 bg-gray-50 px-3 py-2 rounded font-mono break-all">{entity.ontology_path}</code>
                    </details>
                  {/if}
                </div>
              {/if}

              <!-- Field Properties -->
              <div class="bg-white border border-gray-200 rounded-lg p-5">
                <h3 class="text-sm font-semibold text-gray-700 mb-3">Properties</h3>
                <dl class="grid grid-cols-[160px_1fr] gap-x-4 gap-y-3 text-sm">
                  {#if entity.expected_value_type}
                    <dt class="text-gray-500">Expected Value</dt>
                    <dd>
                      <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-50 text-amber-800 border border-amber-200">
                        {entity.expected_value_type}
                      </span>
                    </dd>
                  {/if}
                  {#if entity.set_value}
                    <dt class="text-gray-500">Set Value</dt>
                    <dd class="font-mono text-xs bg-gray-50 px-2 py-1 rounded">{entity.set_value}</dd>
                  {/if}
                  {#if entity.category_id}
                    <dt class="text-gray-500">Category</dt>
                    {#if viewState.response?.refs?.categories?.[entity.category_id]?.url}
                      <dd>
                        <a href={viewState.response.refs.categories[entity.category_id].url} class="text-pletka-primary hover:underline">
                          {entity.category_id}
                        </a>
                      </dd>
                    {:else}
                      <dd class="font-mono text-xs">{entity.category_id}</dd>
                    {/if}
                  {/if}
                  <dt class="text-gray-500">System Name</dt>
                  <dd class="font-mono text-xs">{entity.system_name}</dd>
                </dl>
              </div>
            </div>
          {/if}
        {:else}
          <div>
            {#each sections as section (section.id)}
              <CategorySection {section} state={viewState} {entityType} />
            {/each}
          </div>
        {/if}
      </div>
    {:else if viewState.activeTab === 'stats'}
      <StatsTab state={viewState} {entityType} />
    {:else if viewState.activeTab === 'metadata'}
      <MetadataTab state={viewState} />
      {#if viewState.response?.capabilities?.deprecate_url || viewState.response?.capabilities?.activate_url || viewState.response?.capabilities?.delete_url}
        <div class="mt-6 rounded-lg border border-gray-200 bg-white p-4">
          <h3 class="mb-1 text-sm font-semibold text-gray-900">Lifecycle</h3>
          <p class="mb-3 text-xs text-gray-500">
            Deprecate to retire this entity while keeping it referenceable, or delete it permanently.
          </p>
          <div class="flex flex-wrap gap-2">
            {#if viewState.response?.capabilities?.deprecate_url || viewState.response?.capabilities?.activate_url}
              <button
                type="button"
                disabled={lifecycleBusy}
                onclick={toggleDeprecate}
                class="inline-flex items-center rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {viewState.response?.capabilities?.deprecate_url ? 'Deprecate' : 'Activate'}
              </button>
            {/if}
            {#if viewState.response?.capabilities?.delete_url}
              <button
                type="button"
                disabled={lifecycleBusy}
                onclick={deleteEntity}
                class="inline-flex items-center rounded-md border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
              >
                Delete
              </button>
            {/if}
          </div>
        </div>
      {/if}
    {:else if viewState.activeTab === 'examples'}
      {#if viewState.response?.capabilities?.examples_schema_url}
        <EntityListView schemaUrl={viewState.response.capabilities.examples_schema_url} />
      {/if}
    {:else if viewState.activeTab === 'derivatives'}
      <DiagramTab
        {entityId}
        {entityType}
        {projectId}
        derivatives={viewState.response?.capabilities?.derivatives}
      />
    {:else if viewState.activeTab === 'reuse'}
      <ReuseTab state={viewState} {entityType} />
    {/if}

    {/if}
  {/if}
</div>

<svelte:window
  onclick={() => { if (showHeaderMenu) showHeaderMenu = false; }}
  onkeydown={(e) => {
    if (e.key === 'Escape') {
      if (showHeaderMenu) showHeaderMenu = false;
    }
  }}
/>
