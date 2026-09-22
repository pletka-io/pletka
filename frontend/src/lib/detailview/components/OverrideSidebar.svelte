<script lang="ts">
  import FormSection from '$lib/components/form/FormSection.svelte';
  import type { OverrideEditorField } from '$lib/detailview/override-editor-types';
  import type { OverrideEditorState } from '$lib/detailview/override-editor-state.svelte';
  import { UNCATEGORIZED_ID } from '$lib/detailview/override-editor-state.svelte';
  import type { FormSchema, SelectOption } from '$lib/types/form-schema';
  import { tr } from '$lib/types/weave-types';

  const schemaCache = new Map<string, FormSchema>();

  let {
    categoryId,
    itemId,
    field,
    editor,
    schemaUrl,
    inCollectionGroup,
    entityType,
    lang,
    onclose,
  }: {
    categoryId: string;
    itemId: string;
    field: OverrideEditorField;
    editor: OverrideEditorState;
    schemaUrl: string;
    inCollectionGroup: boolean;
    entityType: string;
    lang: string;
    onclose: () => void;
  } = $props();

  let schema = $state<FormSchema | null>(null);
  let loading = $state(true);
  let errorMessage = $state('');
  let formValues = $state<Record<string, any>>({});
  let lastCommitted = $state('');

  function parseIntOrZero(value: unknown): number {
    const n = Number.parseInt(String(value ?? ''), 10);
    return Number.isFinite(n) && n >= 0 ? n : 0;
  }

  function parseIntOrNull(value: unknown): number | null {
    const raw = String(value ?? '').trim();
    if (!raw) return null;
    const n = Number.parseInt(raw, 10);
    return Number.isFinite(n) && n >= 0 ? n : null;
  }

  function buildFormValues(nextField: OverrideEditorField): Record<string, any> {
    return {
      display_name: { ...(nextField.display_name ?? {}) },
      description: { ...(nextField.description ?? {}) },
      category_id:
        nextField.category_id && nextField.category_id !== UNCATEGORIZED_ID
          ? nextField.category_id
          : '',
      expected_resource_models: [...(nextField.expected_resource_models ?? [])],
      expected_collection_models: [...(nextField.expected_collection_models ?? [])],
      expected_concept_lists: [...(nextField.expected_concept_lists ?? [])],
      set_value: nextField.set_value ?? '',
      min_occurs: String(nextField.min_occurs ?? 0),
      max_occurs: nextField.max_occurs == null ? '' : String(nextField.max_occurs),
      is_required: !!nextField.is_required,
      is_hidden: !!nextField.is_hidden,
      visibility: nextField.visibility ?? '',
    };
  }

  function formSignature(values: Record<string, any>): string {
    return JSON.stringify(values);
  }

  function optionMap(fieldName: string): Map<string, SelectOption> {
    const def = schema?.sections.flatMap((section) => section.fields).find((candidate) => candidate.name === fieldName);
    return new Map((def?.options ?? []).map((option) => [option.value, option]));
  }

  function refsFromOptions(fieldName: string, ids: string[]) {
    const options = optionMap(fieldName);
    return ids.map((id) => {
      const option = options.get(id);
      return {
        id,
        semantic_id: option?.semantic_id || id,
        name: option?.label,
      };
    });
  }

  async function loadSchema(url: string) {
    loading = true;
    errorMessage = '';
    try {
      const cached = schemaCache.get(url);
      if (cached) {
        schema = cached;
        return;
      }
      const res = await fetch(url);
      if (!res.ok) throw new Error(`Failed to load sidebar schema: ${res.status}`);
      const data = (await res.json()) as FormSchema;
      schemaCache.set(url, data);
      schema = data;
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : String(err);
      schema = null;
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void loadSchema(schemaUrl);
  });

  $effect(() => {
    const next = buildFormValues(field);
    formValues = next;
    lastCommitted = formSignature(next);
  });

  $effect(() => {
    if (!schema) return;
    const signature = formSignature(formValues);
    if (signature === lastCommitted) return;

    const targetCategoryID = formValues.category_id ? String(formValues.category_id) : '';
    const nextCategoryID = targetCategoryID || UNCATEGORIZED_ID;
    if (!inCollectionGroup && nextCategoryID !== categoryId) {
      lastCommitted = signature;
      editor.moveFieldToCategory(categoryId, itemId, field.override_id, nextCategoryID);
      onclose();
      return;
    }

    const expectedResourceModels = Array.isArray(formValues.expected_resource_models)
      ? formValues.expected_resource_models.filter(Boolean)
      : [];
    const expectedCollectionModels = Array.isArray(formValues.expected_collection_models)
      ? formValues.expected_collection_models.filter(Boolean)
      : [];
    const expectedConceptLists = Array.isArray(formValues.expected_concept_lists)
      ? formValues.expected_concept_lists.filter(Boolean)
      : (field.expected_concept_lists ?? []);

    editor.updateField(categoryId, itemId, field.override_id, {
      display_name: formValues.display_name ?? {},
      description: formValues.description ?? {},
      category_id: nextCategoryID,
      expected_resource_models: expectedResourceModels,
      expected_resource_model_refs: refsFromOptions('expected_resource_models', expectedResourceModels),
      expected_collection_models: expectedCollectionModels,
      expected_collection_model_refs: refsFromOptions('expected_collection_models', expectedCollectionModels),
      expected_concept_lists: expectedConceptLists,
      expected_concept_list_refs: refsFromOptions('expected_concept_lists', expectedConceptLists).map((ref) => {
        const existing = field.expected_concept_list_refs?.find((candidate) => candidate.id === ref.id);
        return existing ?? ref;
      }),
      set_value: String(formValues.set_value ?? ''),
      min_occurs: parseIntOrZero(formValues.min_occurs),
      max_occurs: parseIntOrNull(formValues.max_occurs),
      is_required: !!formValues.is_required,
      is_hidden: !!formValues.is_hidden,
      visibility: String(formValues.visibility ?? ''),
    } as any);
    lastCommitted = signature;
  });
</script>

<aside class="override-sidebar fixed top-0 right-0 h-full w-[420px] bg-white border-l border-gray-200 shadow-xl z-50 flex flex-col">
  <header class="px-5 py-4 border-b border-gray-200 flex items-center gap-3">
    <div class="flex-1 min-w-0">
      <div class="text-[10px] uppercase tracking-widest text-pletka-primary font-medium">Override · {field.field_id}</div>
      <h3 class="text-base font-semibold text-gray-900 truncate">{tr(field.display_name, lang, field.field_id)}</h3>
    </div>
    <button
      type="button"
      class="text-gray-400 hover:text-gray-700 text-lg"
      title="Close (Esc)"
      onclick={onclose}
    >✕</button>
  </header>

  <div class="flex-1 overflow-y-auto px-5 py-4">
    {#if loading}
      <div class="animate-pulse space-y-4 pt-2">
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-24 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
      </div>
    {:else if errorMessage}
      <div class="rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{errorMessage}</div>
    {:else if schema}
      {#each schema.sections as section (section.id)}
        <FormSection
          {section}
          bind:formValues
          {lang}
          languages={schema.ui.languages}
          errors={{}}
          dynamicOptions={{}}
          hiddenFields={new Set()}
        />
      {/each}
    {/if}
  </div>

  <footer class="px-5 py-3 border-t border-gray-200 bg-gray-50 flex justify-end">
    <button
      type="button"
      class="px-4 py-2 rounded-md bg-pletka-primary text-white text-sm font-medium hover:bg-pletka-secondary focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
      onclick={onclose}
    >Done</button>
  </footer>
</aside>

<button
  type="button"
  class="sidebar-backdrop fixed inset-0 bg-black/20 z-40"
  aria-label="Close override sidebar"
  onclick={onclose}
></button>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') onclose(); }} />
