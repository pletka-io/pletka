<!-- frontend/src/lib/detailview/components/FieldOverride.svelte -->
<script lang="ts">
  import type { ViewField } from '$lib/detailview/types';
  import { tr, type OriginInfo } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import { pathElementsToDisplay, parseOntologyPath, formatPathForCopy } from '$lib/utils/ontology-path';
  import type { BadgeVariant } from '$lib/utils/field-display';
  import Badge from '$lib/components/shared/Badge.svelte';
  import PathPreview from './PathPreview.svelte';

  let {
    field,
    state,
    sharedPrefixLength = 0,
  }: {
    field: ViewField;
    state: {
      viewMode: 'compact' | 'detailed';
      expandedFields: Set<string>;
      projectId: string;
      toggleField: (id: string) => void;
      // Field-search bits — optional so this component still works in
      // contexts (e.g. embedded previews) that don't wire search.
      searchQuery?: string;
      matchSet?: Set<number>;
      activeOverrideID?: number | null;
    };
    sharedPrefixLength?: number;
  } = $props();

  const lang = getUILang();
  const fieldName = $derived(tr(field.display_name, lang, field.field_semantic_id));
  const description = $derived(tr(field.description, lang, ''));
  const fieldKey = $derived(String(field.override_id));
  const isExpanded = $derived(state.expandedFields.has(fieldKey));

  const parsedPath = $derived(
    field.path_elements && field.path_elements.length > 0
      ? pathElementsToDisplay(field.path_elements)
      : parseOntologyPath(field.ontology_path || ''),
  );
  const displayPath = $derived(
    sharedPrefixLength > 0 ? parsedPath.slice(sharedPrefixLength) : parsedPath,
  );

  const typeColors: Record<string, BadgeVariant> = {
    String: 'gray',
    URI: 'blue',
    Model: 'purple',
    Collection: 'green',
    Concept: 'indigo',
    Date: 'yellow',
    Integer: 'yellow',
    GeoJson: 'green',
  };

  function handleToggle() {
    if (state.viewMode === 'compact') {
      state.toggleField(fieldKey);
    }
  }

  // When search is active, hide rows that don't match. Active match
  // gets a yellow highlight (replaces the old .search-current global
  // class). State doesn't supply matchSet → no filter applied.
  const isSearchActive = $derived(!!state.searchQuery?.trim());
  const isFiltered = $derived(
    isSearchActive && !!state.matchSet && !state.matchSet.has(field.override_id),
  );
  const isActiveMatch = $derived(
    isSearchActive && state.activeOverrideID === field.override_id,
  );

  function inheritedOriginLabel(origin?: OriginInfo): string {
    if (!origin || origin.kind !== 'inherited') return '';
    return origin.source_project_label || origin.source_project_id || 'parent project';
  }
</script>

<div
  class="field-row group"
  data-field-id={field.field_id}
  data-override-id={field.override_id}
  class:hidden={isFiltered}
  class:bg-yellow-100={isActiveMatch}
>
  {#if state.viewMode === 'detailed'}
    <!-- Detailed: card layout -->
    <div class="px-4 py-3">
      <div class="flex items-start gap-2">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <h4 class="text-sm font-medium text-gray-900">{fieldName}</h4>
            {#if field.field_semantic_id}
              {#if field.source_field_url}
                <a
                  href={field.source_field_url}
                  class="inline-flex items-center px-1.5 py-0.5 rounded text-xs bg-gray-100 text-gray-600 hover:bg-pletka-primary/10 hover:text-pletka-primary"
                  title="Open source field"
                  onclick={(e) => e.stopPropagation()}
                >{field.field_semantic_id}</a>
              {:else}
                <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs bg-gray-100 text-gray-600">{field.field_semantic_id}</span>
              {/if}
            {/if}
            {#if field.collection_id && field.source_collection_url}
              <a
                href={field.source_collection_url}
                class="inline-flex items-center px-1.5 py-0.5 rounded text-xs bg-emerald-50 text-emerald-800 border border-emerald-200 hover:bg-emerald-100"
                title="Open source collection"
                onclick={(e) => e.stopPropagation()}
              >from {field.collection_id}</a>
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
            {#if field.is_required}
              <Badge variant="red" size="xs">Required</Badge>
            {/if}
            {#if field.origin?.kind === 'inherited'}
              <Badge variant="amber" size="xs">Inherited · {inheritedOriginLabel(field.origin)}</Badge>
            {/if}
          </div>
          {#if description}
            <p class="text-xs text-gray-500 mt-0.5 line-clamp-1">{description}</p>
          {/if}
          {#if displayPath.length > 0}
            <div class="mt-1"><PathPreview path={displayPath} /></div>
          {/if}
        </div>
      </div>
    </div>
  {:else}
    <!-- Compact: row with expand chevron -->
    <div
      class="flex items-center px-4 py-2.5 cursor-pointer hover:bg-gray-50 transition-colors"
      onclick={handleToggle}
      role="button"
      tabindex="0"
      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') handleToggle(); }}
    >
      <svg
        class="h-4 w-4 text-gray-400 mr-2.5 transition-transform flex-shrink-0"
        class:rotate-90={isExpanded}
        fill="none" viewBox="0 0 24 24" stroke="currentColor"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
      </svg>

      <span class="font-medium text-sm text-gray-900 truncate mr-3">{fieldName}</span>
      <div class="flex-1"></div>

      <div class="flex flex-wrap items-center gap-1 max-w-[50%] justify-end">
        {#if field.expected_value_type}
          <Badge variant={typeColors[field.expected_value_type] || 'gray'} size="xs">{field.expected_value_type}</Badge>
        {/if}
        {#if field.set_value}
          <span
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] bg-amber-50 text-amber-900 border border-amber-200 max-w-[180px] truncate"
            title={`Fixed value: ${field.set_value}`}
          >
            <span class="font-mono text-[10px] text-amber-600">=</span>
            <span class="font-mono truncate">{field.set_value}</span>
          </span>
        {/if}
        {#if field.collection_model_refs?.length}
          {#each field.collection_model_refs as ref}
            <a
              href={ref.url}
              class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs bg-green-100 text-green-800 hover:bg-green-200"
              onclick={(e) => e.stopPropagation()}
            >
              <span class="font-medium">{ref.semantic_id}</span>
              <span class="text-green-600">{tr(ref.name, lang, '')}</span>
              {#if ref.origin?.kind === 'inherited'}
                <span class="text-[10px] uppercase tracking-wide text-amber-700">inherited</span>
              {/if}
            </a>
          {/each}
        {/if}
        {#if field.resource_model_refs?.length}
          {#each field.resource_model_refs as ref}
            <a
              href={ref.url}
              class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs bg-purple-100 text-purple-800 hover:bg-purple-200"
              onclick={(e) => e.stopPropagation()}
            >
              <span class="font-medium">{ref.semantic_id}</span>
              <span class="text-purple-600">{tr(ref.name, lang, '')}</span>
              {#if ref.origin?.kind === 'inherited'}
                <span class="text-[10px] uppercase tracking-wide text-amber-700">inherited</span>
              {/if}
            </a>
          {/each}
        {/if}
        {#if field.is_required}
          <Badge variant="red" size="xs">Required</Badge>
        {/if}
      </div>
    </div>

    <!-- Expanded details (compact mode only) -->
    {#if isExpanded}
      <div class="px-4 pb-3 pl-11 text-sm space-y-1.5">
        {#if description}
          <p class="text-gray-600">{description}</p>
        {/if}
        {#if displayPath.length > 0}
          <div class="flex items-start">
            <span class="text-gray-400 w-24 flex-shrink-0">Path:</span>
            <PathPreview path={displayPath} />
            <button
              type="button"
              class="ml-2 p-1 text-gray-400 hover:text-gray-600 rounded flex-shrink-0"
              title="Copy full path"
              onclick={() => { navigator.clipboard.writeText(formatPathForCopy(field.ontology_path || '')); }}
            >
              <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
            </button>
          </div>
        {/if}
        {#if field.field_semantic_id}
          <div class="flex items-center">
            <span class="text-gray-400 w-24 flex-shrink-0">ID:</span>
            {#if field.source_field_url}
              <a
                href={field.source_field_url}
                class="text-pletka-primary hover:underline"
                title="Open source field"
                onclick={(e) => e.stopPropagation()}
              >{field.field_semantic_id}</a>
            {:else}
              <span class="text-gray-600">{field.field_semantic_id}</span>
            {/if}
          </div>
        {/if}
        {#if field.collection_id}
          <div class="flex items-center">
            <span class="text-gray-400 w-24 flex-shrink-0">From:</span>
            {#if field.source_collection_url}
              <a
                href={field.source_collection_url}
                class="text-pletka-primary hover:underline"
                title="Open source collection"
                onclick={(e) => e.stopPropagation()}
              >{field.collection_id}</a>
            {:else}
              <span class="text-gray-600">{field.collection_id}</span>
            {/if}
          </div>
        {/if}
        {#if field.expected_value_type}
          <div class="flex items-center">
            <span class="text-gray-400 w-24 flex-shrink-0">Type:</span>
            <Badge variant={typeColors[field.expected_value_type] || 'gray'} size="xs">{field.expected_value_type}</Badge>
          </div>
        {/if}
        {#if field.set_value}
          <div class="flex items-start">
            <span class="text-gray-400 w-24 flex-shrink-0">Set value:</span>
            <span
              class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] bg-amber-50 text-amber-900 border border-amber-200"
              title="Fixed value (set_value)"
            >
              <span class="font-mono text-[10px] text-amber-600">=</span>
              <span class="font-mono break-all">{field.set_value}</span>
            </span>
          </div>
        {/if}
        {#if field.resource_model_refs?.length}
          <div class="flex items-center">
            <span class="text-gray-400 w-24 flex-shrink-0">Model:</span>
            <div class="flex flex-wrap gap-1">
              {#each field.resource_model_refs as ref}
                <a href={ref.url} class="text-blue-600 hover:text-blue-800 hover:underline">
                  {tr(ref.name, lang, ref.semantic_id)}
                </a>
              {/each}
            </div>
          </div>
        {/if}
        {#if field.collection_model_refs?.length}
          <div class="flex items-center">
            <span class="text-gray-400 w-24 flex-shrink-0">Collection:</span>
            <div class="flex flex-wrap gap-1">
              {#each field.collection_model_refs as ref}
                <a href={ref.url} class="text-blue-600 hover:text-blue-800 hover:underline">
                  {tr(ref.name, lang, ref.semantic_id)}
                </a>
              {/each}
            </div>
          </div>
        {/if}
        {#if field.is_required}
          <div class="flex items-center">
            <span class="text-gray-400 w-24 flex-shrink-0">Required:</span>
            <span class="text-red-600">Yes</span>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>
