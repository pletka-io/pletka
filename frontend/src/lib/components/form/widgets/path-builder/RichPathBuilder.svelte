<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import type { PathElement, ScopeInfo, ParsedPath } from '$lib/types/path-types';
  import type { PathSuggestion, AutocompleteRequest, AutocompleteResponse } from '$lib/types/ontology-types';
  import SuggestionTooltip from './SuggestionTooltip.svelte';

  let {
    field,
    value = $bindable<unknown>(null),
    formValues = {},
    lang = 'en',
    errors = [],
    scopeClass = '',
    includeInverse = false,
    includeParentProjects = true,
  }: {
    field: FieldDef;
    /**
     * Bound form value. Canonical shape is PathElement[] — the widget
     * emits structured data, not strings. Strings are still tolerated on
     * read for backward compatibility with rows that still hold the
     * legacy JSON-stringified ParsedPath in pathElements TEXT/JSONB.
     */
    value: unknown;
    formValues: Record<string, any>;
    lang: string;
    errors: string[];
    scopeClass?: string;
    includeInverse?: boolean;
    includeParentProjects?: boolean;
  } = $props();

  const effectiveIncludeInverse = $derived.by(() => {
    const fromForm = formValues?.include_inverse;
    if (typeof fromForm === 'boolean') return fromForm;
    return includeInverse;
  });

  const effectiveIncludeParentProjects = $derived.by(() => {
    const fromForm = formValues?.include_parent_projects;
    if (typeof fromForm === 'boolean') return fromForm;
    return includeParentProjects;
  });

  // Contextual scope: when this widget renders inside a field-override
  // form (path != scope mode), formValues.ontology_scope holds the
  // current scope class. Prepend its qname to the autocomplete
  // current_path so suggestions stay relevant to the override target.
  const contextualScope = $derived.by(() => {
    if (isScopeMode) return null;
    const raw = formValues?.ontology_scope;
    if (!raw || typeof raw !== 'object') return null;
    const obj = raw as Record<string, unknown>;
    const prefix = typeof obj.prefix === 'string' ? obj.prefix : '';
    const localName = typeof obj.local_name === 'string' ? obj.local_name : '';
    if (!localName) return null;
    return { prefix, local_name: localName } as Pick<PathElement, 'prefix' | 'local_name'>;
  });

  const contextualScopeQname = $derived(
    contextualScope ? prefixedName(contextualScope as PathElement) : ''
  );

  // Multi-class entities (e.g. crmdig:D1 + crm:E36) need property
  // suggestions unioned across both classes' lineages. Surface the
  // additional types from formValues.ontology_scope so the autocomplete
  // backend sees them.
  const contextualScopeAdditional = $derived.by(() => {
    if (isScopeMode) return [] as string[];
    const raw = formValues?.ontology_scope;
    if (!raw || typeof raw !== 'object') return [] as string[];
    const additional = (raw as Record<string, unknown>).additional_types;
    if (!Array.isArray(additional)) return [] as string[];
    const out: string[] = [];
    for (const t of additional) {
      if (!t || typeof t !== 'object') continue;
      const tr = t as Record<string, unknown>;
      const prefix = typeof tr.prefix === 'string' ? tr.prefix : '';
      const localName = typeof tr.local_name === 'string' ? tr.local_name : '';
      if (!localName) continue;
      out.push(prefix ? `${prefix}:${localName}` : localName);
    }
    return out;
  });

  // --- Configuration ---

  // Scope mode: single class selection (used when field.name is
  // "ontology_scope"). The widget emits a single PathElement object;
  // path mode emits a PathElement[] array. Both are structured — no
  // string serialisation back to formValues.
  const isScopeMode = $derived(
    field.name.includes('scope') ||
    (field.validation as any)?.scope_mode === true
  );

  // Resolve projectId from formValues or nearest data-island ancestor
  function getProjectId(): string {
    if (formValues?.project_id) return String(formValues.project_id);
    if (typeof document !== 'undefined') {
      const containers = document.querySelectorAll('[data-island]');
      for (const el of containers) {
        const pid = (el as HTMLElement).dataset['propProjectId'];
        if (pid) return pid;
      }
    }
    return '';
  }

  // Resolve optional versionId from formValues
  function getVersionId(): string {
    if (formValues?.version_id) return String(formValues.version_id);
    return '';
  }

  // --- Helper functions ---

  /** Compute a prefixed display name like "crm:E21_Person". */
  function prefixedName(el: PathElement): string {
    if (!el.prefix) return el.local_name;
    return `${el.prefix}:${el.local_name}`;
  }

  /** Normalize suggestion type to PathElement type vocabulary. */
  function normalizeType(suggestionType: string): string {
    switch (suggestionType) {
      case 'class':
      case 'property-class':
        return 'class';
      case 'object_property':
      case 'datatype_property':
        return 'property';
      case 'literal':
        return 'literal';
      default:
        return suggestionType;
    }
  }

  /** Extract a short class code from a local name (e.g., "E21_Person" -> "E21"). */
  function extractClassCode(localName: string): string | undefined {
    const match = localName.match(/^([A-Z]\d+)/);
    return match ? match[1] : undefined;
  }

  /** Convert a PathSuggestion to a PathElement at the given position. */
  function suggestionToElement(s: PathSuggestion, position: number): PathElement {
    const el: PathElement = {
      type: normalizeType(s.type),
      uri: s.uri || prefixedName({ prefix: s.prefix, local_name: s.local_name } as PathElement),
      prefix: s.prefix,
      local_name: s.local_name,
      position,
      class_code: extractClassCode(s.local_name),
    };
    // Engine-synthesised literal suggestions carry a datatype (xsd:string,
    // rdfs:Literal, etc.). Copy it onto the path element so the persisted
    // shape matches the existing convention in weave_fields.path_elements.
    if (s.datatype) {
      el.datatype = s.datatype;
    }
    return el;
  }

  /** Detect terminal type from the last suggestion's range_classes. */
  function detectTerminalType(s: PathSuggestion): string | undefined {
    if (!s.range_classes || s.range_classes.length === 0) return undefined;
    // If range contains literal types, terminal is literal
    const literalIndicators = ['rdfs:Literal', 'xsd:string', 'xsd:date', 'xsd:dateTime', 'xsd:integer', 'xsd:boolean', 'rdf:literal'];
    for (const rc of s.range_classes) {
      for (const lit of literalIndicators) {
        if (rc.includes(lit) || rc.toLowerCase().includes('literal')) {
          return 'literal';
        }
      }
    }
    return 'class';
  }

  /** Check if a suggestion type represents a class. */
  function isClassType(type: string): boolean {
    return type === 'class' || type === 'property-class';
  }

  /** Parse the autocomplete response envelope, throwing on shape mismatch. */
  function parseAutocompleteResponse(payload: unknown): AutocompleteResponse {
    if (
      typeof payload !== 'object' ||
      payload === null ||
      !('suggestions' in payload) ||
      !Array.isArray((payload as Record<string, unknown>).suggestions)
    ) {
      throw new Error('Invalid autocomplete response: expected { suggestions: PathSuggestion[] }');
    }
    return payload as AutocompleteResponse;
  }

  // --- Legacy arrow-string parsing ---

  /** Extract local name from a full URI. */
  function localNameFromUri(uri: string): string {
    const hash = uri.lastIndexOf('#');
    if (hash >= 0) return uri.slice(hash + 1);
    const slash = uri.lastIndexOf('/');
    if (slash >= 0) return uri.slice(slash + 1);
    // Handle prefixed names like "crm:E21_Person"
    const colon = uri.indexOf(':');
    if (colon >= 0) return uri.slice(colon + 1);
    return uri;
  }

  /** Extract prefix from a prefixed name like "crm:E21_Person". */
  function prefixFromPrefixed(name: string): string {
    const colon = name.indexOf(':');
    if (colon >= 0) return name.slice(0, colon);
    return '';
  }

  /** Heuristic: CRM classes start with E followed by digits; properties with P. */
  function guessTypeFromName(name: string): string {
    if (/^E\d/.test(name)) return 'class';
    if (/^P\d/.test(name)) return 'property';
    return 'class';
  }

  /** Parse legacy arrow-separated path string into PathElement[]. */
  function parseLegacyPath(val: string): PathElement[] {
    if (!val) return [];
    return val
      .split('->')
      .map(s => s.trim())
      .filter(s => s.length > 0)
      .map((segment, i) => {
        const ln = localNameFromUri(segment);
        const prefix = prefixFromPrefixed(segment) || '';
        const uri = prefix ? `${prefix}:${ln}` : ln;
        return {
          type: guessTypeFromName(ln),
          uri,
          prefix,
          local_name: ln,
          position: i,
          class_code: extractClassCode(ln),
        };
      });
  }

  /** Try to parse value into PathElement[]. Accepts:
   * - PathElement[] directly (canonical / new shape)
   * - { elements: PathElement[] } legacy ParsedPath wrapper (backfill)
   * - JSON string of either (legacy field rows)
   * - Arrow-separated string (very old import format)
   */
  function parseValue(val: unknown): PathElement[] {
    if (val == null || val === '') return [];
    // Already an array (the canonical shape).
    if (Array.isArray(val)) return val as PathElement[];
    // Legacy wrapper object.
    if (typeof val === 'object') {
      const obj = val as any;
      if (Array.isArray(obj.elements)) return obj.elements;
      // ScopeInfo single-element shape (back-compat).
      if (obj.uri !== undefined && obj.local_name) {
        return [{
          type: 'class',
          uri: obj.uri,
          prefix: obj.prefix || '',
          local_name: obj.local_name,
          position: 0,
          class_code: obj.class_code || extractClassCode(obj.local_name),
        }];
      }
      return [];
    }
    if (typeof val === 'string') {
      const s = val as string;
      if (s.startsWith('{') || s.startsWith('[')) {
        try {
          const parsed = JSON.parse(s);
          return parseValue(parsed);
        } catch {
          // Fall through to legacy arrow parsing.
        }
      }
      return parseLegacyPath(s);
    }
    return [];
  }

  // --- Output: the form value is the PathElement[] array itself.
  // No JSON-stringification, no ParsedPath/ScopeInfo wrappers. The
  // server treats ontology_scope as derivable from path[0] and validates
  // accordingly.

  // --- Component state ---

  let pathElements = $state<PathElement[]>(parseValue(value));
  let query = $state('');
  let suggestions = $state<PathSuggestion[]>([]);
  let selectedIndex = $state(-1);
  let hoveredIndex = $state<number | null>(null);
  let showDropdown = $state(false);
  let showSettings = $state(false);
  let loading = $state(false);
  let inputEl = $state<HTMLInputElement | null>(null);
  let lastTerminalType = $state<string | undefined>(undefined);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  // Per-call autocomplete setting overrides. null → fall through to the
  // form/component defaults (effectiveIncludeInverse / Parent). The
  // RangeSuggestions switch is a pure override with no upstream default.
  let includeInverseOverride = $state<boolean | null>(null);
  let includeParentProjectsOverride = $state<boolean | null>(null);
  let includeRangeSuggestionsOverride = $state<boolean>(false);

  // --- Derived state ---

  const lastElement = $derived(
    pathElements.length > 0 ? pathElements[pathElements.length - 1] : null
  );

  const effectiveIncludeInverseSetting = $derived(
    includeInverseOverride ?? effectiveIncludeInverse
  );

  const effectiveIncludeParentProjectsSetting = $derived(
    includeParentProjectsOverride ?? effectiveIncludeParentProjects
  );

  const isComplete = $derived(
    pathElements.length > 0 && lastElement !== null &&
    (lastElement.complete === true || lastElement.type === 'literal')
  );

  const lastElementIsProperty = $derived(
    lastElement !== null && lastElement.type === 'property'
  );

  const completionHint = $derived.by(() => {
    if (pathElements.length === 0) return isScopeMode ? 'Search for a class' : 'Start by searching for a class';
    if (isScopeMode) return '';
    if (lastElement?.complete) return 'Path marked as complete';
    if (lastElement?.type === 'literal') return 'Path complete (literal terminal)';
    if (lastElement?.type === 'class' && pathElements.length > 1) {
      return 'Path complete -- or add a property to extend. Type . or done to mark complete.';
    }
    if (lastElement?.type === 'class') return 'Next: add a property';
    if (lastElement?.type === 'property') {
      if (lastTerminalType === 'literal') return 'Next: add a literal or class value. Type . or done to mark complete.';
      return 'Next: add a class. Type . or done to mark complete.';
    }
    return '';
  });

  // Scope mode: readonly after selection
  const scopeSelected = $derived(isScopeMode && pathElements.length > 0);

  // --- Sync effects ---

  // Sync outward when pathElements changes. Both modes emit structured
  // data — never JSON strings.
  //   scope mode:  value = pathElements[0] || null   (single PathElement)
  //   path  mode:  value = pathElements             (PathElement[])
  // FormRenderer JSON-encodes formValues on submit, so the server sees
  // structured shapes directly.
  $effect(() => {
    if (isScopeMode) {
      const next = pathElements.length > 0 ? pathElements[0] : null;
      if (!sameElement(next, value as PathElement | null)) {
        value = next;
      }
      return;
    }
    if (!arraysShallowEqual(pathElements, asArray(value))) {
      value = pathElements;
    }
  });

  // If value changes from outside (e.g. form reset), re-parse into
  // the local elements state.
  $effect(() => {
    const incoming = parseValue(value);
    if (!arraysShallowEqual(incoming, pathElements)) {
      pathElements = incoming;
    }
  });

  function asArray(v: unknown): PathElement[] {
    return Array.isArray(v) ? (v as PathElement[]) : [];
  }

  function arraysShallowEqual(a: PathElement[], b: PathElement[]): boolean {
    if (a === b) return true;
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
    // Compare by identity-bearing fields, NOT by reference. parseValue
    // returns a fresh object array on every call (it deep-clones the
    // structure), so a reference check would always say "differ" and
    // the two sync effects would ping-pong forever
    // (svelte.dev/e/effect_update_depth_exceeded).
    for (let i = 0; i < a.length; i++) {
      if (!sameElement(a[i], b[i])) return false;
    }
    return true;
  }

  function sameElement(a: PathElement | null, b: PathElement | null): boolean {
    if (a === b) return true;
    if (!a || !b) return false;
    return a.prefix === b.prefix && a.local_name === b.local_name && a.type === b.type
      && !!a.complete === !!b.complete;
  }

  // --- Autocomplete ---

  function scheduleSearch(q: string) {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => fetchSuggestions(q), 200);
  }

  async function fetchSuggestions(q: string) {
    const projectId = getProjectId();
    const versionId = getVersionId();
    if (!projectId && !versionId) return;

    loading = true;
    try {
      const body: Partial<AutocompleteRequest> = {
        project_id: projectId || undefined,
        version_id: versionId || undefined,
        current_path: [
          ...(isScopeMode || !contextualScopeQname ? [] : [contextualScopeQname]),
          ...pathElements.map(e => prefixedName(e)),
        ],
        query: q,
        max_results: 20,
        scope_class: isScopeMode ? (scopeClass || '') : contextualScopeQname || scopeClass || '',
        scope_additional_classes: contextualScopeAdditional.length ? contextualScopeAdditional : undefined,
        include_inverse: effectiveIncludeInverseSetting,
        include_parent_projects: effectiveIncludeParentProjectsSetting,
        include_range_suggestions: includeRangeSuggestionsOverride,
      };
      const res = await fetch('/api/v1/ontology/autocomplete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) return;
      const payload = parseAutocompleteResponse(await res.json());
      const data: PathSuggestion[] = payload.suggestions;
      suggestions = isScopeMode
        ? data.filter(s => isClassType(s.type))
        : data;
      selectedIndex = -1;
      showDropdown = suggestions.length > 0;
    } catch (err) {
      suggestions = [];
      showDropdown = false;
      console.error('Ontology autocomplete failed', err);
    } finally {
      loading = false;
    }
  }

  // --- Event handlers ---

  function handleInput(e: Event) {
    query = (e.target as HTMLInputElement).value;
    // Always schedule a search — engine returns sensible defaults for an
    // empty query (root classes when path is empty, next-step suggestions
    // when mid-path). Curators expect to see what's possible the moment
    // they touch the input.
    scheduleSearch(query);
  }

  /** Check if the current input should trigger manual completion. */
  function isCompleteCommand(q: string): boolean {
    const trimmed = q.trim().toLowerCase();
    return trimmed === '.' || trimmed === 'done';
  }

  /** Mark the path as manually complete by flagging the final element. */
  function markComplete() {
    if (pathElements.length === 0 || isScopeMode) return;
    const last = pathElements[pathElements.length - 1];
    // Only a real terminal (class or literal) can be marked complete.
    if (last.type !== 'class' && last.type !== 'literal') return;
    if (last.complete) return; // already marked
    pathElements = [
      ...pathElements.slice(0, -1),
      { ...last, complete: true },
    ];
    query = '';
    suggestions = [];
    showDropdown = false;
  }

  /** Clear the completion flag on the final element so the path can be extended. */
  function unmarkComplete() {
    if (pathElements.length === 0) return;
    const last = pathElements[pathElements.length - 1];
    if (!last.complete) return;
    const { complete, ...rest } = last; // drop the flag entirely (omitempty parity)
    pathElements = [...pathElements.slice(0, -1), rest];
    // isComplete flips to false -> the input box re-appears (Step 5 guard),
    // so focus returns to autocomplete and the modeler continues.
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIndex = Math.min(selectedIndex + 1, suggestions.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIndex = Math.max(selectedIndex - 1, -1);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      // Check for manual complete command first
      if (isCompleteCommand(query)) {
        markComplete();
      } else if (selectedIndex >= 0 && suggestions[selectedIndex]) {
        addElement(suggestions[selectedIndex]);
      }
    } else if (e.key === 'Escape') {
      showDropdown = false;
      selectedIndex = -1;
    } else if (e.key === 'Backspace' && query === '') {
      undoLast();
    }
  }

  function addElement(suggestion: PathSuggestion) {
    const el = suggestionToElement(suggestion, pathElements.length);
    // Track terminal type from range_classes for completion hints
    lastTerminalType = detectTerminalType(suggestion);

    if (isScopeMode) {
      // Replace entire path with single selected class
      pathElements = [el];
      suggestions = [];
      showDropdown = false;
      query = '';
    } else {
      pathElements = [...pathElements, el];
      query = '';
      showDropdown = false;
      suggestions = [];
      // Fetch next step suggestions
      scheduleSearch('');
    }

    setTimeout(() => inputEl?.focus(), 0);
  }

  function undoLast() {
    if (pathElements.length > 0) {
      pathElements = pathElements.slice(0, -1);
      lastTerminalType = undefined;
      scheduleSearch('');
    }
  }

  function clearAll() {
    pathElements = [];
    suggestions = [];
    showDropdown = false;
    query = '';
    lastTerminalType = undefined;
    setTimeout(() => inputEl?.focus(), 0);
  }

  function handleFocus() {
    // Always preload defaults on focus so the dropdown shows what's
    // available without requiring the curator to type first.
    // Engine returns root classes when the path is empty
    // and next-step suggestions when mid-path.
    scheduleSearch(query);
  }

  function handleBlur() {
    // Delay hide so click on dropdown item fires first
    setTimeout(() => {
      showDropdown = false;
      hoveredIndex = null;
    }, 150);
  }

  // --- Pill styling ---

  function pillClass(type: string): string {
    switch (type) {
      case 'class':
        return 'bg-blue-100 text-blue-800 border-blue-200';
      case 'property':
        return 'bg-green-100 text-green-800 border-green-200';
      case 'literal':
        return 'bg-purple-100 text-purple-800 border-purple-200';
      default:
        return 'bg-gray-100 text-gray-800 border-gray-200';
    }
  }

  function typeBadgeClass(type: string): string {
    if (isClassType(type)) return 'bg-blue-100 text-blue-800';
    if (type === 'datatype_property') return 'bg-purple-100 text-purple-800';
    return 'bg-green-100 text-green-800';
  }

  function suggestionLabel(s: PathSuggestion): string {
    // Show label in current lang with fallback, or local_name
    if (s.label) {
      const l = s.label[lang] || s.label['en'];
      if (l) return l;
    }
    return s.local_name;
  }
</script>

<div class="space-y-1">
  <div class="block text-sm font-medium text-gray-700">
    {tr(field.label, lang)}
    {#if field.required}<span class="text-red-500 ml-1">*</span>{/if}
  </div>

  {#if field.help}
    <p class="text-xs text-gray-500">{tr(field.help, lang)}</p>
  {/if}

  <!-- Path input area + settings popover. The flex container lets the
       gear button sit beside the (flex-1) path input without disturbing
       its width when the popover toggles. -->
  <div class="flex gap-2">
    <div class="relative flex-1">
      <div
      class="flex-1 min-h-[2.5rem] flex flex-wrap items-center gap-1 px-2 py-1.5 rounded-md border
        {errors.length ? 'border-red-300' : 'border-gray-300'}
        {field.readonly ? 'bg-gray-50' : 'bg-white'}
        focus-within:ring-2 focus-within:ring-pletka-primary focus-within:border-pletka-primary"
      >
      <!-- Path element pills -->
      {#each pathElements as el, i}
        {#if i > 0}
          <span class="text-gray-400 text-xs select-none" aria-hidden="true">&rarr;</span>
        {/if}
        <span
          class="inline-flex items-center gap-0.5 px-2 py-0.5 rounded-full text-xs font-medium border {pillClass(el.type)}"
          title={`${el.prefix}:${el.local_name}`}
        >
          {prefixedName(el)}
          {#if !field.readonly && !isScopeMode}
            <button
              type="button"
              onclick={() => {
                pathElements = pathElements.slice(0, i);
                lastTerminalType = undefined;
                scheduleSearch('');
                setTimeout(() => inputEl?.focus(), 0);
              }}
              class="ml-0.5 text-current opacity-60 hover:opacity-100 leading-none"
              aria-label="Remove {prefixedName(el)} and everything after"
            >&times;</button>
          {/if}
          {#if el.complete}
            <span class="inline-flex items-center gap-0.5 ml-1 pl-1 border-l border-emerald-300" title="Path marked complete">
              <svg class="w-3 h-3 text-emerald-600" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/></svg>
              <button type="button" class="text-emerald-700 opacity-60 hover:opacity-100 leading-none" aria-label="Remove completion marker and continue" onclick={() => unmarkComplete()}>&times;</button>
            </span>
          {/if}
        </span>
      {/each}

      <!-- Text input (hidden when scope selected or path marked complete) -->
      {#if !field.readonly && !scopeSelected && !isComplete}
        <input
          bind:this={inputEl}
          type="text"
          value={query}
          oninput={handleInput}
          onkeydown={handleKeydown}
          onfocus={handleFocus}
          onblur={handleBlur}
          placeholder={pathElements.length === 0
            ? (isScopeMode ? 'Search for a class...' : 'Search for class or property...')
            : ''}
          class="flex-1 min-w-[8rem] bg-transparent outline-none text-sm text-gray-900 placeholder-gray-400"
          autocomplete="off"
          spellcheck={false}
        />
      {/if}

      <!-- Undo + Clear buttons -->
      {#if pathElements.length > 0 && !field.readonly}
        <div class="ml-auto flex-shrink-0 flex items-center gap-1">
          {#if !isScopeMode && pathElements.length > 1}
            <button
              type="button"
              onclick={undoLast}
              class="text-gray-400 hover:text-gray-600 text-xs"
              title="Undo last step"
            >Undo</button>
          {/if}
          <button
            type="button"
            onclick={clearAll}
            class="text-gray-400 hover:text-gray-600 text-xs"
            title="Clear all"
          >Clear</button>
        </div>
      {/if}

        <!-- Loading spinner -->
        {#if loading}
          <span class="flex-shrink-0 w-4 h-4 border-2 border-gray-300 border-t-pletka-primary rounded-full animate-spin" aria-hidden="true"></span>
        {/if}
      </div>

      <!-- Suggestions dropdown -->
      {#if showDropdown && suggestions.length > 0}
        <div class="absolute left-0 right-0 z-20 mt-1">
          <ul
            class="bg-white border border-gray-200 rounded-md shadow-lg max-h-64 overflow-y-auto"
            role="listbox"
          >
            {#each suggestions as s, i}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <!-- svelte-ignore a11y_interactive_supports_focus -->
              <li
                role="option"
                aria-selected={i === selectedIndex}
                class="relative flex items-start gap-2 px-3 py-2 cursor-pointer text-sm
                  {i === selectedIndex ? 'bg-pletka-primary text-white' : 'hover:bg-gray-50'}"
                onmousedown={(e) => { e.preventDefault(); addElement(s); }}
                onmouseenter={() => { hoveredIndex = i; }}
                onmouseleave={() => { hoveredIndex = null; }}
              >
                <span class="flex-1 min-w-0">
                  <span class="block font-medium truncate">
                    {s.prefix ? `${s.prefix}:` : ''}{s.local_name}
                  </span>
                  {#if s.label?.[lang] || s.label?.en}
                    <span class="block text-xs opacity-70 truncate">
                      {s.label[lang] || s.label.en}
                    </span>
                  {/if}
                </span>
                <span class="flex-shrink-0 inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium
                  {i === selectedIndex ? 'bg-white/20 text-white' : typeBadgeClass(s.type)}">
                  {s.type === 'object_property' ? 'property' : s.type === 'datatype_property' ? 'datatype' : s.type}
                </span>

                <!-- Tooltip on hover -->
                {#if hoveredIndex === i}
                  <div class="absolute left-full top-0 ml-2 w-72 bg-gray-800 text-white rounded-lg shadow-xl p-3 z-30">
                    <SuggestionTooltip suggestion={s} selected={i === selectedIndex} />
                  </div>
                {/if}
              </li>
            {/each}
          </ul>
        </div>
      {/if}
    </div>

    {#if !field.readonly}
      <!-- Autocomplete settings popover. Per-call overrides for inverse,
           parent-projects, range-suggestions, manual-complete. Local to
           the widget instance — not persisted. -->
      <div class="relative flex-shrink-0">
        <button
          type="button"
          onclick={() => showSettings = !showSettings}
          class="relative flex-shrink-0 inline-flex items-center justify-center w-10 h-10 border rounded-md transition-colors
            {showSettings
              ? 'bg-pletka-primary text-white border-pletka-primary'
              : 'border-gray-300 text-gray-500 hover:text-gray-700 hover:bg-gray-50'}"
          title="Autocomplete settings"
          aria-label="Autocomplete settings"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11.983 5.5a1.5 1.5 0 013.034 0l.13.94a1.5 1.5 0 001.127 1.25l.914.222a1.5 1.5 0 01.58 2.621l-.696.644a1.5 1.5 0 00-.4 1.516l.274.91a1.5 1.5 0 01-2.173 1.77l-.84-.48a1.5 1.5 0 00-1.483 0l-.84.48a1.5 1.5 0 01-2.173-1.77l.274-.91a1.5 1.5 0 00-.4-1.516l-.696-.644a1.5 1.5 0 01.58-2.621l.914-.221a1.5 1.5 0 001.127-1.251z" />
            <circle cx="13.5" cy="12" r="2.25" stroke-width="2" />
          </svg>
        </button>

        {#if showSettings}
          <div class="absolute right-0 top-full mt-2 w-72 rounded-md border border-gray-200 bg-white shadow-lg z-30 p-3 space-y-2">
            <div class="text-xs font-semibold text-gray-700">Autocomplete settings</div>
            <label class="flex items-center gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={effectiveIncludeParentProjectsSetting}
                onchange={(e) => includeParentProjectsOverride = (e.currentTarget as HTMLInputElement).checked}
              />
              <span>Include parent projects</span>
            </label>
            <label class="flex items-center gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={effectiveIncludeInverseSetting}
                onchange={(e) => includeInverseOverride = (e.currentTarget as HTMLInputElement).checked}
              />
              <span>Include inverse properties</span>
            </label>
            <label class="flex items-center gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={includeRangeSuggestionsOverride}
                onchange={(e) => includeRangeSuggestionsOverride = (e.currentTarget as HTMLInputElement).checked}
              />
              <span>Include range suggestions</span>
            </label>
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Completion hints -->
  {#if completionHint && !isScopeMode}
    {#if isComplete}
      <p class="text-xs text-green-600 flex items-center gap-1">
        <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
        </svg>
        {completionHint}
      </p>
    {:else if pathElements.length > 0}
      <p class="text-xs text-amber-600 flex items-center gap-1">
        <span class="font-bold">...</span> {completionHint}
      </p>
    {/if}
  {/if}

  <!-- Validation errors -->
  {#if errors.length}
    <p class="text-sm text-red-600">{errors[0]}</p>
  {/if}

  <!-- Hidden input for form submission -->
  <!-- No hidden input: FormRenderer reads bound `value` (PathElement[]) and
       JSON-encodes the whole formValues map on submit. A hidden form
       input would force a string round-trip (the old bug). -->
</div>
