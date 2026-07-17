<script lang="ts">
  import type { FieldDef, LanguageInfo } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable({}),
    lang,
    languages,
    errors = [],
  }: {
    field: FieldDef;
    value: Record<string, string>;
    lang: string;
    languages: LanguageInfo[];
    errors: string[];
  } = $props();

  let showOtherLangs = $state(false);
  let otherLangs = $derived(languages.filter(l => l.code !== lang));
  let filledCount = $derived(
    otherLangs.filter(l => value?.[l.code]?.trim()).length
  );

  function updateValue(code: string, text: string) {
    value = { ...value, [code]: text };
  }

  function toggleExpand() {
    showOtherLangs = !showOtherLangs;
  }
</script>

<div>
  <label class="block text-sm font-medium text-gray-700 mb-2" for="{field.name}-{lang}">
    {tr(field.label, lang)}
    {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
  </label>

  <!-- Primary textarea with globe toggle -->
  <div class="flex gap-2">
    <div class="flex-1">
      <textarea
        id="{field.name}-{lang}"
        value={value?.[lang] ?? ''}
        oninput={(e) => updateValue(lang, (e.target as HTMLTextAreaElement).value)}
        readonly={field.readonly}
        rows={3}
        placeholder={tr(field.help, lang)}
        class="shadow-sm focus:ring-pletka-primary focus:border-pletka-primary block w-full sm:text-sm border-gray-300 rounded-md
          {field.readonly ? 'bg-gray-50' : ''} {errors.length ? 'border-red-300' : ''}"
      ></textarea>
    </div>

    {#if otherLangs.length > 0 && !field.readonly}
      <!-- Globe toggle button -->
      <button type="button"
        onclick={toggleExpand}
        class="relative flex-shrink-0 inline-flex items-center justify-center w-10 h-10 border rounded-md transition-colors self-start
          {showOtherLangs
            ? 'bg-pletka-primary text-white border-pletka-primary'
            : 'border-gray-300 text-gray-500 hover:text-gray-700 hover:bg-gray-50'}"
        title="Show all languages">
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
            d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {#if filledCount > 0}
          <span class="absolute -top-1 -right-1 inline-flex items-center justify-center w-4 h-4 text-xs font-bold text-white bg-pletka-primary rounded-full">
            {filledCount}
          </span>
        {/if}
      </button>
    {/if}
  </div>

  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}

  <!-- Expandable other languages -->
  {#if showOtherLangs && otherLangs.length > 0}
    <div class="mt-3 space-y-3 p-3 bg-gray-50 rounded-lg border border-gray-200">
      {#each otherLangs as l}
        <div class="flex items-start gap-3">
          <div class="flex-shrink-0 flex items-center gap-2 w-24 pt-2">
            {#if l.flag}<span class="text-lg">{l.flag}</span>{/if}
            <span class="text-sm text-gray-600">{l.name}</span>
          </div>
          <div class="flex-1">
            <textarea
              value={value?.[l.code] ?? ''}
              oninput={(e) => updateValue(l.code, (e.target as HTMLTextAreaElement).value)}
              rows={2}
              placeholder={l.name}
              class="shadow-sm focus:ring-pletka-primary focus:border-pletka-primary block w-full sm:text-sm border-gray-300 rounded-md"
            ></textarea>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
