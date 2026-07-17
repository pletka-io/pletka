<script lang="ts">
  import type { FieldDef, SelectOption } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable<string[]>([]),
    lang,
    errors = [],
  }: {
    field: FieldDef;
    value: string[];
    lang: string;
    errors: string[];
  } = $props();

  interface PillItem {
    ref: string;
    label: string;
    isDraft: boolean;
    id: string;
    sourceProjectId: string;
  }

  function parseValue(val: string[], opts: SelectOption[]): PillItem[] {
    if (!Array.isArray(val) || val.length === 0) return [];
    const refs = val.filter(Boolean);
    const optionsByValue = new Map(opts.map(o => [o.value, o]));
    return refs.map(ref => {
      const opt = optionsByValue.get(ref);
      return {
        ref,
        label: opt ? tr(opt.label, lang) : ref,
        isDraft: opt?.status === 'draft',
        id: ref,
        sourceProjectId: opt?.source_project_id ?? '',
      };
    });
  }

  let options = $state<SelectOption[]>([]);
  // `value` (bindable) is the single source of truth. Pills derive from it +
  // the loaded options. Deriving (instead of a pills↔value effect pair) makes
  // a reactive write-cycle impossible — the previous two-effect sync looped
  // with FormRenderer's options overlay (Svelte effect_update_depth_exceeded).
  const pills = $derived(parseValue(value, options));
  let loading = $state(false);
  let showCreateForm = $state(false);
  let createName = $state('');
  let createError = $state<string>('');
  let abortController: AbortController | null = null;
  // Mirrors SelectWidget: prevents re-fetch loop when a {token} URL legitimately
  // returns 0 results.
  let loadedOnceFor = $state<string | null>(null);

  function resolveProjectId(): string {
    if (typeof document !== 'undefined') {
      const containers = document.querySelectorAll('[data-island]');
      for (const el of containers) {
        const pid = (el as HTMLElement).dataset['propProjectId'];
        if (pid) return pid;
      }
    }
    return '';
  }

  // Available options = all options minus already-selected
  const availableOptions = $derived(
    options.filter(o => !pills.some(p => p.ref === o.value))
  );

  // Load options from URL if provided
  async function loadOptions(url: string) {
    abortController?.abort();
    abortController = new AbortController();
    const signal = abortController.signal;
    loading = true;
    loadedOnceFor = url;
    try {
      const res = await fetch(url, { signal });
      if (!res.ok) return;
      const data = await res.json();
      if (Array.isArray(data)) {
        options = data;
      } else if (data.items && Array.isArray(data.items)) {
        options = data.items.map((item: any) => ({
          value: item.ref || item.semantic_id || item.id,
          label: item.ui_name || { en: item.name || item.id },
          status: item.status,
        }));
      }
    } catch (err) {
      if ((err as any)?.name === 'AbortError') return;
      console.error('loadOptions error:', err);
    } finally {
      loading = false;
    }
  }

  // Sync options reactively. Matches SelectWidget so dependent fields whose
  // options are fetched by FormRenderer (via {token} substitution) actually
  // show up here too. Without this, we'd fetch the literal placeholder URL
  // and the picker stayed empty.
  // Sync the options list only (pills derive from value + options). Mirrors
  // SelectWidget: pick up FormRenderer's token-substituted overlay, skip own
  // fetch for dependent fields, guard re-fetch with loadedOnceFor.
  $effect(() => {
    if (!field.options_url && !field.depends_on?.length) {
      options = field.options ?? [];
      return;
    }
    if (Array.isArray(field.options) && field.options.length > 0) {
      options = field.options;
      return;
    }
    if (field.depends_on?.length) {
      // FormRenderer owns the substituted fetch for dependent fields.
      return;
    }
    if (field.options_url && loadedOnceFor !== field.options_url) {
      loadOptions(field.options_url);
    }
  });

  function addFromDropdown(e: Event) {
    const select = e.target as HTMLSelectElement;
    const ref = select.value;
    if (!ref) return;

    const opt = options.find(o => o.value === ref);
    if (!opt) return;
    const current = value ?? [];
    if (current.includes(ref)) return;

    value = [...current, ref];
    select.value = '';
  }

  function removePill(ref: string) {
    value = (value ?? []).filter(r => r !== ref);
  }

  async function createInline() {
    if (!field.create_url || !createName.trim()) return;
    if (!field.entity_type) {
      createError = 'Schema missing entity_type for inline create';
      return;
    }
    try {
      const name = createName.trim();
      const projectId = resolveProjectId();
      const body: Record<string, any> = {
        type: field.entity_type,
        project_id: projectId,
        name: { en: name },
        ui_name: { en: name },
      };
      const res = await fetch(field.create_url, {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/json',
          'X-Requested-With': 'XMLHttpRequest',
        },
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        let msg = `Create failed (${res.status})`;
        try {
          const err = await res.json();
          if (err?.message) msg = err.message;
          else if (err?.error) msg = err.error;
        } catch {}
        createError = msg;
        return;
      }

      const result = await res.json();
      const newRef = result.id || result.ref || result.semantic_id;
      if (!newRef) {
        createError = 'Create response missing id/ref/semantic_id';
        return;
      }
      const newLabel = name;

      options = [...options, {
        value: newRef,
        label: { en: newLabel },
        status: 'draft',
      }];

      if (!(value ?? []).includes(newRef)) {
        value = [...(value ?? []), newRef];
      }

      createName = '';
      showCreateForm = false;
      createError = '';
    } catch (err) {
      console.error('Inline create error:', err);
      createError = err instanceof Error ? err.message : 'Network error';
    }
  }
</script>

<div class="space-y-1">
  <div class="flex items-center justify-between">
    <div class="block text-sm font-medium text-gray-700">
      {tr(field.label, lang)}
      {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
    </div>
    {#if field.create_url && !field.readonly}
      <button
        type="button"
        onclick={() => { showCreateForm = !showCreateForm; createError = ''; }}
        class="text-sm text-pletka-primary hover:text-pletka-secondary font-medium"
      >
        + {field.create_label ? tr(field.create_label, lang) : 'Create new'}
      </button>
    {/if}
  </div>

  {#if field.help}
    <p class="text-xs text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  <div class="flex flex-wrap gap-1.5 min-h-[2.5rem] p-1.5 border rounded-md bg-white
    {errors.length ? 'border-red-300' : 'border-gray-300'}
    {field.readonly ? 'bg-gray-50' : ''}
    focus-within:ring-1 focus-within:ring-pletka-primary focus-within:border-pletka-primary">
    {#if pills.length === 0}
      <span class="text-sm text-gray-400 py-1 px-1">No items selected</span>
    {/if}
    {#each pills as pill (pill.ref)}
      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-sm
        {pill.isDraft ? 'bg-amber-100 text-amber-800' : 'bg-blue-100 text-blue-800'}">
        <span>{pill.label}{pill.isDraft ? ' [Draft]' : ''}{pill.sourceProjectId ? ` (${pill.sourceProjectId})` : ''}</span>
        {#if !field.readonly}
          <button
            type="button"
            onclick={() => removePill(pill.ref)}
            class="ml-0.5 text-current opacity-60 hover:opacity-100 font-bold leading-none"
            aria-label="Remove {pill.label}"
          >&times;</button>
        {/if}
      </span>
    {/each}
  </div>

  {#if !field.readonly}
    <select
      onchange={addFromDropdown}
      class="mt-1 block w-full rounded-md border-gray-300 shadow-sm text-sm
        focus:ring-pletka-primary focus:border-pletka-primary"
      disabled={loading}
    >
      <option value="">{loading ? 'Loading...' : '+ Add...'}</option>
      {#each availableOptions as opt (opt.value)}
        <option value={opt.value}>
          {tr(opt.label, lang)}{opt.status === 'draft' ? ' [Draft]' : ''}{opt.source_project_id ? ` (${opt.source_project_id})` : ''}
        </option>
      {/each}
    </select>
  {/if}

  {#if showCreateForm && field.create_url}
    <div class="mt-2 p-3 bg-amber-50 rounded-md border border-amber-200">
      <div class="text-sm font-medium text-amber-800 mb-2">
        {field.create_label ? tr(field.create_label, lang) : 'Create new'}
      </div>
      <input
        type="text"
        bind:value={createName}
        placeholder="Name (English)"
        class="block w-full rounded-md border-gray-300 shadow-sm text-sm
          focus:ring-pletka-primary focus:border-pletka-primary"
        onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); createInline(); }}}
      />
      {#if createError}
        <p class="mt-2 text-sm text-red-600">{createError}</p>
      {/if}
      <div class="mt-2 flex justify-end gap-2">
        <button
          type="button"
          onclick={() => { showCreateForm = false; createName = ''; createError = ''; }}
          class="px-3 py-1 text-sm text-gray-600 hover:text-gray-800"
        >Cancel</button>
        <button
          type="button"
          onclick={createInline}
          class="px-3 py-1 text-sm bg-amber-500 text-white rounded hover:bg-amber-600"
        >Create</button>
      </div>
    </div>
  {/if}

  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}

  <input type="hidden" name={field.name} value={value} />
</div>
