<script lang="ts">
  import type { ParsedPath, PathElement } from '$lib/types/path-types';
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';

  let {
    projectId = '',
    probeSchemaUrl = '',
    inheritanceTreeUrl = '',
  }: {
    projectId: string;
    // Schema-driven URLs. When the probe is mounted by ProjectSettings
    // both come from the settings pane schema. The standalone
    // autocomplete-playground island (admin diagnostic) only knows
    // projectId, so the component falls back to the canonical
    // settings probe + inheritance-tree paths — unique to this
    // diagnostic surface and acceptable for that mount-point.
    probeSchemaUrl?: string;
    inheritanceTreeUrl?: string;
  } = $props();

  // --- Options state ---
  let probeValues = $state<Record<string, any>>({
    project_id: '',
    include_parent_projects: true,
    include_inverse: false,
    ontology_scope: null,
    ontology_path: null,
  });

  // --- Path values ---
  const scopeValue = $derived(probeValues.ontology_scope ?? null);
  const pathValue = $derived(probeValues.ontology_path ?? null);
  const includeParent = $derived(Boolean(probeValues.include_parent_projects ?? true));
  const includeInverse = $derived(Boolean(probeValues.include_inverse ?? false));
  const schemaUrl = $derived(probeSchemaUrl || `/projects/${projectId}/settings/form-schema/autocomplete`);
  const treeUrl = $derived(inheritanceTreeUrl || `/projects/${projectId}/inheritance-tree`);

  // --- Inheritance tree ---
  let inheritanceHtml = $state('<div class="text-gray-500 italic">Loading...</div>');

  // --- Form values passed through FormRenderer ---
  $effect(() => {
    if (probeValues.project_id !== projectId) {
      probeValues = { ...probeValues, project_id: projectId };
    }
  });

  // --- Parsed output for display ---
  const parsedScope = $derived.by(() => {
    if (!scopeValue) return null;
    if (typeof scopeValue === 'object') return scopeValue;
    if (typeof scopeValue === 'string') {
      try {
        return JSON.parse(scopeValue);
      } catch {
        return null;
      }
    }
    return null;
  });

  const parsedPath = $derived.by(() => {
    if (!pathValue) return null;
    if (Array.isArray(pathValue)) {
      const elements = pathValue as PathElement[];
      const prefixed = (el: PathElement) => el.prefix ? `${el.prefix}:${el.local_name}` : el.local_name;
      return {
        elements,
        terminal_type: elements.length > 0 ? elements[elements.length - 1]?.type ?? '—' : '—',
        short_path: elements.map(prefixed).join(' → '),
        long_path: elements.map(prefixed).join(' → '),
      } as ParsedPath;
    }
    if (typeof pathValue === 'string') {
      try {
        return JSON.parse(pathValue) as ParsedPath;
      } catch {
        return null;
      }
    }
    return null;
  });

  // --- Extract scope class from scope value for path builder filtering ---
  // Uses prefix:LocalName format (e.g., "crm:E21_Person") for unambiguous cache lookup
  const selectedScopeClass = $derived.by(() => {
    if (!parsedScope) return '';
    const scope = parsedScope as any;
    if (Array.isArray(scope?.elements) && scope.elements.length > 0) {
      const el = scope.elements[0];
      if (!el) return '';
      if (el.prefix) return `${el.prefix}:${el.local_name}`;
      return el.local_name ?? '';
    }
    if (scope.local_name) {
      if (scope.prefix) return `${scope.prefix}:${scope.local_name}`;
      return scope.local_name;
    }
    return '';
  });

  const elementCount = $derived(parsedPath?.elements?.length ?? 0);
  const terminalType = $derived(parsedPath?.terminal_type ?? '—');
  const shortPath = $derived(parsedPath?.short_path ?? '');
  const longPath = $derived(parsedPath?.long_path ?? '');

  // --- Load inheritance tree ---
  async function loadInheritanceTree() {
    try {
      const response = await fetch(treeUrl);
      const data = await response.json();
      if (data.success) {
        inheritanceHtml = buildTreeHtml(data);
      } else {
        inheritanceHtml = '<div class="text-red-500">Failed to load</div>';
      }
    } catch {
      inheritanceHtml = '<div class="text-red-500">Failed to load inheritance tree</div>';
    }
  }

  function buildTreeHtml(data: any): string {
    let html = '<div class="space-y-3">';

    // Current project
    html += `<div class="flex items-center gap-2">
      <span class="font-semibold text-gray-900">${data.project_name}</span>
      <span class="px-2 py-0.5 bg-green-100 text-green-800 text-xs rounded">Current</span>
    </div>`;

    if (data.direct_ontologies?.length > 0) {
      html += '<div class="ml-4 flex flex-wrap gap-2">';
      for (const onto of data.direct_ontologies) {
        const star = onto.is_primary ? ' ★' : '';
        html += `<span class="inline-flex items-center px-2 py-1 bg-indigo-50 border border-indigo-200 rounded text-xs">
          <span class="font-medium text-indigo-900">${onto.name}</span>
          <span class="ml-1 text-indigo-600">(${onto.version})${star}</span>
        </span>`;
      }
      html += '</div>';
    }

    // Parent projects
    if (data.parent_projects?.length > 0) {
      html += '<div class="border-l-2 border-gray-300 ml-4 pl-4 space-y-2">';
      html += '<div class="text-xs text-gray-500">Inherited from parents:</div>';
      for (const [idx, parent] of data.parent_projects.entries()) {
        const level = idx === 0 ? 'Parent' : idx === 1 ? 'Grandparent' : `Ancestor (${idx + 1})`;
        html += `<div class="flex items-center gap-2">
          <span class="font-medium text-blue-900">${parent.name}</span>
          <span class="px-2 py-0.5 bg-blue-100 text-blue-700 text-xs rounded">${level}</span>
        </div>`;
        if (parent.ontologies?.length > 0) {
          html += '<div class="ml-4 flex flex-wrap gap-2">';
          for (const onto of parent.ontologies) {
            const star = onto.is_primary ? ' ★' : '';
            html += `<span class="inline-flex items-center px-2 py-1 bg-white border border-blue-200 rounded text-xs">
              <span class="text-blue-800">${onto.name}</span>
              <span class="ml-1 text-blue-500">(${onto.version})${star}</span>
            </span>`;
          }
          html += '</div>';
        }
      }
      html += '</div>';
    }

    html += '</div>';
    return html;
  }

  // Load tree on mount
  $effect(() => {
    if (projectId) {
      loadInheritanceTree();
    }
  });
</script>

<div class="space-y-6">
  <!-- Inheritance Overview -->
  <div class="pb-6 border-b border-gray-200">
    <h4 class="text-md font-medium text-gray-900 mb-3">Ontology Inheritance</h4>
    <div class="text-sm">
      {@html inheritanceHtml}
    </div>
  </div>

  <!-- Options -->
  <div class="flex items-center gap-6">
    <span class="text-sm text-gray-700">
      Include parent projects: <span class="font-medium">{includeParent ? 'Yes' : 'No'}</span>
    </span>
    <span class="text-sm text-gray-700">
      Include inverse properties: <span class="font-medium">{includeInverse ? 'Yes' : 'No'}</span>
    </span>
  </div>

  <FormRenderer
    {schemaUrl}
    bind:formValues={probeValues}
    onSuccessAction="none"
  />

  {#if selectedScopeClass}
    <div class="text-xs text-gray-500">
      Filtering properties by scope: <span class="font-medium text-indigo-700">{selectedScopeClass}</span>
    </div>
  {/if}

  <!-- Output Inspector -->
  {#if parsedPath || parsedScope}
    <div class="bg-gray-50 rounded-lg p-4 space-y-3">
      <h4 class="text-sm font-medium text-gray-700">Output Inspector</h4>

      {#if parsedScope}
        <div>
          <span class="text-xs font-medium text-gray-500 uppercase">Scope</span>
          <pre class="mt-1 text-xs bg-white border border-gray-200 rounded p-2 overflow-x-auto">{JSON.stringify(parsedScope, null, 2)}</pre>
        </div>
      {/if}

      {#if parsedPath}
        <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
          <div class="bg-white rounded p-2 border border-gray-200">
            <div class="text-xs text-gray-500">Elements</div>
            <div class="text-lg font-bold text-gray-900">{elementCount}</div>
          </div>
          <div class="bg-white rounded p-2 border border-gray-200">
            <div class="text-xs text-gray-500">Terminal</div>
            <div class="text-lg font-bold text-gray-900">{terminalType}</div>
          </div>
          <div class="bg-white rounded p-2 border border-gray-200 col-span-2">
            <div class="text-xs text-gray-500">Version</div>
            <div class="text-lg font-bold text-gray-900">{parsedPath.version}</div>
          </div>
        </div>

        {#if longPath}
          <div>
            <span class="text-xs font-medium text-gray-500 uppercase">Long Path</span>
            <div class="mt-1 text-sm font-mono bg-white border border-gray-200 rounded p-2 break-all">{longPath}</div>
          </div>
        {/if}

        {#if shortPath}
          <div>
            <span class="text-xs font-medium text-gray-500 uppercase">Short Path</span>
            <div class="mt-1 text-sm font-mono bg-white border border-gray-200 rounded p-2 break-all">{shortPath}</div>
          </div>
        {/if}

        <div>
          <span class="text-xs font-medium text-gray-500 uppercase">Full JSON</span>
          <pre class="mt-1 text-xs bg-white border border-gray-200 rounded p-2 overflow-x-auto max-h-64">{JSON.stringify(parsedPath, null, 2)}</pre>
        </div>
      {/if}
    </div>
  {/if}
</div>
