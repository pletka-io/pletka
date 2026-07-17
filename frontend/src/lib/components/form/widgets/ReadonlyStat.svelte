<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(0),
    lang,
  }: {
    field: FieldDef;
    value: number;
    lang: string;
  } = $props();
</script>

<div class="p-4 bg-gray-50 rounded-lg border border-gray-200">
  <dt class="text-sm font-medium text-gray-500">{tr(field.label, lang)}</dt>
  <dd class="mt-1 text-2xl font-semibold text-gray-900">{value ?? 0}</dd>
  {#if field.help}
    <p class="mt-1 text-xs text-gray-400">{tr(field.help, lang)}</p>
  {/if}
</div>
