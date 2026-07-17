<script lang="ts">
  import type {
    ProjectAdoptionsTabSchema,
    ProjectAdoptionTabItem,
    ProjectAdoptionClosureResponse,
    ProjectAdoptionClosureItem,
  } from '$lib/types/project-page';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import Badge from '$lib/components/shared/Badge.svelte';

  let { schema }: { schema: ProjectAdoptionsTabSchema } = $props();

  const lang = getUILang();
  const labels = $derived(schema.labels);

  type SectionKind = 'models' | 'collections' | 'fields';

  /** Per-receipt expansion state. Keyed by source_project_id|source_entity_id.
   *  Each section starts collapsed; opening triggers a one-shot fetch. */
  type SectionState = {
    open: boolean;
    loading: boolean;
    items: ProjectAdoptionClosureItem[] | null;
    error: string | null;
  };

  const sectionKey = (item: ProjectAdoptionTabItem, kind: SectionKind) =>
    `${item.source_project_id}|${item.source_entity_id}|${kind}`;

  let sections = $state<Record<string, SectionState>>({});

  function ensureSection(key: string): SectionState {
    if (!sections[key]) {
      sections[key] = { open: false, loading: false, items: null, error: null };
    }
    return sections[key];
  }

  async function toggleSection(item: ProjectAdoptionTabItem, kind: SectionKind) {
    const key = sectionKey(item, kind);
    const s = ensureSection(key);
    if (s.open) {
      sections[key] = { ...s, open: false };
      return;
    }
    if (s.items) {
      sections[key] = { ...s, open: true };
      return;
    }
    if (!item.closure_url_template) {
      sections[key] = { ...s, open: true, error: 'closure URL missing' };
      return;
    }
    sections[key] = { ...s, open: true, loading: true, error: null };
    try {
      const url = item.closure_url_template.replace('{kind}', kind);
      const res = await fetch(url);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as ProjectAdoptionClosureResponse;
      sections[key] = { ...s, open: true, loading: false, items: data.items ?? [], error: null };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      sections[key] = { ...s, open: true, loading: false, error: msg };
    }
  }

  function formatDate(value: string): string {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return new Intl.DateTimeFormat(lang, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
  }

  function itemLabel(item: ProjectAdoptionTabItem): string {
    return tr(item.label, lang, item.source_entity_id);
  }

  /** Prefer the source entity's UIName (server-resolved) and fall back
   *  to the bare entity-type label only when the name is missing. */
  function headerLabel(item: ProjectAdoptionTabItem): string {
    if (item.name) {
      const t = tr(item.name, lang, '');
      if (t) return t;
    }
    return itemLabel(item);
  }

  function closureLabel(c: ProjectAdoptionClosureItem): string {
    if (c.name) {
      const t = tr(c.name, lang, '');
      if (t) return t;
    }
    return c.semantic_id || c.id;
  }
</script>

<div class="space-y-4">
  <div class="rounded-lg border border-gray-200 bg-white p-5">
    <h2 class="text-lg font-semibold text-gray-900">{tr(labels?.title, lang, 'Adoptions')}</h2>
    <p class="mt-1 text-sm text-gray-600">{tr(labels?.description, lang, '')}</p>
  </div>

  {#if schema.items.length === 0}
    <div class="rounded-lg border border-dashed border-gray-300 bg-gray-50 p-8 text-sm text-gray-600">
      {tr(schema.empty_message, lang, '')}
    </div>
  {:else}
    {#each schema.items as item (`${item.source_project_id}:${item.source_entity_id}`)}
      <section class="rounded-lg border border-gray-200 bg-white p-5">
        <header class="flex flex-wrap items-start justify-between gap-3">
          <div class="flex-1 min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="text-base font-semibold text-gray-900">
                {#if item.current_url}
                  <a href={item.current_url} class="hover:text-blue-700">{headerLabel(item)}</a>
                {:else}
                  {headerLabel(item)}
                {/if}
              </h3>
              <Badge variant="purple" size="xs">{item.entity_type}</Badge>
              <span class="font-mono text-xs text-gray-500">{item.source_entity_id}</span>
              <span class="text-xs text-gray-500">{tr(labels?.from, lang, '')}</span>
              <span class="font-mono text-xs text-gray-700">{item.source_project_id}</span>
              {#if item.source_url}
                <a href={item.source_url} class="text-xs font-medium text-gray-500 hover:text-blue-700">{tr(labels?.source_link, lang, '')}</a>
              {/if}
            </div>
            <p class="mt-1 text-xs text-gray-500">
              {tr(labels?.adopted_prefix, lang, '')} {formatDate(item.adopted_at)}
              {#if item.created_by?.label || item.created_by?.id}
                · {tr(labels?.by_prefix, lang, '')} {item.created_by?.label || item.created_by?.id}
              {/if}
            </p>
          </div>
        </header>

        <div class="mt-4 flex flex-wrap gap-2">
          {#each [
            { kind: 'models', label: tr(labels?.models, lang, ''), direct: item.model_count_direct, total: item.model_count },
            { kind: 'collections', label: tr(labels?.collections, lang, ''), direct: item.collection_count_direct, total: item.collection_count },
            { kind: 'fields', label: tr(labels?.fields, lang, ''), direct: item.field_count_direct, total: item.field_count },
          ] as section (section.kind)}
            {@const key = sectionKey(item, section.kind as SectionKind)}
            {@const state = sections[key]}
            {@const open = state?.open ?? false}
            <button
              type="button"
              onclick={() => toggleSection(item, section.kind as SectionKind)}
              disabled={section.total === 0}
              title={`${section.direct} direct · ${section.total} transitive ${section.label}`}
              class="inline-flex items-center gap-1 rounded-full border px-3 py-1 text-xs font-medium
                {section.total === 0
                  ? 'border-gray-200 bg-gray-50 text-gray-400 cursor-not-allowed'
                  : open
                    ? 'border-blue-300 bg-blue-50 text-blue-800'
                    : 'border-gray-300 bg-gray-50 text-gray-700 hover:bg-gray-100'}"
            >
              <span class="text-xs">{open ? '▾' : '▸'}</span>
              <span class="font-mono">{section.direct}<span class="text-gray-400"> / </span>{section.total}</span>
              <span>{section.label}</span>
            </button>
          {/each}
        </div>

        {#each ['models', 'collections', 'fields'] as kind (kind)}
          {@const key = sectionKey(item, kind as SectionKind)}
          {@const state = sections[key]}
          {#if state?.open}
            <div class="mt-3 rounded-md border border-blue-100 bg-blue-50/40 p-3">
              {#if state.loading}
                <div class="text-xs text-gray-500">{tr(labels?.loading, lang, '')}</div>
              {:else if state.error}
                <div class="text-xs text-red-700">{tr(labels?.load_error_prefix, lang, '')} {state.error}</div>
              {:else if state.items && state.items.length > 0}
                <ul class="divide-y divide-blue-100">
                  {#each state.items as c (c.id)}
                    <li class="flex items-center gap-3 py-1.5">
                      <Badge variant="gray" size="xs">{c.source_project_id}</Badge>
                      <span class="font-mono text-xs text-gray-500">{c.semantic_id || c.id}</span>
                      <span class="flex-1 truncate text-sm text-gray-800">
                        {#if c.current_url}
                          <a href={c.current_url} class="hover:text-blue-700">{closureLabel(c)}</a>
                        {:else}
                          {closureLabel(c)}
                        {/if}
                      </span>
                      {#if c.source_url}
                        <a href={c.source_url} class="text-[10px] uppercase tracking-wide text-gray-400 hover:text-blue-700">{tr(labels?.source_column_label, lang, '')}</a>
                      {/if}
                    </li>
                  {/each}
                </ul>
              {:else}
                <div class="text-xs text-gray-500">{tr(labels?.section_empty, lang, '')}</div>
              {/if}
            </div>
          {/if}
        {/each}
      </section>
    {/each}
  {/if}
</div>
