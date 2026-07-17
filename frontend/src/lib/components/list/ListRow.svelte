<script lang="ts">
  import { evalVisibleWhen, type Column, type RowAction } from '$lib/types/list-schema';
  import type { LanguageInfo } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    item,
    columns,
    actions,
    lang,
    languages,
    renamingId = null,
    hideActions = false,
    reorderable = true,
    onaction,
    onrename,
    onrenamestart,
    onrenamecancel,
  }: {
    item: any;
    columns: Column[];
    actions: RowAction[];
    lang: string;
    languages: LanguageInfo[];
    renamingId: string | null;
    hideActions?: boolean;
    reorderable?: boolean;
    onaction: (actionId: string, item: any) => void;
    onrename: (item: any, value: Record<string, string>) => void;
    onrenamestart: (itemId: string) => void;
    onrenamecancel: () => void;
  } = $props();

  let renameValue = $state('');
  let renameInput = $state<HTMLInputElement | null>(null);

  let isRenaming = $derived(renamingId === item.id);

  $effect(() => {
    if (isRenaming && renameInput) {
      renameInput.focus();
      renameInput.select();
    }
  });

  function startRename() {
    const primary = columns.find((c) => c.primary);
    if (!primary) return;
    const val = item[primary.key];
    renameValue = typeof val === 'object' ? tr(val, lang) : String(val ?? '');
    onrenamestart(item.id);
  }

  function commitRename() {
    const primary = columns.find((c) => c.primary);
    if (!primary || !renameValue.trim()) {
      onrenamecancel();
      return;
    }
    const val = typeof item[primary.key] === 'object'
      ? { ...item[primary.key], [lang]: renameValue.trim() }
      : renameValue.trim();
    onrename(item, val);
  }

  function handleRenameKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      commitRename();
    } else if (e.key === 'Escape') {
      onrenamecancel();
    }
  }

  function computeValue(col: Column, item: any): number {
    if (!col.compute) return 0;
    // Simple parser: "field_a + field_b"
    const parts = col.compute.split('+').map((p) => p.trim());
    return parts.reduce((sum, key) => sum + (Number(item[key]) || 0), 0);
  }

  const iconPaths: Record<string, string> = {
    pencil: 'M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z',
    'chart-bar': 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z',
    trash: 'M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16',
    'archive-box': 'M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4',
    'arrow-uturn-up': 'M9 14l-4-4m0 0l4-4m-4 4h11a4 4 0 014 4v6',
  };

  const badgeColors: Record<string, string> = {
    blue: 'bg-blue-100 text-blue-800',
    purple: 'bg-purple-50 text-purple-600',
    green: 'bg-green-100 text-green-800',
    gray: 'bg-gray-100 text-gray-500',
    amber: 'bg-amber-100 text-amber-800',
    red: 'bg-red-100 text-red-800',
  };
</script>

<div class="flex items-center px-4 py-3 hover:bg-gray-50 group">
  {#if reorderable}
    <!-- Drag Handle: only rendered when the parent list supports reorder. -->
    <div class="drag-handle cursor-grab active:cursor-grabbing flex-shrink-0 mr-3 text-gray-400 hover:text-gray-600">
      <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
        <path d="M7 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4z"/>
      </svg>
    </div>
  {/if}

  <!-- Content columns -->
  <div class="flex-1 min-w-0">
    {#each columns as col}
      {#if col.primary}
        {#if isRenaming}
          <input
            bind:this={renameInput}
            bind:value={renameValue}
            onkeydown={handleRenameKeydown}
            onblur={commitRename}
            class="text-sm font-medium text-gray-900 border border-pletka-primary rounded px-2 py-0.5 w-full focus:outline-none focus:ring-1 focus:ring-pletka-primary"
          />
        {:else}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <p class="text-sm font-medium text-gray-900 truncate cursor-text hover:text-pletka-primary"
             onclick={startRename}>
            {typeof item[col.key] === 'object' ? tr(item[col.key], lang) : item[col.key]}
          </p>
        {/if}
      {:else if col.secondary}
        {#if item[col.key]}
          <p class="text-xs text-gray-500 mt-0.5 truncate group-hover:whitespace-normal group-hover:line-clamp-3">
            {typeof item[col.key] === 'object' ? tr(item[col.key], lang) : item[col.key]}
          </p>
        {/if}
      {/if}
    {/each}
  </div>

  <!-- Badge columns -->
  <div class="flex items-center space-x-2 ml-4 flex-shrink-0">
    {#each columns as col}
      {#if col.type === 'badge'}
        {@const val = Number(item[col.key]) || 0}
        {@const style = val === 0 && col.zero_style ? col.zero_style : col.badge_style || 'gray'}
        <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {badgeColors[style] || badgeColors.gray}">
          {val} {tr(col.label, lang)}
        </span>
      {:else if col.type === 'text_badge'}
        {@const raw = item[col.key]}
        {#if raw}
          {@const valStr = String(raw)}
          {@const style = (col.badge_style_by_value && col.badge_style_by_value[valStr]) || col.badge_style || 'gray'}
          <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {badgeColors[style] || badgeColors.gray}">
            {valStr}
          </span>
        {/if}
      {:else if col.type === 'computed_badge'}
        {@const val = computeValue(col, item)}
        {#if !col.hide_zero || val > 0}
          <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {badgeColors[col.badge_style || 'gray']}">
            {val} {tr(col.label, lang)}
          </span>
        {/if}
      {:else if col.type === 'deprecated_badge'}
        {#if item[col.key]}
          <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {badgeColors[col.badge_style || 'amber'] || badgeColors.gray}"
                title="This entity is deprecated. Existing references stay intact; new connections aren't allowed.">
            {tr(col.label, lang) || 'Deprecated'}
          </span>
        {/if}
      {:else if col.type === 'boolean_badge'}
        {#if item[col.key]}
          <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {badgeColors[col.badge_style || 'blue'] || badgeColors.gray}">
            {tr(col.label, lang)}
          </span>
        {/if}
      {/if}
    {/each}
  </div>

  <!-- Action buttons -->
  {#if !hideActions}
    <div class="flex items-center space-x-1 ml-4 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
      {#each actions as action}
        {#if evalVisibleWhen(action.visible_when, item as Record<string, unknown>)}
          <button type="button"
            onclick={() => onaction(action.id, item)}
            class="p-1.5 rounded-md text-gray-400 {action.style === 'danger' ? 'hover:text-red-600 hover:bg-red-50' : 'hover:text-blue-600 hover:bg-blue-50'}"
            title={tr(action.label, lang)}
            data-row-action={action.id}>
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={iconPaths[action.icon] || ''} />
            </svg>
          </button>
        {/if}
      {/each}
    </div>
  {/if}
</div>
