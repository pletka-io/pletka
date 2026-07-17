<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(false),
    lang,
    errors = [],
  }: {
    field: FieldDef;
    value: boolean;
    lang: string;
    errors: string[];
  } = $props();
</script>

<div>
  <label class="flex items-center gap-2 text-sm font-medium text-gray-700">
    <input
      type="checkbox"
      bind:checked={value}
      disabled={field.readonly}
      class="rounded border-gray-300 text-pletka-primary focus:ring-pletka-primary"
    />
    <span>
      {tr(field.label, lang)}
      {#if field.required}<span class="text-red-500">*</span>{/if}
    </span>
  </label>

  {#if field.help && !errors.length}
    <p class="mt-1 ml-6 text-sm text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  {#if errors.length}
    <p class="mt-1 ml-6 text-sm text-red-600">{errors[0]}</p>
  {/if}
</div>
