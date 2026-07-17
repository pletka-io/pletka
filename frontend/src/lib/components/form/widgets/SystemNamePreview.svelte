<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr, slugify } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(null),
    formValues,
    lang,
  }: {
    field: FieldDef;
    value: string | null;
    formValues: Record<string, any>;
    lang: string;
  } = $props();

  // Derive the preview from the source field
  let derivedValue = $derived.by(() => {
    if (field.readonly && value) return value;
    if (!field.derived_from) return value || '';

    // Parse "ui_name.en" -> formValues.ui_name.en
    const [fieldName, fieldLang] = field.derived_from.split('.');
    const source = formValues[fieldName];
    if (!source) return '';
    const text = typeof source === 'object' ? source[fieldLang || 'en'] : source;
    return slugify(text || '');
  });

  // Keep value in sync with derived
  $effect(() => {
    if (!field.readonly) {
      value = derivedValue;
    }
  });
</script>

<div>
  <div class="block text-sm font-medium text-gray-700 mb-1">
    {tr(field.label, lang)}
  </div>

  <div class="flex items-center gap-2">
    <code class="block w-full px-3 py-2 bg-gray-50 border border-gray-300 rounded-md text-sm text-gray-600 font-mono">
      {derivedValue || '—'}
    </code>
    {#if field.immutable_after_create && !field.readonly}
      <span class="text-xs text-amber-600 whitespace-nowrap" title="Cannot be changed after creation">
        <svg class="w-4 h-4 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
            d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
        </svg>
      </span>
    {/if}
  </div>

  {#if tr(field.help, lang)}
    <p class="mt-1 text-xs text-gray-500">{tr(field.help, lang)}</p>
  {/if}
</div>
