<script lang="ts">
  /**
   * OverrideRefsCell wraps PillMultiSelect for the override-table editor.
   * Override editing IS the field form expressed for a model/collection
   * context — same widget, same options endpoints, same draft-create
   * flow.
   *
   * Sync model: parent owns `value` (lives in OverrideEditorState).
   * `inner` mirrors it so PillMultiSelect can `bind:value` mutate
   * locally. We emit upward via `onchange` only when the user actually
   * changed the array, never as a reaction to our own emission. The
   * `lastSync` sentinel records the most recent shape we either pulled
   * in from the prop or pushed out via onchange — both directions check
   * against it before doing anything, which breaks the value↔inner
   * feedback loop that otherwise trips Svelte's effect-depth guard.
   */
  import type { FieldDef } from '$lib/types/form-schema';
  import type { Translations } from '$lib/types/weave-types';
  import PillMultiSelect from '$lib/components/form/widgets/PillMultiSelect.svelte';

  type RefShape = { id: string; semantic_id?: string; name?: Translations };

  let {
    kind,
    projectId,
    value,
    lang,
    onchange,
  }: {
    kind: 'resource' | 'collection';
    projectId: string;
    value: string[];
    lang: string;
    /**
     * Called with both the new bare-id list AND a richer ref list
     * carrying labels we picked up from the options fetch. The parent
     * uses `refs` to keep the display-side `*_model_refs` array in sync
     * with the editable `*_models` bare array — without this plumbing,
     * a freshly-added pill renders its raw id in the row chip until
     * the next /overrides reload because filterRefsByIDs can only
     * stub-fill what it doesn't already know.
     */
    onchange: (next: string[], refs: RefShape[]) => void;
  } = $props();

  // Lazy options cache populated by the first /options fetch we trigger
  // via PillMultiSelect's options_url. Keyed by id → {name, semantic_id}.
  // Populated by a sibling fetch on mount so onchange always has labels
  // available, even before PillMultiSelect's own loadOptions completes.
  let labelByID = $state<Map<string, { name: Translations; semantic_id?: string }>>(new Map());

  $effect(() => {
    const url =
      kind === 'resource'
        ? `/projects/${projectId}/models/options`
        : `/projects/${projectId}/collections/options`;
    fetch(url)
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => {
        if (!Array.isArray(data)) return;
        const map = new Map<string, { name: Translations; semantic_id?: string }>();
        for (const opt of data as Array<{ value: string; label: Translations; semantic_id?: string }>) {
          map.set(opt.value, { name: opt.label, semantic_id: opt.semantic_id });
        }
        labelByID = map;
      })
      .catch(() => {});
  });

  function arrayEq(a: string[], b: string[]): boolean {
    return a.length === b.length && a.every((v, i) => v === b[i]);
  }

  // svelte-ignore state_referenced_locally
  let inner = $state<string[]>([...(value ?? [])]);
  // svelte-ignore state_referenced_locally
  let lastSync = [...(value ?? [])];

  // Pull external prop changes into inner — but only when they differ
  // from what we last sync'd. Without this guard the prop arrives back
  // through onchange-driven re-renders and re-triggers the same write
  // every flush cycle.
  $effect(() => {
    const next = value ?? [];
    if (!arrayEq(next, lastSync)) {
      lastSync = [...next];
      if (!arrayEq(next, inner)) {
        inner = [...next];
      }
    }
  });

  // Push user edits up via onchange — but only when inner diverges from
  // the last syncd shape. This stops the cell from echoing what we just
  // pulled from the prop.
  $effect(() => {
    if (!arrayEq(inner, lastSync)) {
      lastSync = [...inner];
      const refs: RefShape[] = inner.map((id) => {
        const meta = labelByID.get(id);
        return meta ? { id, semantic_id: meta.semantic_id, name: meta.name } : { id };
      });
      onchange([...inner], refs);
    }
  });

  const field: FieldDef = $derived({
    name: kind === 'resource' ? 'expected_resource_models' : 'expected_collection_models',
    widget: 'pill-multi-select',
    label: kind === 'resource'
      ? { en: 'Allowed models', nl: 'Toegestane modellen' }
      : { en: 'Allowed collections', nl: 'Toegestane collecties' },
    value: inner,
    entity_type: kind === 'resource' ? 'model' : 'collection',
    options_url: kind === 'resource'
      ? `/projects/${projectId}/models/options`
      : `/projects/${projectId}/collections/options`,
    create_url: '/api/v1/drafts',
    create_label: kind === 'resource'
      ? { en: 'Create Draft Model' }
      : { en: 'Create Draft Collection' },
  });
</script>

<div class="override-refs-cell">
  <PillMultiSelect {field} bind:value={inner} {lang} errors={[]} />
</div>

<style>
  /* Hide PillMultiSelect's own label + help — column header already says
     "Allowed targets". */
  .override-refs-cell :global(label) { display: none; }
  .override-refs-cell :global(p) { display: none; }
</style>
