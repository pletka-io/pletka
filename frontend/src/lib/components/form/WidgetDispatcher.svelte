<script lang="ts">
  import type { FieldDef, LanguageInfo } from '$lib/types/form-schema';
  import { resolveWidget } from './widget-registry';

  let {
    field,
    value = $bindable(),
    formValues,
    lang,
    languages,
    errors = [],
  }: {
    field: FieldDef;
    value: any;
    formValues: Record<string, any>;
    lang: string;
    languages: LanguageInfo[];
    errors: string[];
  } = $props();

  const Widget = $derived(resolveWidget(field.widget));
</script>

{#if Widget}
  <Widget {field} bind:value {formValues} {lang} {languages} {errors} />
{:else}
  <p class="text-red-500 text-sm">Unknown widget: {field.widget}</p>
{/if}
