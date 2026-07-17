<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
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

  function toggle(optionValue: string, checked: boolean) {
    const current = Array.isArray(value) ? value : [];
    if (checked) {
      if (!current.includes(optionValue)) value = [...current, optionValue];
    } else {
      value = current.filter((v) => v !== optionValue);
    }
  }

  function isChecked(optionValue: string): boolean {
    return Array.isArray(value) && value.includes(optionValue);
  }
</script>

<fieldset>
  <legend class="block text-sm font-medium text-gray-700 mb-1">
    {tr(field.label, lang)}
    {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
  </legend>

  <div class="space-y-1 border border-gray-300 rounded-md p-2 max-h-48 overflow-auto
    {errors.length ? 'border-red-300' : ''}">
    {#if (field.options ?? []).length === 0}
      <p class="text-sm text-gray-500 italic">No options available.</p>
    {/if}
    {#each field.options ?? [] as option}
      <label class="flex items-center gap-2 text-sm text-gray-700">
        <input
          type="checkbox"
          checked={isChecked(option.value)}
          onchange={(e) => toggle(option.value, (e.currentTarget as HTMLInputElement).checked)}
          class="rounded border-gray-300 text-pletka-primary focus:ring-pletka-primary"
        />
        <span>{tr(option.label, lang)}</span>
      </label>
    {/each}
  </div>

  {#if field.help && !errors.length}
    <p class="mt-1 text-sm text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}
</fieldset>
