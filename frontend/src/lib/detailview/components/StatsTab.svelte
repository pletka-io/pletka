<script lang="ts">
  import type { EntityViewState } from '$lib/detailview/state.svelte';
  import type { StatItem } from '$lib/detailview/types';

  import { onMount } from 'svelte';

  let {
    state,
    entityType = '',
  }: {
    state: EntityViewState;
    entityType?: string;
  } = $props();

  const isField = $derived(entityType === 'field');

  onMount(() => {
    state.loadStats();
  });

  const stats = $derived(state.stats);

  // Client-side computed stats for required/optional and value types
  const clientStats = $derived(() => {
    const sections = state.response?.sections ?? [];
    const totalFields = state.totalFieldCount;

    const valueTypeCounts: Record<string, number> = {};
    const requiredCount = { required: 0, optional: 0 };

    for (const section of sections) {
      for (const item of section.items) {
        for (const f of item.fields) {
          const vt = f.expected_value_type || 'Unknown';
          valueTypeCounts[vt] = (valueTypeCounts[vt] || 0) + 1;
          if (f.is_required) requiredCount.required++;
          else requiredCount.optional++;
        }
      }
    }

    const valueTypes = Object.entries(valueTypeCounts)
      .sort(([, a], [, b]) => b - a);

    return { totalFields, valueTypes, requiredCount };
  });

  function maxCount(items: StatItem[]): number {
    if (!items || items.length === 0) return 1;
    return Math.max(...items.map(i => i.count), 1);
  }

  function splitId(id: string): { prefix: string; name: string } {
    const colonIdx = id.indexOf(':');
    if (colonIdx > 0) {
      return { prefix: id.slice(0, colonIdx), name: id.slice(colonIdx + 1) };
    }
    return { prefix: '', name: id };
  }
</script>

<div class="space-y-6">
  <!-- Summary Cards -->
  {#if isField}
    <!-- Field-specific stats: usage of this field within the project. -->
    <div class="grid grid-cols-2 sm:grid-cols-3 gap-4">
      <div class="bg-purple-50 border border-purple-100 rounded-lg p-4">
        <div class="flex items-center mb-2">
          <svg class="h-5 w-5 text-purple-500 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
          </svg>
          <span class="text-sm font-medium text-purple-600">Models using</span>
        </div>
        <div class="text-3xl font-bold text-purple-700">{stats?.models_using ?? 0}</div>
      </div>
      <div class="bg-green-50 border border-green-100 rounded-lg p-4">
        <div class="flex items-center mb-2">
          <svg class="h-5 w-5 text-green-500 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
          <span class="text-sm font-medium text-green-600">Collections using</span>
        </div>
        <div class="text-3xl font-bold text-green-700">{stats?.collections_using ?? 0}</div>
      </div>
      <div class="bg-amber-50 border border-amber-100 rounded-lg p-4">
        <div class="flex items-center mb-2">
          <svg class="h-5 w-5 text-amber-500 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5h2a2 2 0 012 2v3a2 2 0 002 2h3a2 2 0 012 2v2M9 21V9a2 2 0 012-2h2a2 2 0 012 2v12M5 21h14M5 7h0M5 11h0M5 15h0M5 19h0" />
          </svg>
          <span class="text-sm font-medium text-amber-600">Override rows</span>
        </div>
        <div class="text-3xl font-bold text-amber-700">{stats?.field_override_count ?? 0}</div>
      </div>
    </div>
  {:else}
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
      <div class="bg-purple-50 border border-purple-100 rounded-lg p-4">
        <div class="flex items-center mb-2">
          <svg class="h-5 w-5 text-purple-500 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
          </svg>
          <span class="text-sm font-medium text-purple-600">Categories</span>
        </div>
        <div class="text-3xl font-bold text-purple-700">{stats?.total_categories ?? 0}</div>
      </div>
      <div class="bg-green-50 border border-green-100 rounded-lg p-4">
        <div class="flex items-center mb-2">
          <svg class="h-5 w-5 text-green-500 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
          <span class="text-sm font-medium text-green-600">Collections</span>
        </div>
        <div class="text-3xl font-bold text-green-700">{stats?.total_collections ?? 0}</div>
      </div>
      <div class="bg-blue-50 border border-blue-100 rounded-lg p-4">
        <div class="flex items-center mb-2">
          <svg class="h-5 w-5 text-blue-500 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7" />
          </svg>
          <span class="text-sm font-medium text-blue-600">Fields</span>
        </div>
        <div class="text-3xl font-bold text-blue-700">{stats?.total_fields ?? 0}</div>
      </div>
      <div class="bg-orange-50 border border-orange-100 rounded-lg p-4">
        <div class="flex items-center mb-2">
          <svg class="h-5 w-5 text-orange-500 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
          </svg>
          <span class="text-sm font-medium text-orange-600">Ontology Scopes</span>
        </div>
        <div class="text-3xl font-bold text-orange-700">{stats?.scopes_count ?? 0}</div>
      </div>
    </div>
  {/if}

  {#if !isField}
  <!-- Required vs Optional + Value Types - two columns -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
    <!-- Required vs Optional -->
    <div class="bg-white shadow-sm border border-gray-200 rounded-lg p-5">
      <h4 class="text-sm font-semibold text-gray-700 mb-3">Required vs Optional</h4>
      <div class="flex items-center space-x-4">
        <div class="flex-1 bg-gray-200 rounded-full h-3 overflow-hidden">
          {#if clientStats().totalFields > 0}
            <div
              class="bg-red-400 h-full rounded-full"
              style="width: {(clientStats().requiredCount.required / clientStats().totalFields * 100).toFixed(1)}%"
            ></div>
          {/if}
        </div>
        <div class="flex items-center space-x-4 text-sm flex-shrink-0">
          <span class="flex items-center">
            <span class="w-3 h-3 rounded-full bg-red-400 mr-1.5"></span>
            Required: {clientStats().requiredCount.required}
          </span>
          <span class="flex items-center">
            <span class="w-3 h-3 rounded-full bg-gray-300 mr-1.5"></span>
            Optional: {clientStats().requiredCount.optional}
          </span>
        </div>
      </div>
    </div>

    <!-- Value Types -->
    {#if clientStats().valueTypes.length > 0}
      <div class="bg-white shadow-sm border border-gray-200 rounded-lg p-5">
        <h4 class="text-sm font-semibold text-gray-700 mb-3">Value Types</h4>
        <div class="space-y-2 max-h-48 overflow-y-auto">
          {#each clientStats().valueTypes as [type, count]}
            <div class="flex items-center justify-between">
              <span class="text-sm text-gray-700 min-w-0 truncate">{type}</span>
              <div class="flex items-center ml-4">
                <div class="w-28 bg-gray-200 rounded-full h-2 mr-3">
                  <div
                    class="bg-indigo-400 h-full rounded-full"
                    style="width: {(count / clientStats().totalFields * 100).toFixed(1)}%"
                  ></div>
                </div>
                <span class="text-sm text-gray-500 w-8 text-right">{count}</span>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>

  <!-- Breakdown sections - two column grid of cards -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
    <!-- Patterns by Category -->
    {#if stats?.categories_breakdown && stats.categories_breakdown.length > 0}
      <div class="bg-white shadow-sm border border-gray-200 rounded-lg p-5">
        <h4 class="text-sm font-semibold text-gray-700 mb-3">
          Patterns by Category
          <span class="text-gray-400 font-normal ml-1">({stats.categories_breakdown.length})</span>
        </h4>
        <div class="max-h-56 overflow-y-auto space-y-1.5">
          {#each stats.categories_breakdown as item}
            <div class="flex items-center justify-between">
              <span class="text-sm text-gray-700 truncate min-w-0 flex-1">{item.name || item.id}</span>
              <div class="flex items-center ml-3 flex-shrink-0">
                <div class="w-24 bg-gray-100 rounded-full h-2.5 mr-2">
                  <div class="bg-purple-500 h-full rounded-full" style="width: {item.percentage}%"></div>
                </div>
                <span class="text-xs text-gray-500 w-7 text-right">{item.count}</span>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <!-- Field Usage -->
    {#if stats?.fields_breakdown && stats.fields_breakdown.length > 0}
      <div class="bg-white shadow-sm border border-gray-200 rounded-lg p-5">
        <h4 class="text-sm font-semibold text-gray-700 mb-3">
          Field Usage
          <span class="text-gray-400 font-normal ml-1">({stats.fields_breakdown.length})</span>
        </h4>
        <div class="max-h-56 overflow-y-auto space-y-1.5">
          {#each stats.fields_breakdown as item}
            <div class="flex items-center justify-between">
              <span class="text-sm text-gray-700 truncate min-w-0 flex-1">{item.name || item.id.slice(0, 8)}</span>
              <div class="flex items-center ml-3 flex-shrink-0">
                <div class="w-24 bg-gray-100 rounded-full h-2.5 mr-2">
                  <div class="bg-blue-500 h-full rounded-full" style="width: {(item.count / maxCount(stats.fields_breakdown) * 100)}%"></div>
                </div>
                <span class="text-xs text-gray-500 w-7 text-right">{item.count}</span>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <!-- Field Ontology Scopes -->
    {#if stats?.field_scopes_breakdown && stats.field_scopes_breakdown.length > 0}
      <div class="bg-white shadow-sm border border-gray-200 rounded-lg p-5">
        <h4 class="text-sm font-semibold text-gray-700 mb-3">
          Field Ontology Scopes
          <span class="text-gray-400 font-normal ml-1">({stats.field_scopes_breakdown.length})</span>
        </h4>
        <div class="max-h-56 overflow-y-auto space-y-1.5">
          {#each stats.field_scopes_breakdown as item}
            <div class="flex items-center justify-between">
              <span class="text-sm text-gray-700 truncate min-w-0 flex-1">{item.name}</span>
              <div class="flex items-center ml-3 flex-shrink-0">
                <div class="w-24 bg-gray-100 rounded-full h-2.5 mr-2">
                  <div class="bg-orange-500 h-full rounded-full" style="width: {item.percentage}%"></div>
                </div>
                <span class="text-xs text-gray-500 w-7 text-right">{item.count}</span>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <!-- Collection Ontology Scopes -->
    {#if stats?.coll_scopes_breakdown && stats.coll_scopes_breakdown.length > 0}
      <div class="bg-white shadow-sm border border-gray-200 rounded-lg p-5">
        <h4 class="text-sm font-semibold text-gray-700 mb-3">
          Collection Ontology Scopes
          <span class="text-gray-400 font-normal ml-1">({stats.coll_scopes_breakdown.length})</span>
        </h4>
        <div class="max-h-56 overflow-y-auto space-y-1.5">
          {#each stats.coll_scopes_breakdown as item}
            <div class="flex items-center justify-between">
              <span class="text-sm text-gray-700 truncate min-w-0 flex-1">{item.name}</span>
              <div class="flex items-center ml-3 flex-shrink-0">
                <div class="w-24 bg-gray-100 rounded-full h-2.5 mr-2">
                  <div class="bg-green-500 h-full rounded-full" style="width: {item.percentage}%"></div>
                </div>
                <span class="text-xs text-gray-500 w-7 text-right">{item.count}</span>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <!-- Used Classes -->
    {#if stats?.classes_breakdown && stats.classes_breakdown.length > 0}
      <div class="bg-white shadow-sm border border-gray-200 rounded-lg p-5">
        <h4 class="text-sm font-semibold text-gray-700 mb-3">
          Used Classes
          <span class="text-gray-400 font-normal ml-1">({stats.classes_breakdown.length})</span>
        </h4>
        <div class="max-h-56 overflow-y-auto space-y-1.5">
          {#each stats.classes_breakdown as item}
            {@const parts = splitId(item.id)}
            <div class="flex items-center justify-between">
              <span class="text-sm min-w-0 truncate flex-1">
                {#if parts.prefix}
                  <span class="text-gray-400">{parts.prefix}:</span>
                {/if}
                <span class="text-gray-700 font-medium">{parts.name}</span>
              </span>
              <div class="flex items-center ml-3 flex-shrink-0">
                <div class="w-24 bg-gray-100 rounded-full h-2.5 mr-2">
                  <div class="bg-purple-400 h-full rounded-full" style="width: {item.percentage}%"></div>
                </div>
                <span class="text-xs text-gray-500 w-7 text-right">{item.count}</span>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <!-- Properties -->
    {#if stats?.properties_breakdown && stats.properties_breakdown.length > 0}
      <div class="bg-white shadow-sm border border-gray-200 rounded-lg p-5">
        <h4 class="text-sm font-semibold text-gray-700 mb-3">
          Properties
          <span class="text-gray-400 font-normal ml-1">({stats.properties_breakdown.length})</span>
        </h4>
        <div class="max-h-56 overflow-y-auto space-y-1.5">
          {#each stats.properties_breakdown as item}
            {@const parts = splitId(item.id)}
            <div class="flex items-center justify-between">
              <span class="text-sm min-w-0 truncate flex-1">
                {#if parts.prefix}
                  <span class="text-gray-400">{parts.prefix}:</span>
                {/if}
                <span class="text-gray-700 font-medium">{parts.name}</span>
              </span>
              <div class="flex items-center ml-3 flex-shrink-0">
                <div class="w-24 bg-gray-100 rounded-full h-2.5 mr-2">
                  <div class="bg-blue-400 h-full rounded-full" style="width: {item.percentage}%"></div>
                </div>
                <span class="text-xs text-gray-500 w-7 text-right">{item.count}</span>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
  {/if}
</div>
