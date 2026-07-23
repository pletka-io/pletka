<script lang="ts">
  import { tick } from 'svelte';
  import type { FieldDef, SelectOption } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable(null),
    lang,
    errors = [],
  }: {
    field: FieldDef;
    value: string | null;
    lang: string;
    errors: string[];
  } = $props();

  let options = $state<SelectOption[]>([]);
  let loadingOptions = $state(false);
  let abortController: AbortController | null = null;
  let loadedOnceFor = $state<string | null>(null);
  let open = $state(false);
  let query = $state('');
  let activeIndex = $state(0);
  let inputEl = $state<HTMLInputElement | null>(null);

  async function loadOptions(url: string) {
    abortController?.abort();
    abortController = new AbortController();
    const signal = abortController.signal;
    loadingOptions = true;
    loadedOnceFor = url;
    try {
      const res = await fetch(url, { signal });
      if (!res.ok) return;
      const data = await res.json();
      if (Array.isArray(data)) options = data;
    } catch (err) {
      if ((err as any)?.name === 'AbortError') return;
      console.error('SearchSelectWidget loadOptions error:', err);
    } finally {
      loadingOptions = false;
    }
  }

  $effect(() => {
    if (!field.options_url && !field.depends_on?.length) {
      options = field.options ?? [];
      return;
    }
    if (Array.isArray(field.options) && field.options.length > 0) {
      options = field.options;
      return;
    }
    if (field.depends_on?.length) return;
    if (field.options_url && loadedOnceFor !== field.options_url) {
      loadOptions(field.options_url);
    }
  });

  let selectedOption = $derived(options.find((o) => o.value === value) ?? null);
  let filteredOptions = $derived.by(() => {
    const q = query.trim().toLowerCase();
    const source = options.filter((o) => o.value !== '');
    if (!q) return source;
    return source.filter((o) => {
      const label = tr(o.label, lang).toLowerCase();
      const description = tr(o.description, lang).toLowerCase();
      const semanticID = (o.semantic_id || '').toLowerCase();
      return label.includes(q) || description.includes(q) || semanticID.includes(q);
    });
  });
  let menuOptions = $derived.by(() => {
    const items: Array<{ option: SelectOption | null; label: string; description?: string; semanticID?: string; status?: string; sourceProjectId?: string; sourceProjectLabel?: string }> = [];
    if (!field.required) {
      items.push({ option: null, label: '—' });
    }
    for (const option of filteredOptions) {
      items.push({
        option,
        label: tr(option.label, lang),
        description: tr(option.description, lang),
        semanticID: option.semantic_id,
        status: option.status,
        sourceProjectId: option.source_project_id,
        sourceProjectLabel: option.source_project_label,
      });
    }
    return items;
  });

  function statusClasses(status?: string): string {
    switch (status) {
      case 'valid':
        return 'bg-emerald-100 text-emerald-700';
      case 'has_issues':
        return 'bg-amber-100 text-amber-800';
      default:
        return 'bg-slate-100 text-slate-700';
    }
  }

  function selectOption(option: SelectOption | null) {
    value = option?.value ?? '';
    query = '';
    open = false;
  }

  function openMenu() {
    open = true;
    activeIndex = 0;
    tick().then(() => inputEl?.focus());
  }

  function closeMenu() {
    open = false;
    query = '';
  }

  function moveActive(delta: number) {
    if (!open) {
      openMenu();
      return;
    }
    if (menuOptions.length === 0) return;
    activeIndex = (activeIndex + delta + menuOptions.length) % menuOptions.length;
  }

  function selectActive() {
    const item = menuOptions[activeIndex];
    if (!item) return;
    selectOption(item.option);
  }

  function onTriggerKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      openMenu();
    }
  }

  function onInputKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      moveActive(1);
      return;
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      moveActive(-1);
      return;
    }
    if (event.key === 'Enter') {
      event.preventDefault();
      selectActive();
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      closeMenu();
    }
  }

  $effect(() => {
    if (!open) return;
    activeIndex = 0;
    tick().then(() => inputEl?.focus());
  });

  $effect(() => {
    if (activeIndex < menuOptions.length) return;
    activeIndex = Math.max(menuOptions.length - 1, 0);
  });
</script>

<div class="relative">
  <label class="block text-sm font-medium text-gray-700 mb-1" for={field.name}>
    {tr(field.label, lang)}
    {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
  </label>

  <button
    id={field.name}
    type="button"
    disabled={field.readonly || loadingOptions}
    onclick={() => (open ? closeMenu() : openMenu())}
    onkeydown={onTriggerKeydown}
    class="shadow-sm focus:ring-pletka-primary focus:border-pletka-primary flex w-full items-center justify-between rounded-md border border-gray-300 bg-white px-3 py-2 text-left sm:text-sm
      {field.readonly ? 'bg-gray-50 text-gray-500' : ''} {errors.length ? 'border-red-300' : ''}"
  >
    <span class={selectedOption ? 'text-gray-900' : 'text-gray-400'}>
      {#if loadingOptions}
        {field.required ? 'Loading options...' : 'Loading…'}
      {:else if selectedOption}
        {tr(selectedOption.label, lang)}{selectedOption.source_project_id ? ` (${selectedOption.source_project_label || selectedOption.source_project_id})` : ''}
      {:else}
        {field.required ? 'Select…' : '—'}
      {/if}
    </span>
    <svg class="h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
    </svg>
  </button>

  {#if open && !field.readonly}
    <div
      role="listbox"
      tabindex="-1"
      class="absolute z-30 mt-1 w-full rounded-md border border-gray-200 bg-white shadow-lg"
      onkeydown={onInputKeydown}
    >
      <div class="border-b border-gray-100 p-2">
        <input
          type="text"
          bind:this={inputEl}
          bind:value={query}
          placeholder="Filter options…"
          class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
        />
      </div>
      <div class="max-h-64 overflow-y-auto p-1">
        {#each menuOptions as item, index}
          <button
            type="button"
            onclick={() => selectOption(item.option)}
            onmouseenter={() => (activeIndex = index)}
            class="flex w-full items-start justify-between gap-3 rounded px-3 py-2 text-left text-sm hover:bg-gray-50 {activeIndex === index ? 'bg-gray-100' : ''} {item.option && value === item.option.value ? 'bg-pletka-primary/5 text-pletka-primary' : 'text-gray-800'}"
          >
            <div class="min-w-0">
              <div class="truncate">{item.label}</div>
              {#if item.description || item.semanticID || item.sourceProjectId}
                <div class="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                  {#if item.semanticID}
                    <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-600">{item.semanticID}</span>
                  {/if}
                  {#if item.sourceProjectId}
                    <span class="rounded bg-amber-50 px-1.5 py-0.5 text-[11px] text-amber-700">from {item.sourceProjectLabel || item.sourceProjectId}</span>
                  {/if}
                  {#if item.description}
                    <span class="truncate">{item.description}</span>
                  {/if}
                </div>
              {/if}
            </div>
            {#if item.option && item.status}
              <span class={`mt-0.5 rounded-full px-2 py-0.5 text-[11px] font-medium ${statusClasses(item.status)}`}>
                {item.status.replace('_', ' ')}
              </span>
            {/if}
          </button>
        {/each}
        {#if menuOptions.length === 0}
          <div class="px-3 py-2 text-sm text-gray-400">No matches</div>
        {/if}
      </div>
    </div>
  {/if}

  {#if field.help && !errors.length}
    <p class="mt-1 text-xs text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {/if}
</div>
