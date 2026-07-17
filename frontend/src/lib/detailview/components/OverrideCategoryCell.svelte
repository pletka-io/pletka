<script lang="ts">
  /**
   * OverrideCategoryCell wraps SelectWidget for the override sidebars —
   * fields, collections, and (future) models. Same widget the field
   * form uses for `category_id`, with the same /categories/options +
   * /api/v1/drafts wiring, so curators get the same draft-create UX
   * everywhere they pick a category.
   */
  import type { FieldDef } from '$lib/types/form-schema';
  import SelectWidget from '$lib/components/form/widgets/SelectWidget.svelte';

  let {
    projectId,
    value,
    options,
    lang,
    onchange,
    label = { en: 'Category', nl: 'Categorie' },
  }: {
    projectId: string;
    value: string | null | undefined;
    /**
     * Pre-resolved option list. When supplied, the cell skips the
     * /categories/options fetch and feeds these directly to
     * SelectWidget. Pass when the parent already has the project
     * roster (e.g. from response.available_categories).
     */
    options?: Array<{ value: string; label: Record<string, string>; semantic_id?: string }>;
    lang: string;
    onchange: (next: string) => void;
    label?: Record<string, string>;
  } = $props();

  // Mirror so SelectWidget can `bind:value`. Sentinel pattern keeps both
  // directions of the sync sane: pull from prop only when it differs
  // from what we last sync'd; push via onchange only when inner truly
  // diverges from the latest synced value.
  // svelte-ignore state_referenced_locally
  let inner = $state<string | null>(value ?? null);
  // svelte-ignore state_referenced_locally
  let lastSync: string | null = value ?? null;

  $effect(() => {
    const next = value ?? null;
    if (next !== lastSync) {
      lastSync = next;
      if (inner !== next) inner = next;
    }
  });

  $effect(() => {
    if (inner !== lastSync) {
      lastSync = inner;
      onchange(inner ?? '');
    }
  });

  const field: FieldDef = $derived({
    name: 'category_id',
    widget: 'select',
    label,
    value: inner,
    entity_type: 'category',
    // Either consume pre-resolved options (parent passed them) or
    // fetch lazily. Both flows reuse the same SelectWidget rendering.
    options: options as any,
    options_url: options ? undefined : `/projects/${projectId}/categories/options`,
    create_url: '/api/v1/drafts',
    create_label: { en: 'Create Category', nl: 'Categorie aanmaken' },
  });
</script>

<SelectWidget {field} bind:value={inner} {lang} errors={[]} />
