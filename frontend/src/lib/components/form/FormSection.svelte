<script lang="ts">
  import type { Section, LanguageInfo, FieldDef, VisibilityRule } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import WidgetDispatcher from './WidgetDispatcher.svelte';

  let {
    section,
    formValues = $bindable(),
    lang,
    languages,
    errors = {},
    dynamicOptions = {},
    hiddenFields = new Set<string>(),
  }: {
    section: Section;
    formValues: Record<string, any>;
    lang: string;
    languages: LanguageInfo[];
    errors: Record<string, string[]>;
    dynamicOptions?: Record<string, { value: string; label: string }[]>;
    hiddenFields?: Set<string>;
  } = $props();

  let collapsed = $state(false);
  let collapsedSectionID = $state<string | null>(null);

  $effect(() => {
    if (collapsedSectionID === section.id) return;
    collapsed = section.collapsed ?? false;
    collapsedSectionID = section.id;
  });

  /** Static visibility: field.visible_when rule (field-migration). */
  function isFieldVisible(field: FieldDef, values: Record<string, any>): boolean {
    if (!field.visible_when) return true;
    const currentValue = values[field.visible_when.field];
    return currentValue === field.visible_when.equals;
  }

  // Memoize the overlaid field by the dynamicOptions array reference. Called
  // in the render path, effectiveField would otherwise return a fresh field
  // object (and fresh options array) on every render. Widgets sync their local
  // options off field.options in a $effect; a new array each render means that
  // effect never stabilises and re-fires forever — Svelte's tightened loop
  // detection then throws effect_update_depth_exceeded. dynamicOptions[name]
  // keeps a stable reference until the field is re-fetched, so caching on it
  // yields a stable field object and the widget effect settles.
  const fieldOverlayCache = new Map<string, { dyn: unknown; built: FieldDef }>();

  /** Overlay dynamic options onto a field when options_url-sourced options are available (settings-fixes). */
  function effectiveField(f: FieldDef): FieldDef {
    const dyn = dynamicOptions[f.name];
    if (!dyn) return f;
    const cached = fieldOverlayCache.get(f.name);
    if (cached && cached.dyn === dyn) return cached.built;
    const built: FieldDef = {
      ...f,
      // Spread the raw entry first so shape-specific fields an endpoint adds
      // beyond {value,label} (e.g. the tree widget's prefix/has_children/
      // children) survive the overlay untouched.
      options: dyn.map((o: any) => ({
        ...o,
        value: o.value,
        // Endpoints may return either a scalar label or a Translations
        // object. Pass through objects untouched; wrap scalars so tr() can
        // resolve them. Without this, an already-wrapped label became
        // {en: {en: "..."}} and rendered as "[object Object]".
        label:
          o.label && typeof o.label === 'object'
            ? o.label
            : { en: String(o.label ?? '') },
      })),
    };
    fieldOverlayCache.set(f.name, { dyn, built });
    return built;
  }

  function shouldShow(field: FieldDef, values: Record<string, any>): boolean {
    if (hiddenFields.has(field.name)) return false;
    return isFieldVisible(field, values);
  }
</script>

{#if tr(section.label, lang)}
  <fieldset class="border border-gray-200 rounded-lg p-4 mb-4">
    <legend class="px-2 text-sm font-medium text-gray-700">
      <button type="button" class="flex items-center gap-1" onclick={() => collapsed = !collapsed}>
        <svg class="w-4 h-4 transition-transform {collapsed ? '' : 'rotate-90'}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
        </svg>
        {tr(section.label, lang)}
      </button>
    </legend>

    {#if !collapsed}
      <div class="space-y-4 mt-2">
        {#each section.fields as field (field.name)}
          {#if shouldShow(field, formValues)}
            {@const f = effectiveField(field)}
            <WidgetDispatcher
              field={f}
              bind:value={formValues[f.name]}
              {formValues}
              {lang}
              {languages}
              errors={errors[f.name] ?? []}
            />
          {/if}
        {/each}
      </div>
    {/if}
  </fieldset>
{:else}
  <div class="space-y-4 mb-4">
    {#each section.fields as field (field.name)}
      {#if shouldShow(field, formValues)}
        {@const f = effectiveField(field)}
        <WidgetDispatcher
          field={f}
          bind:value={formValues[f.name]}
          {formValues}
          {lang}
          {languages}
          errors={errors[f.name] ?? []}
        />
      {/if}
    {/each}
  </div>
{/if}
