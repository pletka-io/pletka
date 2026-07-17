<!--
  ReuseTab: shows how this entity relates to models/collections, split into two
  relationship tabs so the two directions are never conflated:

  - "Included in" (membership): the entity is a member of / bundled into the
    listed models/collections. Fields (via overrides) and collections (via
    part_of_collection) have this.
  - "Referenced by" (value-target): a field in the listed models/collections
    points at this entity as its expected value type (weave_override_refs).
    Models and collections have this.

  Which tabs appear is driven purely by the backend payload: field →
  included_in only; model → referenced_by only; collection → both. Each tab
  keeps the This-project / Other-projects scope sub-tabs and the
  models/collections column split.
-->
<script lang="ts">
  import type { EntityViewState } from '$lib/detailview/state.svelte';
  import type { FieldUsageRef, ReuseSection } from '$lib/detailview/types';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import { onMount } from 'svelte';

  // NOTE: alias the prop to `viewState`. A local identifier named `state`
  // collides with the `$state` rune below — the compiler reads `$state` as a
  // store auto-subscribe of `state`, throwing "s.subscribe is not a function".
  let {
    state: viewState,
    entityType,
  }: {
    state: EntityViewState;
    entityType: string;
  } = $props();

  onMount(() => {
    viewState.loadReuse();
  });

  const lang = getUILang();
  const reuse = $derived(viewState.reuse);
  const loading = $derived(viewState.reuseLoading);

  const noun = $derived(entityType === 'concept-list' ? 'concept list' : entityType);

  type Rel = 'included' | 'referenced';
  interface RelTab {
    key: Rel;
    label: string;
    sec: ReuseSection;
    count: number;
  }

  function sectionCount(sec: ReuseSection): number {
    const here = (sec.models?.length ?? 0) + (sec.collections?.length ?? 0);
    const cross = (sec.other_projects ?? []).reduce(
      (n, g) => n + (g.models?.length ?? 0) + (g.collections?.length ?? 0),
      0,
    );
    return here + cross;
  }

  const relTabs = $derived(
    (
      [
        reuse?.included_in
          ? { key: 'included', label: 'Included in', sec: reuse.included_in }
          : null,
        reuse?.referenced_by
          ? { key: 'referenced', label: 'Referenced by', sec: reuse.referenced_by }
          : null,
      ].filter(Boolean) as Omit<RelTab, 'count'>[]
    ).map((t) => ({ ...t, count: sectionCount(t.sec) })),
  );

  const grandTotal = $derived(relTabs.reduce((n, t) => n + t.count, 0));

  let activeRel = $state<Rel>('included');
  // Keep the active relationship valid as payload loads (a field has no
  // "included" tab if it defaults there but only "referenced" exists, etc.).
  $effect(() => {
    if (relTabs.length && !relTabs.some((t) => t.key === activeRel)) {
      activeRel = relTabs[0].key;
    }
  });
  const active = $derived(relTabs.find((t) => t.key === activeRel) ?? relTabs[0] ?? null);

  const models = $derived(active?.sec.models ?? []);
  const collections = $derived(active?.sec.collections ?? []);
  const otherProjects = $derived(active?.sec.other_projects ?? []);

  const thisProjectCount = $derived(models.length + collections.length);
  const crossCount = $derived(
    otherProjects.reduce((n, g) => n + (g.models?.length ?? 0) + (g.collections?.length ?? 0), 0),
  );

  // Scope sub-tab (This project / Other projects). Reset to 'project' whenever
  // the relationship tab changes so a stale 'cross' scope never sticks.
  let scope = $state<'project' | 'cross'>('project');
  $effect(() => {
    activeRel;
    scope = 'project';
  });

  const verb = $derived(activeRel === 'included' ? 'including' : 'referencing');
  const modelsHeader = $derived(`Models ${verb} this ${noun}`);
  const collectionsHeader = $derived(`Collections ${verb} this ${noun}`);

  function rowName(ref: FieldUsageRef): string {
    return tr(ref.name, lang, ref.semantic_id || ref.id);
  }
  function rowDesc(ref: FieldUsageRef): string {
    if (!ref.description) return '';
    return tr(ref.description, lang, '');
  }
</script>

{#snippet column(header: string, refs: FieldUsageRef[], emptyText: string)}
  <div class="bg-white border border-gray-200 rounded-lg">
    <div class="px-5 py-3 border-b border-gray-200 flex items-center justify-between">
      <h3 class="text-sm font-semibold text-gray-700">{header}</h3>
      <span class="text-xs text-gray-500">{refs.length}</span>
    </div>
    {#if refs.length === 0}
      <div class="px-5 py-4 text-sm text-gray-400">{emptyText}</div>
    {:else}
      <ul class="divide-y divide-gray-100">
        {#each refs as ref (ref.id)}
          <li class="px-5 py-3">
            {#if ref.url}
              <a class="block group" href={ref.url}>
                <div class="flex items-center justify-between gap-2">
                  <span class="text-sm font-medium text-pletka-primary group-hover:underline">{rowName(ref)}</span>
                  {#if ref.semantic_id}
                    <span class="text-xs text-gray-400 font-mono">{ref.semantic_id}</span>
                  {/if}
                </div>
                {#if rowDesc(ref)}
                  <div class="mt-0.5 text-xs text-gray-500 line-clamp-2">{rowDesc(ref)}</div>
                {/if}
              </a>
            {:else}
              <span class="text-sm font-medium text-gray-700">{rowName(ref)}</span>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/snippet}

{#snippet columns(ms: FieldUsageRef[], cs: FieldUsageRef[])}
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
    {@render column(modelsHeader, ms, `No models ${verb} this ${noun}.`)}
    {@render column(collectionsHeader, cs, `No collections ${verb} this ${noun}.`)}
  </div>
{/snippet}

<div class="space-y-4">
  {#if loading && !reuse}
    <div class="bg-white border border-gray-200 rounded-lg p-6 text-center text-sm text-gray-500">
      Loading reuse&hellip;
    </div>
  {:else if grandTotal === 0}
    <div class="bg-white border border-gray-200 rounded-lg p-6 text-center text-sm text-gray-500">
      This {noun} is not yet referenced by any model or collection.
    </div>
  {:else if active}
    <!-- Relationship tabs (only those the entity type supports) -->
    {#if relTabs.length > 1}
      <div class="flex items-center gap-1 border-b border-gray-200">
        {#each relTabs as t (t.key)}
          <button
            type="button"
            onclick={() => (activeRel = t.key)}
            class="px-3 py-2 text-sm font-medium border-b-2 -mb-px {activeRel === t.key
              ? 'border-pletka-primary text-pletka-primary'
              : 'border-transparent text-gray-500 hover:text-gray-700'}"
          >
            {t.label} <span class="ml-1 text-xs text-gray-400">{t.count}</span>
          </button>
        {/each}
      </div>
    {/if}

    <!-- Scope sub-tabs (only when cross-project usage exists) -->
    {#if crossCount > 0}
      <div class="flex items-center gap-1 border-b border-gray-100">
        <button
          type="button"
          onclick={() => (scope = 'project')}
          class="px-3 py-2 text-sm font-medium border-b-2 -mb-px {scope === 'project'
            ? 'border-pletka-primary text-pletka-primary'
            : 'border-transparent text-gray-500 hover:text-gray-700'}"
        >
          This project <span class="ml-1 text-xs text-gray-400">{thisProjectCount}</span>
        </button>
        <button
          type="button"
          onclick={() => (scope = 'cross')}
          class="px-3 py-2 text-sm font-medium border-b-2 -mb-px {scope === 'cross'
            ? 'border-pletka-primary text-pletka-primary'
            : 'border-transparent text-gray-500 hover:text-gray-700'}"
        >
          Other projects
          <span class="ml-1 text-xs text-gray-400">{crossCount} in {otherProjects.length}</span>
        </button>
      </div>
    {/if}

    {#if scope === 'project' || crossCount === 0}
      {@render columns(models, collections)}
    {:else}
      <div class="space-y-6">
        {#each otherProjects as g (g.project_id)}
          <div>
            <div class="mb-2 flex items-center gap-2">
              <span class="inline-flex items-center rounded bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700">
                {g.project_name || g.project_id}
              </span>
              <span class="text-xs text-gray-400 font-mono">{g.project_id}</span>
            </div>
            {@render columns(g.models ?? [], g.collections ?? [])}
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
