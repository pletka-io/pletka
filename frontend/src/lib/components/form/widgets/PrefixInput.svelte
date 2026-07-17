<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

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

  type CheckResult = {
    available: boolean;
    valid: boolean;
    normalized?: string;
    message?: string;
    suggestions?: string[];
  };

  let status: 'idle' | 'checking' | 'available' | 'taken' | 'invalid' = $state('idle');
  let serverMessage = $state('');
  let suggestions: string[] = $state([]);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  let lastChecked = '';
  let userTouched = $state(false);

  function normalize(raw: string): string {
    return raw.trim().toUpperCase();
  }

  // derivePrefix turns a project name into a candidate prefix:
  // multi-word → initials (e.g. "Test Project Alpha" → "TPA"),
  // single word → first 3 letters (e.g. "LUX" → "LUX", "Census" → "CEN").
  // Returns "" if no valid 2-10-letter candidate is producible.
  function derivePrefix(name: string): string {
    const words = (name || '')
      .split(/[^A-Za-z]+/)
      .filter(Boolean);
    if (words.length === 0) return '';
    let candidate: string;
    if (words.length === 1) {
      candidate = words[0].slice(0, 3).toUpperCase();
    } else {
      candidate = words.map((w) => w[0].toUpperCase()).join('').slice(0, 10);
    }
    if (candidate.length < 2 || !/^[A-Z]+$/.test(candidate)) return '';
    return candidate;
  }

  // Auto-fill prefix from the source field while the user hasn't typed
  // in this input. Keeps the suggestion live as the name is edited.
  let derivedSource = $derived.by(() => {
    if (!field.derived_from) return '';
    const [src, srcLang] = field.derived_from.split('.');
    const raw = formValues?.[src];
    if (raw == null) return '';
    return typeof raw === 'object' ? (raw[srcLang || 'en'] || '') : String(raw);
  });

  $effect(() => {
    if (userTouched || field.readonly) return;
    const candidate = derivePrefix(derivedSource);
    if (candidate && candidate !== value) {
      value = candidate;
      scheduleCheck(candidate);
    }
  });

  async function check(prefix: string) {
    if (!field.check_url) return;
    if (prefix === lastChecked) return;
    lastChecked = prefix;
    status = 'checking';
    serverMessage = '';
    suggestions = [];
    try {
      const res = await fetch(`${field.check_url}?prefix=${encodeURIComponent(prefix)}`, {
        credentials: 'same-origin',
      });
      if (!res.ok) {
        status = 'idle';
        return;
      }
      const data: CheckResult = await res.json();
      if (!data.valid) {
        status = 'invalid';
        serverMessage = data.message ?? '';
        return;
      }
      if (data.available) {
        status = 'available';
        serverMessage = data.message ?? '';
      } else {
        status = 'taken';
        serverMessage = data.message ?? '';
        suggestions = data.suggestions ?? [];
      }
    } catch {
      status = 'idle';
    }
  }

  function scheduleCheck(prefix: string) {
    if (debounceTimer) clearTimeout(debounceTimer);
    if (!prefix || prefix.length < 2) {
      status = 'idle';
      serverMessage = '';
      suggestions = [];
      return;
    }
    debounceTimer = setTimeout(() => check(prefix), 350);
  }

  function handleInput(e: Event) {
    const raw = (e.target as HTMLInputElement).value;
    const upper = normalize(raw);
    userTouched = true;
    value = upper;
    scheduleCheck(upper);
  }

  function pickSuggestion(s: string) {
    userTouched = true;
    value = s;
    lastChecked = '';
    check(s);
  }
</script>

<div>
  <label class="block text-sm font-medium text-gray-700 mb-1" for={field.name}>
    {tr(field.label, lang)}
    {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
  </label>

  <div class="relative">
    <input
      id={field.name}
      type="text"
      value={value ?? ''}
      oninput={handleInput}
      readonly={field.readonly}
      autocomplete="off"
      spellcheck={false}
      class="shadow-sm focus:ring-pletka-primary focus:border-pletka-primary block w-full sm:text-sm rounded-md pr-10 font-mono uppercase
        {field.readonly ? 'bg-gray-50 text-gray-500' : ''}
        {errors.length || status === 'taken' || status === 'invalid' ? 'border-red-300' : 'border-gray-300'}
        {status === 'available' ? 'border-green-400' : ''}"
    />

    {#if status === 'checking'}
      <span class="absolute inset-y-0 right-2 flex items-center text-gray-400" aria-label="Checking">
        <svg class="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"></path>
        </svg>
      </span>
    {:else if status === 'available'}
      <span class="absolute inset-y-0 right-2 flex items-center text-green-600" aria-label="Available">
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
        </svg>
      </span>
    {:else if status === 'taken' || status === 'invalid'}
      <span class="absolute inset-y-0 right-2 flex items-center text-red-500" aria-label="Not available">
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </span>
    {/if}
  </div>

  {#if errors.length}
    <p class="mt-1 text-sm text-red-600">{errors[0]}</p>
  {:else if status === 'taken' || status === 'invalid'}
    <p class="mt-1 text-sm text-red-600">{serverMessage}</p>
  {:else if status === 'available'}
    <p class="mt-1 text-sm text-green-600">{serverMessage}</p>
  {:else if field.help}
    <p class="mt-1 text-sm text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  {#if suggestions.length}
    <div class="mt-2">
      <p class="text-xs text-gray-500 mb-1">Try one of these:</p>
      <div class="flex flex-wrap gap-2">
        {#each suggestions as s}
          <button
            type="button"
            onclick={() => pickSuggestion(s)}
            class="inline-flex items-center px-2 py-1 rounded-md border border-gray-300 bg-white text-xs font-mono uppercase hover:border-pletka-primary hover:text-pletka-primary"
          >
            {s}
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>
