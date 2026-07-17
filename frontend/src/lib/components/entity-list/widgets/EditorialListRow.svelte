<!-- frontend/src/lib/components/entity-list/widgets/EditorialListRow.svelte -->
<script lang="ts">
  import type { RowWidgetProps } from '../row-widget-registry';
  import { evalVisibleWhen } from '$lib/types/entity-list-schema';
  import RowActionsKebab from '../RowActionsKebab.svelte';

  let props: RowWidgetProps = $props();

  const lang = $derived(props.lang);
  const item = $derived(props.item);
  const layout = $derived(props.rowLayout);

  function getTranslated(value: unknown, fallback = ''): string {
    if (!value) return fallback;
    if (typeof value === 'string') return value;
    if (typeof value === 'object') {
      const rec = value as Record<string, string>;
      return rec[lang] || rec.en || Object.values(rec)[0] || fallback;
    }
    return fallback;
  }

  function buildUrl(template: string, id: string): string {
    return template.replace('{id}', id);
  }

  const itemId = $derived(item.id || item.ID || '');
  const detailUrl = $derived(props.detailUrlTemplate ? buildUrl(props.detailUrlTemplate, itemId) : '');
  const visibleActions = $derived((props.rowActions ?? []).filter((action) => evalVisibleWhen(action.visible_when, item)));

  const title = $derived(getTranslated(item[layout.title_field], 'Unnamed'));
  const description = $derived(getTranslated(item[layout.subtitle_field], ''));
  const byline = $derived(
    layout.byline_field ? String(item[layout.byline_field] ?? '') : '',
  );
  const idChip = $derived(String(item.id ?? item.ID ?? ''));
  const counts = $derived(layout.count_columns ?? []);

  function navigateToDetail() {
    if (detailUrl) {
      window.location.href = detailUrl;
      return;
    }
    if (props.onEdit && itemId) {
      props.onEdit(itemId);
    }
  }

  function formatCount(value: unknown, placeholder: string): { text: string; muted: boolean } {
    if (value === null || value === undefined || value === '') {
      return { text: placeholder || '—', muted: true };
    }
    const n = typeof value === 'number' ? value : Number(value);
    if (Number.isFinite(n) && n === 0 && placeholder) {
      return { text: placeholder, muted: true };
    }
    return { text: String(value), muted: false };
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="row-editorial {detailUrl || props.onEdit ? 'cursor-pointer hover:bg-gray-50' : ''} transition-colors duration-150"
  onclick={detailUrl || props.onEdit ? navigateToDetail : undefined}
>
  <div class="grid grid-cols-[80px_1fr_240px] gap-6 px-6 py-5 items-start">
    <!-- ID chip -->
    <div class="pt-1.5">
      {#if idChip}
        <span
          class="id-chip inline-block rounded border border-gray-200 bg-white px-2 py-0.5 font-mono text-xs text-gray-500 transition-colors"
        >{idChip}</span>
      {/if}
    </div>

    <!-- Body -->
    <div class="min-w-0">
      {#if byline}
        <p class="byline mb-1 text-[10px] uppercase font-mono text-gray-500" style="letter-spacing:0.14em;">{byline}</p>
      {/if}
      <h3 class="font-display text-xl leading-snug">
        {#if detailUrl}
          <a
            href={detailUrl}
            onclick={(e: MouseEvent) => e.stopPropagation()}
            class="title-link text-gray-900 transition-colors hover:underline"
          >{title}</a>
        {:else}
          <span class="title-link text-gray-900 transition-colors">{title}</span>
        {/if}
        {#if item.incomplete && item.can_edit}
          <span
            class="ml-2 inline-flex items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 align-middle text-[10px] uppercase tracking-wide text-amber-800 ring-1 ring-amber-200"
            title="This project needs setup before fields, models, or collections can be created"
          >
            <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
            </svg>
            Needs setup
          </span>
        {/if}
      </h3>
      {#if description}
        <p class="mt-1.5 max-w-[62ch] text-sm text-gray-500 line-clamp-2">{description}</p>
      {/if}
    </div>

    <!-- Counts / Actions. The counts grid spans the full 240px outer
         column with equal-width cells so values line up vertically
         across rows. Each cell text-right-aligns its value so the
         numbers themselves still hang on the right edge of their
         column. The previous `flex justify-end` wrapper let the inner
         grid size to its content, which made cell widths drift per
         row when count digits varied. -->
    <div class="w-full">
      {#if visibleActions.length > 0}
        <div class="flex items-center justify-end pt-1.5">
          <RowActionsKebab actions={visibleActions} {item} {lang} onAction={props.onAction} />
        </div>
      {:else if counts.length > 0}
        <div
          class="grid w-full gap-3 pt-1.5 font-mono text-sm tabular-nums text-gray-700"
          style="grid-template-columns: repeat({counts.length}, minmax(0, 1fr));"
        >
          {#each counts as col}
            {@const formatted = formatCount(item[col.key], col.zero_placeholder ?? '')}
            <span class="text-right {formatted.muted ? 'text-gray-300' : ''}">{formatted.text}</span>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .row-editorial:hover :global(.title-link) { color: var(--pletka-secondary, #6aaeaa); }
  .row-editorial:hover :global(.id-chip) {
    border-color: var(--pletka-primary, #92ccc8);
    color: var(--pletka-secondary, #6aaeaa);
  }
  .font-display {
    font-family: 'Newsreader', 'Source Serif Pro', Georgia, serif;
    font-optical-sizing: auto;
    letter-spacing: -0.01em;
  }
</style>
