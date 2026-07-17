<script lang="ts">
  import type { EntityViewState } from '$lib/detailview/state.svelte';
  import Badge from '$lib/components/shared/Badge.svelte';

  let {
    state,
  }: {
    state: EntityViewState;
  } = $props();

  const entity = $derived(state.response?.entity);

  const uiName = $derived(() => {
    if (!entity) return {};
    return (entity.name as Record<string, string>) ?? {};
  });

  const description = $derived(() => {
    if (!entity) return {};
    return (entity.description as Record<string, string>) ?? {};
  });

  const semanticId = $derived(entity?.id || '');
  const status = $derived(entity?.status || '');
  const modelType = $derived(entity?.model_type || '');
  const isModel = $derived(entity?.type === 'model');
  const isConceptList = $derived(entity?.type === 'concept-list');
  const createdAt = $derived(entity?.created_at || '');
  const updatedAt = $derived(entity?.updated_at || '');
  const adoptions = $derived(state.response?.adoptions || []);

  function modelTypeLabel(t: string): string {
    switch (t) {
      case 'core': return 'Core';
      case 'auxiliary': return 'Auxiliary';
      case 'example': return 'Example';
      default: return t || 'Auxiliary';
    }
  }
  function modelTypeVariant(t: string): 'blue' | 'gray' | 'yellow' {
    switch (t) {
      case 'core': return 'blue';
      case 'auxiliary': return 'gray';
      case 'example': return 'yellow';
      default: return 'gray';
    }
  }

  const ontologyScope = $derived(() => {
    if (!entity?.ontology_scope) return '';
    return `${entity.ontology_scope.prefix}:${entity.ontology_scope.local_name}`;
  });

  function formatDate(d: string | Date): string {
    if (!d) return '-';
    const date = typeof d === 'string' ? new Date(d) : d;
    return date.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
  }
</script>

<div class="bg-white shadow-sm rounded-lg divide-y divide-gray-200">
  <!-- Identity -->
  <div class="p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">Identity</h3>
    <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-4">
      <div>
        <dt class="text-sm font-medium text-gray-500">Semantic ID</dt>
        <dd class="mt-1 text-sm text-gray-900">{semanticId || '-'}</dd>
      </div>
      <div>
        <dt class="text-sm font-medium text-gray-500">Status</dt>
        <dd class="mt-1">
          {#if status}
            <Badge variant={status === 'published' ? 'green' : status === 'draft' ? 'gray' : 'yellow'} size="sm">
              {status}
            </Badge>
          {:else}
            <span class="text-sm text-gray-400">-</span>
          {/if}
        </dd>
      </div>
      {#if !isConceptList}
        <div>
          <dt class="text-sm font-medium text-gray-500">Ontology Scope</dt>
          <dd class="mt-1">
            {#if ontologyScope()}
              <Badge variant="purple" size="sm">{ontologyScope()}</Badge>
            {:else}
              <span class="text-sm text-gray-400">Not set</span>
            {/if}
          </dd>
        </div>
      {/if}
      {#if isModel}
        <div>
          <dt class="text-sm font-medium text-gray-500">Model Type</dt>
          <dd class="mt-1">
            <Badge variant={modelTypeVariant(modelType)} size="sm">{modelTypeLabel(modelType)}</Badge>
          </dd>
        </div>
      {/if}
      {#if isConceptList}
        <div>
          <dt class="text-sm font-medium text-gray-500">Source Vocabulary</dt>
          <dd class="mt-1 text-sm text-gray-900">
            {entity?.source_vocabulary ? entity.source_vocabulary.name?.en || entity.source_vocabulary.system_name || entity.source_vocabulary.id : entity?.vocabulary_label || entity?.vocabulary_id || 'Not set'}
          </dd>
        </div>
        <div>
          <dt class="text-sm font-medium text-gray-500">Parent Term</dt>
          <dd class="mt-1 text-sm text-gray-900">
            {#if entity?.parent_term?.uri || entity?.parent_term_uri || entity?.list_type_uri}
              <div>{entity?.parent_term?.label?.en || entity?.list_type_label || entity?.parent_term?.uri || entity?.parent_term_uri || entity?.list_type_uri}</div>
              <a class="mt-1 block truncate font-mono text-xs text-blue-700 hover:text-blue-900" href={entity?.parent_term?.uri || entity?.parent_term_uri || entity.list_type_uri} target="_blank" rel="noreferrer">
                {entity?.parent_term?.uri || entity?.parent_term_uri || entity.list_type_uri}
              </a>
            {:else}
              Not set
            {/if}
          </dd>
        </div>
        <div>
          <dt class="text-sm font-medium text-gray-500">Entries</dt>
          <dd class="mt-1 text-sm text-gray-900">{entity?.entry_count ?? 0}</dd>
        </div>
      {/if}
    </dl>
  </div>

  <!-- Multilingual Names -->
  <div class="p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">Name (multilingual)</h3>
    {#if Object.keys(uiName()).length > 0}
      <dl class="space-y-2">
        {#each Object.entries(uiName()) as [lang, value]}
          <div class="flex items-center">
            <dt class="w-12 text-xs font-medium text-gray-400 uppercase">{lang}</dt>
            <dd class="text-sm text-gray-900">{value}</dd>
          </div>
        {/each}
      </dl>
    {:else}
      <p class="text-sm text-gray-400">No names set</p>
    {/if}
  </div>

  <!-- Multilingual Descriptions -->
  <div class="p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">Description (multilingual)</h3>
    {#if Object.keys(description()).length > 0}
      <dl class="space-y-2">
        {#each Object.entries(description()) as [lang, value]}
          <div class="flex items-start">
            <dt class="w-12 text-xs font-medium text-gray-400 uppercase pt-0.5">{lang}</dt>
            <dd class="text-sm text-gray-900">{value}</dd>
          </div>
        {/each}
      </dl>
    {:else}
      <p class="text-sm text-gray-400">No descriptions set</p>
    {/if}
  </div>

  <!-- Timestamps -->
  <div class="p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">Timestamps</h3>
    <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-4">
      <div>
        <dt class="text-sm font-medium text-gray-500">Created</dt>
        <dd class="mt-1 text-sm text-gray-900">{formatDate(createdAt)}</dd>
      </div>
      <div>
        <dt class="text-sm font-medium text-gray-500">Last Updated</dt>
        <dd class="mt-1 text-sm text-gray-900">{formatDate(updatedAt)}</dd>
      </div>
    </dl>
  </div>

  {#if adoptions.length > 0}
    <div class="p-6">
      <h3 class="text-lg font-semibold text-gray-900 mb-4">Adoptions</h3>
      <p class="mb-4 text-sm text-gray-600">
        Explicit reuse receipts captured for this {entity?.type || 'entity'}.
      </p>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 text-sm">
          <thead class="bg-gray-50">
            <tr>
              <th class="px-4 py-2 text-left font-medium text-gray-500">Type</th>
              <th class="px-4 py-2 text-left font-medium text-gray-500">Source</th>
              <th class="px-4 py-2 text-left font-medium text-gray-500">Entity</th>
              <th class="px-4 py-2 text-left font-medium text-gray-500">Adopted</th>
              <th class="px-4 py-2 text-left font-medium text-gray-500">By</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 bg-white">
            {#each adoptions as item (`${item.entity_type}:${item.source_project_id}:${item.source_entity_id}`)}
              <tr>
                <td class="px-4 py-3">
                  <Badge variant="blue" size="xs">{item.entity_type}</Badge>
                </td>
                <td class="px-4 py-3 font-mono text-gray-600">{item.source_project_id}</td>
                <td class="px-4 py-3">
                  {#if item.source_url}
                    <a href={item.source_url} class="font-medium text-blue-700 hover:text-blue-900">
                      {item.source_entity_id}
                    </a>
                  {:else}
                    <span class="font-medium text-gray-900">{item.source_entity_id}</span>
                  {/if}
                </td>
                <td class="px-4 py-3 text-gray-600">{formatDate(item.adopted_at)}</td>
                <td class="px-4 py-3 text-gray-600">{item.created_by?.label || item.created_by?.id || '—'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>
