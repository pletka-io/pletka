<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(''),
    lang,
    errors = [],
  }: {
    field: FieldDef;
    value: string;
    lang: string;
    errors: string[];
  } = $props();
</script>

<fieldset>
  <legend class="block text-sm font-medium text-gray-700 mb-3">
    {tr(field.label, lang)}
    {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
  </legend>

  <div class="space-y-3">
    {#each field.options ?? [] as option}
      <label class="flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors
        {value === option.value ? 'border-pletka-primary bg-pletka-primary/5' : 'border-gray-200 hover:border-gray-300'}
        {field.readonly ? 'opacity-60 cursor-not-allowed' : ''}">
        <input
          type="radio"
          name={field.name}
          value={option.value}
          checked={value === option.value}
          disabled={field.readonly}
          onchange={() => { value = option.value; }}
          class="mt-0.5 h-4 w-4 text-pletka-primary focus:ring-pletka-primary border-gray-300"
        />
        <div>
          <span class="text-sm font-medium text-gray-900">{tr(option.label, lang)}</span>
          {#if option.description}
            <p class="text-sm text-gray-500 mt-0.5">{tr(option.description, lang)}</p>
          {/if}
        </div>
      </label>
    {/each}
  </div>

  {#if errors.length}
    <p class="mt-2 text-sm text-red-600">{errors[0]}</p>
  {/if}
</fieldset>
