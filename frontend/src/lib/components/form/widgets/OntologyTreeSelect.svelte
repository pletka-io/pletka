<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import OntologyTreeSelect from './OntologyTreeSelect.svelte';

  // One node of the /options/extensions?base_version={version_id} forest
  // (pkg/weave/projectontologyversion.ExtensionTreeNode). `label` is a plain
  // string on nested `children` (never overlaid by FormSection's dynamic
  // options handling) but arrives wrapped as Translations on the root array
  // (FormSection wraps scalar labels so tr() can resolve them) — labelText()
  // below handles both shapes.
  interface TreeNode {
    value: string;
    label: string | Record<string, string>;
    prefix?: string;
    has_children?: boolean;
    children?: TreeNode[];
  }

  let {
    field,
    value = $bindable<string[]>([]),
    lang,
    errors = [],
    nodes,
    depth = 0,
  }: {
    field: FieldDef;
    value: string[];
    lang: string;
    errors?: string[];
    /** Internal: recursion passes the current level's nodes explicitly.
     *  Omitted at the top level, where nodes come from field.options. */
    nodes?: TreeNode[];
    /** Internal: recursion depth, 0 = root level (renders the field
     *  label/help/error chrome once). */
    depth?: number;
  } = $props();

  const options = $derived<TreeNode[]>(
    (depth === 0 ? (field.options as unknown as TreeNode[] | undefined) : nodes) ?? [],
  );

  let expanded = $state<Record<string, boolean>>({});

  function toggle(v: string) {
    expanded = { ...expanded, [v]: !expanded[v] };
  }

  function isChecked(v: string): boolean {
    return Array.isArray(value) && value.includes(v);
  }

  function flip(v: string) {
    const current = Array.isArray(value) ? value : [];
    value = current.includes(v) ? current.filter((x) => x !== v) : [...current, v];
  }

  function labelText(label: TreeNode['label']): string {
    return typeof label === 'string' ? label : tr(label, lang);
  }
</script>

<div class:space-y-1={depth === 0}>
  {#if depth === 0}
    <div class="block text-sm font-medium text-gray-700">
      {tr(field.label, lang)}
      {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
    </div>
    {#if field.help}
      <p class="text-xs text-gray-500">{tr(field.help, lang)}</p>
    {/if}
  {/if}

  <div class="space-y-0.5 {depth === 0 ? `border rounded-md p-2 max-h-64 overflow-auto ${errors.length ? 'border-red-300' : 'border-gray-300'}` : ''}">
    {#if depth === 0 && options.length === 0}
      <p class="text-sm text-gray-500 italic py-1">No extensions available.</p>
    {/if}
    {#each options as node (node.value)}
      <div class="ontology-tree-node">
        <div class="flex items-center gap-1">
          {#if node.has_children}
            <button
              type="button"
              class="w-4 shrink-0 text-gray-500 hover:text-gray-800"
              onclick={() => toggle(node.value)}
              aria-expanded={expanded[node.value] ?? false}
              aria-label={expanded[node.value] ? 'Collapse' : 'Expand'}
            >{expanded[node.value] ? '▾' : '▸'}</button>
          {:else}
            <span class="w-4 shrink-0"></span>
          {/if}
          <label class="flex items-center gap-2 text-sm text-gray-700">
            <input
              type="checkbox"
              checked={isChecked(node.value)}
              onchange={() => flip(node.value)}
              class="rounded border-gray-300 text-pletka-primary focus:ring-pletka-primary"
            />
            <span>{labelText(node.label)}</span>
            {#if node.prefix}
              <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-600">{node.prefix}</span>
            {/if}
          </label>
        </div>
        {#if node.has_children && expanded[node.value]}
          <div class="ml-5 border-l border-gray-200 pl-2">
            <OntologyTreeSelect {field} bind:value {lang} nodes={node.children ?? []} depth={depth + 1} />
          </div>
        {/if}
      </div>
    {/each}
  </div>

  {#if depth === 0 && errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}
</div>
