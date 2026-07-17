<script lang="ts">
  import type { EntityRowLayout, RowAction } from '$lib/types/entity-list-schema';
  import { evalVisibleWhen } from '$lib/types/entity-list-schema';
  import type { PathElement } from '$lib/types/weave-types';
  import EntityListBadge from './EntityListBadge.svelte';
  import RowActionsKebab from './RowActionsKebab.svelte';
  import PathBadges from './PathBadges.svelte';

  let {
    item,
    rowLayout,
    detailUrlTemplate,
    editUrlTemplate,
    projectId,
    releaseVersion = '',
    lang,
    viewMode = 'detailed',
    rowActions = [],
    onAction,
    onEdit,
  }: {
    item: Record<string, any>;
    rowLayout: EntityRowLayout;
    detailUrlTemplate: string;
    editUrlTemplate?: string;
    projectId: string;
    releaseVersion?: string;
    lang: string;
    viewMode?: string;
    rowActions?: RowAction[];
    onAction?: (actionId: string, item: Record<string, any>) => void;
    onEdit?: (id: string) => void;
  } = $props();

  // Ontology path (fields only): rendered on its own line in detailed view
  // when the schema names the field via row_layout.path_field.
  let pathElements = $derived(
    rowLayout.path_field ? ((item[rowLayout.path_field] as PathElement[] | undefined) ?? []) : [],
  );

  function getTranslatedText(value: any, fallback: string = ''): string {
    if (!value) return fallback;
    if (typeof value === 'string') return value;
    if (typeof value === 'object') {
      return value[lang] || value['en'] || Object.values(value)[0] || fallback;
    }
    return fallback;
  }

  function buildUrl(template: string, id: string): string {
    return template.replace('{id}', id);
  }

  function appendVersion(raw: string): string {
    if (!releaseVersion || !raw) return raw;
    try {
      const url = new URL(raw, window.location.origin);
      url.searchParams.set('version', releaseVersion);
      return `${url.pathname}${url.search}${url.hash}`;
    } catch {
      return raw;
    }
  }

  function navigateToDetail() {
    if (detailUrl) {
      window.location.href = detailUrl;
      return;
    }
    if (onEdit && itemId) {
      onEdit(itemId);
    }
  }

  let itemId = $derived(item.id || item.ID || '');
  let detailUrl = $derived(detailUrlTemplate ? appendVersion(buildUrl(detailUrlTemplate, itemId)) : '');
  let editUrl = $derived(editUrlTemplate ? appendVersion(buildUrl(editUrlTemplate, itemId)) : '');
  let canEdit = $derived(() => {
    const itemProjectId = item.project_id || '';
    return !!onEdit && (!itemProjectId || itemProjectId === projectId);
  });
  let visibleActions = $derived(rowActions.filter((action) => evalVisibleWhen(action.visible_when, item)));

  let title = $derived(getTranslatedText(item[rowLayout.title_field], 'Unnamed'));
  let subtitle = $derived(getTranslatedText(item[rowLayout.subtitle_field], ''));

  // Lead box — a prominent box before the title (ontology
  // scope class). Palette mirrors EntityListBadge's named styles.
  const leadBoxPalette: Record<string, string> = {
    purple: 'bg-purple-50 text-purple-800 border-purple-200',
    blue: 'bg-blue-50 text-blue-800 border-blue-200',
    green: 'bg-green-50 text-green-800 border-green-200',
    gray: 'bg-gray-50 text-gray-700 border-gray-200',
    amber: 'bg-amber-50 text-amber-900 border-amber-200',
  };
  let leadBoxValue = $derived(
    rowLayout.lead_box ? getTranslatedText(item[rowLayout.lead_box.key], '') : '',
  );
  let leadBoxClass = $derived(
    leadBoxPalette[rowLayout.lead_box?.style ?? 'purple'] ?? leadBoxPalette.purple,
  );
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="row-compact {detailUrl || onEdit ? 'cursor-pointer hover:bg-gray-50' : ''} transition-colors duration-150"
  onclick={detailUrl || onEdit ? navigateToDetail : undefined}
>
  <div class="px-4 py-4 sm:px-6">
    <!-- Scope-class box leads the whole row as a left column, vertically
         centred beside all lines — not inline in the title. -->
    <div class="flex items-center gap-3">
      {#if leadBoxValue}
        <span class="flex-shrink-0 px-2.5 py-2 rounded-md border text-xs font-mono font-medium {leadBoxClass}">
          {leadBoxValue}
        </span>
      {/if}
      <div class="flex-1 min-w-0">
    <!-- Line 1: Title + Identity + Date -->
    <div class="flex items-center justify-between">
      <div class="flex items-center min-w-0 gap-2">
        {#if detailUrl}
          <a
            href={detailUrl}
            onclick={(e: MouseEvent) => e.stopPropagation()}
            class="title-link block truncate text-base font-semibold text-gray-900 transition-colors hover:underline"
          >{title}</a>
        {:else}
          <p class="title-link text-base font-semibold text-gray-900 truncate transition-colors">{title}</p>
        {/if}
        {#each rowLayout.identity_fields as field}
          {#if item[field.key]}
            <span class="flex-shrink-0 px-2 py-0.5 rounded text-xs {field.style === 'mono' ? 'font-mono text-gray-500 bg-gray-50' : 'font-mono text-gray-500 bg-gray-100'}">
              {item[field.key]}
            </span>
          {/if}
        {/each}
      </div>
      <div class="flex items-center gap-1 flex-shrink-0 ml-2">
        {#if item.updated_at}
          <span class="text-xs text-gray-400">
            {new Date(item.updated_at).toLocaleDateString()}
          </span>
        {/if}
        {#if visibleActions.length > 0}
          <RowActionsKebab actions={visibleActions} {item} {lang} {onAction} />
        {:else}
          {#if canEdit() && editUrl}
            <a
              href={editUrl}
              onclick={(e: MouseEvent) => e.stopPropagation()}
              class="p-1.5 text-gray-400 hover:text-pletka-primary rounded-md hover:bg-gray-100"
              title="Edit"
            >
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
              </svg>
            </a>
          {/if}
          {#if detailUrl}
            <svg class="h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
            </svg>
          {/if}
        {/if}
      </div>
    </div>

    <!-- Line 2: Subtitle -->
    {#if subtitle}
      <div class="mt-1">
        <p class="text-sm text-gray-500 truncate">{subtitle}</p>
      </div>
    {/if}

    <!-- Optional ontology-path row (fields, detailed view) -->
    {#if viewMode === 'detailed' && pathElements.length > 0}
      <div class="mt-2">
        <PathBadges elements={pathElements} />
      </div>
    {/if}

    <!-- Line 3: Semantic badges (left) + Process badges (right) -->
    <div class="mt-2 flex items-center justify-between">
      <div class="flex items-center gap-1.5 flex-wrap">
        {#each rowLayout.semantic_badges as badge}
          <EntityListBadge config={badge} {item} {projectId} {lang} />
        {/each}
      </div>
      <div class="flex items-center gap-1.5 flex-shrink-0">
        {#each rowLayout.process_badges as badge}
          <EntityListBadge config={badge} {item} {projectId} {lang} />
        {/each}
      </div>
    </div>
      </div>
    </div>
  </div>
</div>

<style>
  .row-compact:hover :global(.title-link) { color: var(--pletka-secondary, #6aaeaa); }
</style>
