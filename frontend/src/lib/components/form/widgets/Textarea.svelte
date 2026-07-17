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

<div>
  <label class="block text-sm font-medium text-gray-700 mb-1" for={field.name}>
    {tr(field.label, lang)}
    {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
  </label>

  <textarea
    id={field.name}
    bind:value
    readonly={field.readonly}
    rows={4}
    class="shadow-sm focus:ring-pletka-primary focus:border-pletka-primary block w-full sm:text-sm border-gray-300 rounded-md
      {field.readonly ? 'bg-gray-50 text-gray-500' : ''} {errors.length ? 'border-red-300' : ''}"
  ></textarea>

  {#if field.help && !errors.length}
    <p class="mt-1 text-sm text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}
</div>
