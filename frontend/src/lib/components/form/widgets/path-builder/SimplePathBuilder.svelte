<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import type { PathElement, TypeRef } from '$lib/types/path-types';
  import type { PathSuggestion, AutocompleteRequest, AutocompleteResponse } from '$lib/types/ontology-types';

  /**
   * SimplePathBuilder: minimal autocomplete + pill-list path picker.
   *
   * Trades visual richness (no tooltips, no inline domain/range previews,
   * no inheritance badges) for clarity and speed. Same input/output
   * shape and same backend endpoint as RichPathBuilder — the dispatcher
   * (OntologyPathBuilder.svelte) flips between them at runtime so a user
   * who knows their ontology can move fast, while a learner uses the
   * richer view.
   */

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
    value: unknown;
    formValues: Record<string, any>;
    lang: string;
    errors: string[];
    scopeClass?: string;
    includeInverse?: boolean;
    includeParentProjects?: boolean;
  } = $props();

  const isScopeMode = $derived(
    field.name.includes('scope') || (field.validation as any)?.scope_mode === true
  );

  // Contextual scope: when used inside a field-override form,
  // formValues.ontology_scope holds the override target's scope class.
  // Prepend its qname to current_path so suggestions stay relevant.
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

  // When a contextual scope has secondary class types (multi-class
  // entity), pass them through so property suggestions union the class
  // lineages on the backend.
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

  function getVersionId(): string {
    if (formValues?.version_id) return String(formValues.version_id);
    return '';
  }

  function prefixedName(el: PathElement): string {
    if (!el.prefix) return el.local_name;
    return `${el.prefix}:${el.local_name}`;
  }

  function normalizeType(t: string): string {
    if (t === 'class' || t === 'property-class') return 'class';
    if (t === 'object_property' || t === 'datatype_property') return 'property';
    if (t === 'literal') return 'literal';
    return t;
  }

  function isClassType(t: string): boolean {
    return t === 'class' || t === 'property-class';
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

  function extractClassCode(localName: string): string | undefined {
    const m = localName.match(/^([A-Z]\d+)/);
    return m ? m[1] : undefined;
  }

  function suggestionToElement(s: PathSuggestion, position: number): PathElement {
    return {
      type: normalizeType(s.type),
      uri: s.uri || prefixedName({ prefix: s.prefix, local_name: s.local_name } as PathElement),
      prefix: s.prefix,
      local_name: s.local_name,
      position,
      class_code: extractClassCode(s.local_name),
    };
  }

  function parseValue(val: unknown): PathElement[] {
    if (val == null || val === '') return [];
    if (Array.isArray(val)) return val as PathElement[];
    if (typeof val === 'object') {
      const obj = val as any;
      if (Array.isArray(obj.elements)) return obj.elements;
      if (obj.uri !== undefined && obj.local_name) {
        return [{
          type: 'class',
          uri: obj.uri,
          prefix: obj.prefix || '',
          local_name: obj.local_name,
          position: 0,
          class_code: obj.class_code || extractClassCode(obj.local_name),
          additional_types: Array.isArray(obj.additional_types) ? obj.additional_types : undefined,
        }];
      }
    }
    return [];
  }

  function sameTypeRefs(a: TypeRef[] | undefined, b: TypeRef[] | undefined): boolean {
    const al = a ?? [];
    const bl = b ?? [];
    if (al.length !== bl.length) return false;
    for (let i = 0; i < al.length; i++) {
      if (al[i].prefix !== bl[i].prefix || al[i].local_name !== bl[i].local_name) return false;
    }
    return true;
  }

  function arraysShallowEqual(a: PathElement[], b: PathElement[]): boolean {
    if (a === b) return true;
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) {
      if (a[i].prefix !== b[i].prefix || a[i].local_name !== b[i].local_name || a[i].type !== b[i].type
        || !!a[i].complete !== !!b[i].complete) {
        return false;
      }
    }
    return true;
  }

  function sameElement(a: PathElement | null, b: PathElement | null): boolean {
    if (a === b) return true;
    if (!a || !b) return false;
    if (a.prefix !== b.prefix || a.local_name !== b.local_name || a.type !== b.type) return false;
    return sameTypeRefs(a.additional_types, b.additional_types);
  }

  let pathElements = $state<PathElement[]>(parseValue(value));
  let query = $state('');
  let suggestions = $state<PathSuggestion[]>([]);
  let selectedIndex = $state(-1);
  let showDropdown = $state(false);
  let loading = $state(false);
  let inputEl = $state<HTMLInputElement | null>(null);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Multi-class scope: secondary class picker. Active only when
  // isScopeMode and a primary class is already chosen.
  let addingAdditional = $state(false);
  let addQuery = $state('');
  let addSuggestions = $state<PathSuggestion[]>([]);
  let addSelectedIndex = $state(-1);
  let addShowDropdown = $state(false);
  let addLoading = $state(false);
  let addInputEl = $state<HTMLInputElement | null>(null);
  let addDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  const additionalTypes = $derived<TypeRef[]>(
    isScopeMode && pathElements[0]?.additional_types
      ? (pathElements[0].additional_types as TypeRef[])
      : [],
  );

  const scopeSelected = $derived(isScopeMode && pathElements.length > 0);

  // A path chain is complete when its last element carries the editor-only
  // `complete` flag (set by typing "done" / ".") or is a literal terminus.
  // Mirrors RichPathBuilder so both modes finish a path the same way.
  const isComplete = $derived(
    !isScopeMode && pathElements.length > 0 &&
    (pathElements[pathElements.length - 1].complete === true ||
     pathElements[pathElements.length - 1].type === 'literal')
  );

  $effect(() => {
    if (isScopeMode) {
      const next = pathElements.length > 0 ? pathElements[0] : null;
      if (!sameElement(next, value as PathElement | null)) {
        value = next;
      }
      return;
    }
    if (!arraysShallowEqual(pathElements, Array.isArray(value) ? (value as PathElement[]) : [])) {
      value = pathElements;
    }
  });

  $effect(() => {
    const incoming = parseValue(value);
    if (!arraysShallowEqual(incoming, pathElements)) {
      pathElements = incoming;
    }
  });

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
        include_inverse: includeInverse,
        include_parent_projects: includeParentProjects,
        include_range_suggestions: false,
      };
      const res = await fetch('/api/v1/ontology/autocomplete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) return;
      const payload = parseAutocompleteResponse(await res.json());
      const data: PathSuggestion[] = payload.suggestions;
      suggestions = isScopeMode ? data.filter(s => isClassType(s.type)) : data;
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

  function handleInput(e: Event) {
    query = (e.target as HTMLInputElement).value;
    if (query.length === 0 && pathElements.length === 0) {
      suggestions = [];
      showDropdown = false;
      return;
    }
    scheduleSearch(query);
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
      // Typing "done" or "." finishes the path; otherwise take the highlighted suggestion.
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

  /** Whether the current input is the manual-completion command. */
  function isCompleteCommand(q: string): boolean {
    const trimmed = q.trim().toLowerCase();
    return trimmed === '.' || trimmed === 'done';
  }

  // Mark the path finished by flagging the final element. Only a real
  // terminal (class or literal) can be completed; a dangling property is
  // ignored. Mirrors RichPathBuilder.markComplete.
  function markComplete() {
    if (pathElements.length === 0 || isScopeMode) return;
    const last = pathElements[pathElements.length - 1];
    if (last.type !== 'class' && last.type !== 'literal') return;
    if (last.complete) return;
    pathElements = [...pathElements.slice(0, -1), { ...last, complete: true }];
    query = '';
    suggestions = [];
    showDropdown = false;
  }

  // Clear the completion flag so the path can be extended again.
  function unmarkComplete() {
    if (pathElements.length === 0) return;
    const last = pathElements[pathElements.length - 1];
    if (!last.complete) return;
    const { complete, ...rest } = last;
    pathElements = [...pathElements.slice(0, -1), rest];
    setTimeout(() => inputEl?.focus(), 0);
  }

  function addElement(s: PathSuggestion) {
    const el = suggestionToElement(s, pathElements.length);
    if (isScopeMode) {
      pathElements = [el];
      suggestions = [];
      showDropdown = false;
      query = '';
    } else {
      pathElements = [...pathElements, el];
      query = '';
      showDropdown = false;
      suggestions = [];
      scheduleSearch('');
    }
    setTimeout(() => inputEl?.focus(), 0);
  }

  function undoLast() {
    if (pathElements.length > 0) {
      pathElements = pathElements.slice(0, -1);
      scheduleSearch('');
    }
  }

  function clearAll() {
    pathElements = [];
    suggestions = [];
    showDropdown = false;
    query = '';
    setTimeout(() => inputEl?.focus(), 0);
  }

  function handleFocus() {
    if (query || pathElements.length > 0) scheduleSearch(query);
  }

  function handleBlur() {
    setTimeout(() => { showDropdown = false; }, 150);
  }

  // ---- Additional scope types (multi-class) ----

  function startAddAdditional() {
    addingAdditional = true;
    addQuery = '';
    addSuggestions = [];
    addShowDropdown = false;
    setTimeout(() => addInputEl?.focus(), 0);
  }

  function cancelAddAdditional() {
    addingAdditional = false;
    addQuery = '';
    addSuggestions = [];
    addShowDropdown = false;
    addSelectedIndex = -1;
  }

  function scheduleAddSearch(q: string) {
    if (addDebounceTimer) clearTimeout(addDebounceTimer);
    addDebounceTimer = setTimeout(() => fetchAdditionalSuggestions(q), 200);
  }

  async function fetchAdditionalSuggestions(q: string) {
    const projectId = getProjectId();
    const versionId = getVersionId();
    if (!projectId && !versionId) return;
    addLoading = true;
    try {
      const body: Partial<AutocompleteRequest> = {
        project_id: projectId || undefined,
        version_id: versionId || undefined,
        current_path: [],
        query: q,
        max_results: 20,
        scope_class: '',
        include_inverse: false,
        include_parent_projects: includeParentProjects,
        include_range_suggestions: false,
      };
      const res = await fetch('/api/v1/ontology/autocomplete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) return;
      const payload = parseAutocompleteResponse(await res.json());
      addSuggestions = payload.suggestions.filter(s => isClassType(s.type));
      addSelectedIndex = -1;
      addShowDropdown = addSuggestions.length > 0;
    } catch (err) {
      addSuggestions = [];
      addShowDropdown = false;
      console.error('Additional-class autocomplete failed', err);
    } finally {
      addLoading = false;
    }
  }

  function handleAddInput(e: Event) {
    addQuery = (e.target as HTMLInputElement).value;
    scheduleAddSearch(addQuery);
  }

  function handleAddKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      addSelectedIndex = Math.min(addSelectedIndex + 1, addSuggestions.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      addSelectedIndex = Math.max(addSelectedIndex - 1, -1);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (addSelectedIndex >= 0 && addSuggestions[addSelectedIndex]) {
        addAdditional(addSuggestions[addSelectedIndex]);
      }
    } else if (e.key === 'Escape') {
      cancelAddAdditional();
    }
  }

  function addAdditional(s: PathSuggestion) {
    if (pathElements.length === 0) return;
    const primary = pathElements[0];
    const tr: TypeRef = {
      uri: s.uri || (s.prefix ? `${s.prefix}:${s.local_name}` : s.local_name),
      prefix: s.prefix,
      local_name: s.local_name,
      class_code: extractClassCode(s.local_name),
    };
    // Skip when same as primary or already present.
    if (tr.prefix === primary.prefix && tr.local_name === primary.local_name) {
      cancelAddAdditional();
      return;
    }
    const existing = (primary.additional_types ?? []) as TypeRef[];
    for (const other of existing) {
      if (other.prefix === tr.prefix && other.local_name === tr.local_name) {
        cancelAddAdditional();
        return;
      }
    }
    const next: PathElement = {
      ...primary,
      additional_types: [...existing, tr],
    };
    pathElements = [next];
    cancelAddAdditional();
  }

  function removeAdditional(index: number) {
    if (pathElements.length === 0) return;
    const primary = pathElements[0];
    const existing = (primary.additional_types ?? []) as TypeRef[];
    const next = existing.filter((_, i) => i !== index);
    pathElements = [{
      ...primary,
      additional_types: next.length ? next : undefined,
    }];
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

  <div class="relative">
    <div
      class="min-h-[2.5rem] flex flex-wrap items-center gap-1 px-2 py-1.5 rounded-md border
        {errors.length ? 'border-red-300' : 'border-gray-300'}
        {field.readonly ? 'bg-gray-50' : 'bg-white'}
        focus-within:ring-2 focus-within:ring-pletka-primary focus-within:border-pletka-primary"
    >
      {#each pathElements as el, i}
        {#if i > 0}
          <span class="text-gray-400 text-xs select-none" aria-hidden="true">&rarr;</span>
        {/if}
        <span
          class="inline-flex items-center gap-0.5 px-2 py-0.5 rounded-full text-xs font-medium border bg-gray-100 text-gray-800 border-gray-200"
          title={`${el.prefix}:${el.local_name}`}
        >
          {prefixedName(el)}
          {#if !field.readonly && !isScopeMode}
            <button
              type="button"
              onclick={() => {
                pathElements = pathElements.slice(0, i);
                scheduleSearch('');
                setTimeout(() => inputEl?.focus(), 0);
              }}
              class="ml-0.5 text-current opacity-60 hover:opacity-100 leading-none"
              aria-label="Remove {prefixedName(el)} and everything after"
            >&times;</button>
          {/if}
        </span>
        {#if el.complete}
          <span class="inline-flex items-center gap-0.5 ml-1 pl-1 border-l border-emerald-300 text-emerald-700 text-xs font-medium" title="Path marked complete">
            &check; complete
            {#if !field.readonly}
              <button type="button" class="opacity-60 hover:opacity-100 leading-none" aria-label="Remove completion marker and continue" onclick={() => unmarkComplete()}>&times;</button>
            {/if}
          </span>
        {/if}
      {/each}

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
            ? (isScopeMode ? 'Search for a class…' : 'Search…')
            : (isScopeMode ? '' : 'Search, or type “done” to finish…')}
          class="flex-1 min-w-[6rem] bg-transparent outline-none text-sm text-gray-900 placeholder-gray-400"
          autocomplete="off"
          spellcheck={false}
        />
      {/if}

      {#if pathElements.length > 0 && !field.readonly}
        <div class="ml-auto flex-shrink-0 flex items-center gap-1">
          {#if !isScopeMode && pathElements.length > 1}
            <button type="button" onclick={undoLast} class="text-gray-400 hover:text-gray-600 text-xs" title="Undo last step">Undo</button>
          {/if}
          <button type="button" onclick={clearAll} class="text-gray-400 hover:text-gray-600 text-xs" title="Clear all">Clear</button>
        </div>
      {/if}

      {#if loading}
        <span class="flex-shrink-0 w-4 h-4 border-2 border-gray-300 border-t-pletka-primary rounded-full animate-spin" aria-hidden="true"></span>
      {/if}
    </div>

    {#if isScopeMode && scopeSelected}
      <div class="mt-2 flex flex-wrap items-center gap-1 text-xs">
        <span class="text-gray-500">Additional types:</span>
        {#each additionalTypes as t, i}
          <span
            class="inline-flex items-center gap-0.5 px-2 py-0.5 rounded-full text-xs font-medium border bg-purple-50 text-purple-800 border-purple-200"
            title={`${t.prefix}:${t.local_name}`}
          >
            {t.prefix ? `${t.prefix}:` : ''}{t.local_name}
            {#if !field.readonly}
              <button
                type="button"
                onclick={() => removeAdditional(i)}
                class="ml-0.5 text-current opacity-60 hover:opacity-100 leading-none"
                aria-label="Remove additional type"
              >&times;</button>
            {/if}
          </span>
        {/each}
        {#if !field.readonly}
          {#if addingAdditional}
            <div class="relative inline-block">
              <input
                bind:this={addInputEl}
                type="text"
                value={addQuery}
                oninput={handleAddInput}
                onkeydown={handleAddKeydown}
                onblur={() => setTimeout(() => { addShowDropdown = false; }, 150)}
                placeholder="Search class…"
                class="text-xs px-2 py-0.5 rounded border border-gray-300 outline-none focus:ring-1 focus:ring-pletka-primary"
                autocomplete="off"
                spellcheck={false}
              />
              {#if addLoading}
                <span class="ml-1 inline-block w-3 h-3 border border-gray-300 border-t-pletka-primary rounded-full animate-spin" aria-hidden="true"></span>
              {/if}
              <button
                type="button"
                onclick={cancelAddAdditional}
                class="ml-1 text-gray-400 hover:text-gray-600"
                aria-label="Cancel adding"
              >&times;</button>
              {#if addShowDropdown && addSuggestions.length > 0}
                <ul
                  class="absolute left-0 top-full mt-1 z-30 bg-white border border-gray-200 rounded-md shadow-lg max-h-48 overflow-y-auto min-w-[14rem]"
                  role="listbox"
                >
                  {#each addSuggestions as s, i}
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <!-- svelte-ignore a11y_interactive_supports_focus -->
                    <li
                      role="option"
                      aria-selected={i === addSelectedIndex}
                      class="px-2 py-1 cursor-pointer text-xs {i === addSelectedIndex ? 'bg-pletka-primary text-white' : 'hover:bg-gray-50'}"
                      onmousedown={(e) => { e.preventDefault(); addAdditional(s); }}
                    >
                      {s.prefix ? `${s.prefix}:` : ''}{s.local_name}
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          {:else}
            <button
              type="button"
              onclick={startAddAdditional}
              class="text-pletka-primary hover:text-pletka-secondary text-xs font-medium"
            >+ Add type</button>
          {/if}
        {/if}
      </div>
    {/if}

    {#if showDropdown && suggestions.length > 0}
      <div class="absolute z-20 w-full mt-1" style="position: relative;">
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
              class="flex items-center gap-2 px-3 py-1.5 cursor-pointer text-sm
                {i === selectedIndex ? 'bg-pletka-primary text-white' : 'hover:bg-gray-50'}"
              onmousedown={(e) => { e.preventDefault(); addElement(s); }}
            >
              <span class="flex-1 min-w-0 truncate">
                {s.prefix ? `${s.prefix}:` : ''}{s.local_name}
              </span>
              <span class="flex-shrink-0 text-xs opacity-70">
                {isClassType(s.type) ? 'class' : 'property'}
              </span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>

  {#if errors.length}
    <p class="text-sm text-red-600">{errors[0]}</p>
  {/if}
</div>
