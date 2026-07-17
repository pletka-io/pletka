<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import SimplePathBuilder from './path-builder/SimplePathBuilder.svelte';
  import RichPathBuilder from './path-builder/RichPathBuilder.svelte';

  /**
   * OntologyPathBuilder is a dispatcher between two visual variants:
   *
   *   - simple: minimal autocomplete + pill list. No tooltips, no
   *     inline domain/range previews, no inheritance badges. Fast for
   *     users who already know the ontology.
   *   - rich: full UX with hover tooltips on both suggestions and
   *     placed pills, type-aware pill colors, completion hint text,
   *     and "." / "done" manual-complete shortcut. Better for
   *     learning + discovery.
   *
   * Mode is per-user, persisted in localStorage. The toggle pill in
   * the header flips it. Both variants share the same input/output
   * shape so swapping the active widget never desynchronises form state.
   */

  let {
    field,
    value = $bindable<unknown>(null),
    formValues = {},
    lang = 'en',
    errors = [],
    scopeClass = '',
    includeInverse = false,
    includeParentProjects = true,
  }: {
    field: FieldDef;
    value: unknown;
    formValues: Record<string, any>;
    lang: string;
    errors: string[];
    scopeClass?: string;
    includeInverse?: boolean;
    includeParentProjects?: boolean;
  } = $props();

  type Mode = 'simple' | 'rich';
  const STORAGE_KEY = 'pletka.ontologyPathBuilder.mode';
  const DEFAULT_MODE: Mode = 'simple';

  function loadMode(): Mode {
    if (typeof window === 'undefined') return DEFAULT_MODE;
    try {
      const stored = window.localStorage.getItem(STORAGE_KEY);
      if (stored === 'simple' || stored === 'rich') return stored;
    } catch {
      // localStorage may throw in private mode / sandboxed iframes —
      // fall back silently to the default.
    }
    return DEFAULT_MODE;
  }

  function saveMode(m: Mode) {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage.setItem(STORAGE_KEY, m);
    } catch {
      // ignore — non-fatal
    }
  }

  let mode = $state<Mode>(loadMode());

  function toggleMode() {
    mode = mode === 'simple' ? 'rich' : 'simple';
    saveMode(mode);
  }
</script>

<div class="space-y-1">
  <!-- Mode toggle: floats above the widget; doesn't reflow the layout
       when flipped. Hidden in readonly mode (no point switching). -->
  {#if !field.readonly}
    <div class="flex items-center justify-end gap-1 text-xs">
      <span class="text-gray-400">View:</span>
      <button
        type="button"
        onclick={() => { mode = 'simple'; saveMode(mode); }}
        class="px-1.5 py-0.5 rounded {mode === 'simple' ? 'bg-pletka-primary text-white' : 'text-gray-500 hover:bg-gray-100'}"
        title="Simple: minimal suggestion list, no hover details"
      >Simple</button>
      <button
        type="button"
        onclick={() => { mode = 'rich'; saveMode(mode); }}
        class="px-1.5 py-0.5 rounded {mode === 'rich' ? 'bg-pletka-primary text-white' : 'text-gray-500 hover:bg-gray-100'}"
        title="Rich: hover tooltips, type colors, completion hints"
      >Rich</button>
    </div>
  {/if}

  {#if mode === 'rich'}
    <RichPathBuilder
      {field}
      bind:value
      {formValues}
      {lang}
      {errors}
      {scopeClass}
      {includeInverse}
      {includeParentProjects}
    />
  {:else}
    <SimplePathBuilder
      {field}
      bind:value
      {formValues}
      {lang}
      {errors}
      {scopeClass}
      {includeInverse}
      {includeParentProjects}
    />
  {/if}
</div>
