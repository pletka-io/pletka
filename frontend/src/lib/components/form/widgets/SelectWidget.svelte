<script lang="ts">
  import type { FieldDef, SelectOption } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(null),
    lang,
    errors = [],
  }: {
    field: FieldDef;
    value: string | null;
    lang: string;
    errors: string[];
  } = $props();

  let options = $state<SelectOption[]>([]);
  let loadingOptions = $state(false);
  let abortController: AbortController | null = null;
  // Tracks whether we've already fetched options_url for this field. The
  // server may legitimately return [] (e.g. a project with no categories
  // yet), so we cannot key the "do we still need to load?" guard on
  // options.length — that would re-fetch forever.
  let loadedOnceFor = $state<string | null>(null);
  let showCreateForm = $state(false);
  let createName = $state('');
  let createError = $state<string>('');

  async function loadOptions(url: string) {
    abortController?.abort();
    abortController = new AbortController();
    const signal = abortController.signal;
    loadingOptions = true;
    // Mark "attempted" up front so a legitimate empty response doesn't
    // retrigger the effect on the next render.
    loadedOnceFor = url;
    try {
      const res = await fetch(url, { signal });
      if (!res.ok) return;
      const data = await res.json();
      if (Array.isArray(data)) {
        options = data;
      }
    } catch (err) {
      if ((err as any)?.name === 'AbortError') return;
      console.error('SelectWidget loadOptions error:', err);
    } finally {
      // Always clear the spinner. Previously this was gated on
      // !signal.aborted, which left the spinner stuck after a chain of
      // re-renders aborted in-flight fetches.
      loadingOptions = false;
    }
  }

  // Skip our own fetch when:
  //  - the parent already supplied options via effectiveField (prop overlay)
  //  - or the field has dependencies (FormRenderer owns the substituted fetch).
  // Without this guard, a {token} options_url returns 0 results, options stays
  // empty, the effect re-fires on every prop overlay, and successive aborts
  // leave loadingOptions stuck at true (spinner forever).
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
      // Wait for FormRenderer's substituted fetch.
      return;
    }
    if (field.options_url && loadedOnceFor !== field.options_url) {
      loadOptions(field.options_url);
    }
  });

  function resolveProjectId(): string {
    // Mirror OntologyPathBuilder: every Svelte island that lives inside
    // a project carries data-prop-project-id on its mount root.
    if (typeof document !== 'undefined') {
      const containers = document.querySelectorAll('[data-island]');
      for (const el of containers) {
        const pid = (el as HTMLElement).dataset['propProjectId'];
        if (pid) return pid;
      }
    }
    return '';
  }

  async function createInline() {
    if (!field.create_url || !createName.trim()) return;
    if (!field.entity_type) {
      createError = 'Schema missing entity_type for inline create';
      return;
    }
    try {
      // Unified inline-create body. The /api/v1/drafts endpoint accepts
      // {type, project_id, name, ontology_scope?} for any of the
      // referenced entity kinds. Older endpoints expecting {ui_name:{...}}
      // also tolerate this shape (name === ui_name); we standardise here.
      const name = createName.trim();
      const projectId = resolveProjectId();
      const body: Record<string, any> = {
        type: field.entity_type,
        project_id: projectId,
        name: { en: name },
        // Keep ui_name for any legacy non-/api/v1/drafts CreateURLs that
        // happen to still be wired. Backends ignore unknown keys.
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
      const newValue = result.id || result.ref || result.semantic_id;
      if (!newValue) {
        createError = 'Create response missing id/ref/semantic_id';
        return;
      }
      const newLabel = createName.trim();

      field.options = [...(field.options ?? []), {
        value: newValue,
        label: { en: newLabel },
      }];
      options = [...options, {
        value: newValue,
        label: { en: newLabel },
      }];
      value = newValue;

      createName = '';
      showCreateForm = false;
      createError = '';
    } catch (err) {
      console.error('Inline create error:', err);
      createError = err instanceof Error ? err.message : 'Network error';
    }
  }
</script>

<div>
  <div class="flex items-center justify-between mb-1">
    <label class="block text-sm font-medium text-gray-700" for={field.name}>
      {tr(field.label, lang)}
      {#if field.required}<span class="text-red-500">*</span>{/if}
    </label>
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

  <select
    id={field.name}
    bind:value
    disabled={field.readonly || loadingOptions}
    class="block w-full rounded-md border-gray-300 shadow-sm focus:border-pletka-primary focus:ring-pletka-primary sm:text-sm
      {field.readonly ? 'bg-gray-50' : ''} {errors.length ? 'border-red-300' : ''}"
  >
    <!--
      Placeholder option:
        - Required + loading → "Loading options…"
        - Optional + loading → "Loading…"
        - Otherwise omit entirely if the schema already provides an
          empty-value option (e.g. "None (no parent)"); else emit a
          minimal "—" placeholder so the user can clear the selection.
      Help text lives BELOW the select; never inside the dropdown.
    -->
    {#if loadingOptions}
      <option value="">{field.required ? 'Loading options...' : 'Loading…'}</option>
    {:else if !field.required && !options.some(o => o.value === '')}
      <option value="">—</option>
    {/if}
    {#each options as option}
      <option value={option.value}>{tr(option.label, lang)}{option.source_project_id ? ` (${option.source_project_id})` : ''}</option>
    {/each}
  </select>

  {#if value && options.find(o => o.value === value)?.hint}
    <p class="mt-1 text-xs text-gray-600 font-mono">{options.find(o => o.value === value)?.hint}</p>
  {/if}

  {#if field.help}
    <p class="mt-1 text-xs text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}

  {#if showCreateForm && field.create_url}
    <div class="mt-2 p-3 bg-gray-50 rounded-md border border-gray-200">
      <div class="text-sm font-medium text-gray-700 mb-2">
        {field.create_label ? tr(field.create_label, lang) : 'Create new'}
      </div>
      <input
        type="text"
        bind:value={createName}
        placeholder="Name (English)"
        aria-label="Name for new item"
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
          class="px-3 py-1 text-sm bg-pletka-primary text-white rounded hover:bg-pletka-secondary"
        >Create</button>
      </div>
    </div>
  {/if}
</div>
