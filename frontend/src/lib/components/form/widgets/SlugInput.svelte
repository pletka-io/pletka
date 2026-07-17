<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { routeSlugify, tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(''),
    formValues,
    lang,
    errors = [],
  }: {
    field: FieldDef;
    value: string;
    formValues: Record<string, any>;
    lang: string;
    errors: string[];
  } = $props();

  let userTouched = $state(Boolean(value));

  let derivedSource = $derived.by(() => {
    if (!field.derived_from) return '';
    const [src, srcLang] = field.derived_from.split('.');
    const raw = formValues?.[src];
    if (raw == null) return '';
    return typeof raw === 'object' ? (raw[srcLang || 'en'] || '') : String(raw);
  });

  let suggestedValue = $derived(routeSlugify(derivedSource));

  $effect(() => {
    if (field.readonly || userTouched) return;
    if (suggestedValue !== value) {
      value = suggestedValue;
    }
  });

  function handleInput(e: Event) {
    userTouched = true;
    value = routeSlugify((e.target as HTMLInputElement).value);
  }

  function resetToSuggested() {
    userTouched = false;
    value = suggestedValue;
  }
</script>

<div>
  <div class="flex items-center justify-between mb-1">
    <label class="block text-sm font-medium text-gray-700" for={field.name}>
      {tr(field.label, lang)}
      {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
    </label>
    {#if suggestedValue && userTouched && suggestedValue !== value}
      <button
        type="button"
        onclick={resetToSuggested}
        class="text-xs font-medium text-pletka-primary hover:text-pletka-secondary"
      >
        Use suggestion
      </button>
    {/if}
  </div>

  <input
    id={field.name}
    type="text"
    value={value ?? ''}
    oninput={handleInput}
    readonly={field.readonly}
    spellcheck={false}
    autocomplete="off"
    class="shadow-sm focus:ring-pletka-primary focus:border-pletka-primary block w-full sm:text-sm border-gray-300 rounded-md font-mono
      {field.readonly ? 'bg-gray-50 text-gray-500' : ''} {errors.length ? 'border-red-300' : ''}"
  />

  {#if !errors.length && suggestedValue}
    <p class="mt-1 text-xs text-gray-500">
      Suggested slug: <code class="font-mono">{suggestedValue}</code>
    </p>
  {/if}

  {#if field.help && !errors.length}
    <p class="mt-1 text-sm text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}
</div>
