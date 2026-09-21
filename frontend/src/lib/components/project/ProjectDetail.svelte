<!-- frontend/src/lib/components/project/ProjectDetail.svelte -->
<script lang="ts">
  import { ProjectDetailState } from '$lib/stores/project-detail.svelte';
  import type { ProjectAdoptionsTabSchema, ProjectOverviewSchema, ProjectReleaseTabSchema } from '$lib/types/project-page';
  import ProjectHeader from './ProjectHeader.svelte';
  import TabBar from './TabBar.svelte';
  import ProjectOverview from './ProjectOverview.svelte';
  import ProjectAdoptions from './ProjectAdoptions.svelte';
  import ProjectReleases from './ProjectReleases.svelte';
  import EntityListView from '$lib/components/entity-list/EntityListView.svelte';
  import LoadingState from '$lib/components/shared/LoadingState.svelte';

  let {
    projectId = '',
    schemaUrl,
    entityLabel = 'project',
  }: {
    projectId?: string;
    schemaUrl?: string;
    entityLabel?: string;
  } = $props();

  // svelte-ignore state_referenced_locally
  const state = new ProjectDetailState(projectId, schemaUrl, entityLabel);

  $effect(() => {
    state.init();
  });

  const activeTab = $derived(
    state.findTab(state.activeTabId),
  );

  const activeContent = $derived(state.tabContent.get(state.activeTabId));

  function handleTabClick(tabId: string) {
    // Clicking the tab you are already on closes whatever the list has open
    // (George: "clicking Examples should take me back to the list").
    if (state.findTab(tabId)?.id === state.activeTabId) {
      state.resetActiveList();
      return;
    }
    state.setActiveTab(tabId);
  }

  /**
   * Dispatch tab content to the right renderer based on schema shape.
   * - entity-list tabs: routed to EntityListView with schemaUrl = tab.content_url
   *   (EntityListView fetches its own schema and data).
   * - overview tab: fetched by state, content has sections[] → ProjectOverview.
   * - other schema-driven tabs: fetched by state, fall through to a placeholder.
   */
  function contentKind(content: unknown): 'overview' | 'adoptions' | 'releases' | 'unknown' {
    if (!content || typeof content !== 'object') return 'unknown';
    const c = content as Record<string, unknown>;
    if (Array.isArray(c.sections)) return 'overview';
    if (c.kind === 'adoptions' && Array.isArray(c.items)) return 'adoptions';
    if (c.kind === 'releases' && Array.isArray(c.items)) return 'releases';
    return 'unknown';
  }
</script>

<div class="max-w-7xl mx-auto">
  {#if state.loading && !state.pageSchema}
    <LoadingState message="Loading {entityLabel}..." />
  {:else if state.error && !state.pageSchema}
    <div class="bg-red-50 border border-red-200 rounded-lg p-6">
      <h3 class="text-red-800 font-medium">Failed to load {entityLabel}</h3>
      <p class="text-red-600 text-sm mt-1">{state.error}</p>
      <button
        type="button"
        class="mt-3 px-3 py-1.5 text-sm bg-red-100 hover:bg-red-200 text-red-800 rounded"
        onclick={() => state.init()}
      >
        Retry
      </button>
    </div>
  {:else if state.pageSchema}
    <ProjectHeader entity={state.pageSchema.entity} release={state.pageSchema.release} navLinks={state.pageSchema.nav_links ?? []} />

    {#if state.pageSchema.warnings && state.pageSchema.warnings.length}
      <div class="mb-4 space-y-3">
        {#each state.pageSchema.warnings as w (w.id)}
          <div
            class="flex items-start gap-3 rounded-md border-l-4 p-4
              {w.severity === 'error' ? 'border-red-400 bg-red-50 text-red-800' : ''}
              {w.severity === 'warning' ? 'border-amber-400 bg-amber-50 text-amber-900' : ''}
              {w.severity === 'info' ? 'border-blue-400 bg-blue-50 text-blue-900' : ''}"
          >
            <svg class="mt-0.5 h-5 w-5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
            <div class="flex-1">
              <p class="text-sm">{w.message?.[state.pageSchema.ui.primary_language] ?? w.message?.en ?? ''}</p>
              {#if w.action_href}
                <a
                  href={w.action_href}
                  class="mt-2 inline-block text-sm font-medium underline hover:no-underline"
                >
                  {w.action_label?.[state.pageSchema.ui.primary_language] ?? w.action_label?.en ?? ''}
                </a>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}

    <TabBar
      tabs={state.pageSchema.tabs}
      activeTabId={state.activeTabId}
      onTabClick={handleTabClick}
      isActiveBranch={(t) => state.isActiveBranch(t)}
    />

    <div>
      {#if activeTab && state.isEntityListTab(activeTab) && activeTab.content_url}
        {#key `${activeTab.id}:${state.listReset}`}
          <EntityListView
            schemaUrl={activeTab.content_url}
            onmutate={() => state.refreshCounts()}
            initialItemId={state.readHashItem() ?? ''}
            onitemchange={(id) => state.writeHashItem(id)}
          />
        {/key}
      {:else if state.tabLoading.get(state.activeTabId)}
        <LoadingState message="Loading {activeTab?.id || ''}..." />
      {:else if state.tabErrors.get(state.activeTabId)}
        <div class="bg-red-50 border border-red-200 rounded-lg p-4">
          <p class="text-red-700 text-sm">{state.tabErrors.get(state.activeTabId)}</p>
        </div>
      {:else if activeContent}
        {#if contentKind(activeContent) === 'overview'}
          <ProjectOverview schema={activeContent as ProjectOverviewSchema} />
        {:else if contentKind(activeContent) === 'adoptions'}
          <ProjectAdoptions schema={activeContent as ProjectAdoptionsTabSchema} />
        {:else if contentKind(activeContent) === 'releases'}
          <ProjectReleases schema={activeContent as ProjectReleaseTabSchema} />
        {:else}
          <div class="bg-yellow-50 border border-yellow-200 rounded p-4 text-sm text-yellow-800">
            Unknown content type for tab "{state.activeTabId}"
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
