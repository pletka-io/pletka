<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import { fade } from 'svelte/transition';

  let {
    statsUrl,
    lang,
    onclose,
  }: {
    statsUrl: string;
    lang: string;
    onclose: () => void;
  } = $props();

  let data = $state<any>(null);
  let loading = $state(true);
  let error = $state('');

  const COLLAPSE_THRESHOLD = 5;

  let sections = $state<Record<string, boolean>>({});

  $effect(() => {
    fetchStats();
  });

  async function fetchStats() {
    loading = true;
    try {
      const res = await fetch(statsUrl);
      if (!res.ok) throw new Error(`Failed to load stats: ${res.status}`);
      data = await res.json();

      // Auto-expand small sections, collapse large ones
      sections = {
        fields: (data.fields?.length || 0) <= COLLAPSE_THRESHOLD,
        model_fields: (data.model_fields?.length || 0) <= COLLAPSE_THRESHOLD,
        collection_fields: (data.collection_fields?.length || 0) <= COLLAPSE_THRESHOLD,
      };
    } catch (e: any) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function toggle(key: string) {
    sections = { ...sections, [key]: !sections[key] };
  }

  function isCategoryStats(payload: any): boolean {
    return !!payload?.category;
  }

  function isNamespaceStats(payload: any): boolean {
    return !!payload?.namespace_binding;
  }

  function namespaceProjectUsages(payload: any): any[] {
    return payload?.project_usages || [];
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') onclose(); }} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto" transition:fade={{duration: 150}}>
  <div class="fixed inset-0 bg-gray-500 bg-opacity-75" onclick={onclose}></div>
  <div class="relative bg-white rounded-lg shadow-xl max-w-lg w-full p-6 m-4">
    <!-- Header -->
    <div class="flex justify-between items-start mb-4">
      <h3 class="text-lg font-medium text-gray-900">
        {#if isCategoryStats(data)}
          {tr(data.category.ui_name, lang) || data.category.system_name} — Usage Statistics
        {:else if isNamespaceStats(data)}
          {data.namespace_binding.prefix} — Namespace Statistics
        {:else}
          Loading...
        {/if}
      </h3>
      <button aria-label="Close" onclick={onclose} class="text-gray-400 hover:text-gray-500">
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
        </svg>
      </button>
    </div>

    {#if loading}
      <div class="animate-pulse space-y-3">
        <div class="h-4 bg-gray-200 rounded w-1/2"></div>
        <div class="h-4 bg-gray-200 rounded w-3/4"></div>
        <div class="h-4 bg-gray-200 rounded w-1/3"></div>
      </div>
    {:else if error}
      <p class="text-red-600 text-sm">{error}</p>
    {:else if data}
      {#if isNamespaceStats(data)}
        <div class="space-y-4">
          <div>
            <p class="text-sm text-gray-500">
              Namespace:
              <code class="text-xs bg-gray-100 px-1 py-0.5 rounded">{data.namespace_binding.namespace}</code>
            </p>
            <div class="mt-2 flex flex-wrap gap-2">
              <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-700">
                {data.project_count} projects
              </span>
              <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-purple-50 text-purple-700">
                {data.total_usage} total references
              </span>
              <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-700">
                {data.namespace_binding.source}
              </span>
            </div>
          </div>

          <div>
            <h4 class="text-sm font-medium text-gray-900 mb-2">Projects using this namespace</h4>
            {#if namespaceProjectUsages(data).length === 0}
              <p class="text-sm text-gray-400 italic">No project references found yet.</p>
            {:else}
              <ul class="space-y-2 max-h-72 overflow-y-auto">
                {#each namespaceProjectUsages(data) as usage}
                  <li class="rounded-md border border-gray-200 px-3 py-2">
                    <div class="flex items-center justify-between gap-3">
                      <div>
                        <p class="text-sm font-medium text-gray-900">{usage.project_id}</p>
                        <p class="text-xs text-gray-500">
                          {usage.total_count} total references
                        </p>
                      </div>
                      <div class="flex flex-wrap gap-2 justify-end">
                        {#if usage.field_count > 0}
                          <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-700">
                            {usage.field_count} fields
                          </span>
                        {/if}
                        {#if usage.model_count > 0}
                          <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-purple-50 text-purple-700">
                            {usage.model_count} models
                          </span>
                        {/if}
                        {#if usage.collection_count > 0}
                          <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-green-50 text-green-700">
                            {usage.collection_count} collections
                          </span>
                        {/if}
                      </div>
                    </div>
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        </div>
      {:else}
      <!-- Summary -->
      <div class="mb-4">
        <p class="text-sm text-gray-500">
          Semantic ID: <code class="text-xs bg-gray-100 px-1 py-0.5 rounded">{data.category.semantic_id}</code>
        </p>
        <p class="text-sm font-medium text-gray-700 mt-2">
          Total usage: {data.total_usage}
        </p>
      </div>

      <!-- Fields -->
      {@const fields = data.fields || []}
      <div class="mb-4">
        <button type="button" class="flex items-center gap-1 text-sm font-medium text-gray-900 mb-1 w-full text-left"
          onclick={() => toggle('fields')}>
          <svg class="w-3 h-3 transition-transform {sections.fields ? 'rotate-90' : ''}" fill="currentColor" viewBox="0 0 20 20">
            <path d="M6 4l8 6-8 6V4z"/>
          </svg>
          Fields ({fields.length})
        </button>
        {#if fields.length > 0}
          <p class="text-xs text-amber-600 mb-1">These assignments block deletion</p>
        {/if}
        {#if sections.fields && fields.length > 0}
          <ul class="space-y-1 max-h-48 overflow-y-auto">
            {#each fields as f}
              <li class="text-sm text-gray-600 flex items-center">
                <span class="w-1.5 h-1.5 bg-blue-400 rounded-full mr-2 flex-shrink-0"></span>
                {tr(f.ui_name, lang) || f.identifier || f.id}
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      <!-- Model Overrides -->
      {@const modelFields = data.model_fields || []}
      <div class="mb-4">
        <button type="button" class="flex items-center gap-1 text-sm font-medium text-gray-900 mb-1 w-full text-left"
          onclick={() => toggle('model_fields')}>
          <svg class="w-3 h-3 transition-transform {sections.model_fields ? 'rotate-90' : ''}" fill="currentColor" viewBox="0 0 20 20">
            <path d="M6 4l8 6-8 6V4z"/>
          </svg>
          Model Overrides ({modelFields.length})
        </button>
        {#if sections.model_fields && modelFields.length > 0}
          <ul class="space-y-1 max-h-48 overflow-y-auto">
            {#each modelFields as mf}
              <li class="text-sm text-gray-600 flex items-center">
                <span class="w-1.5 h-1.5 bg-purple-400 rounded-full mr-2 flex-shrink-0"></span>
                {mf.model_name} &rarr; {mf.field_name}
              </li>
            {/each}
          </ul>
        {:else if modelFields.length === 0}
          <p class="text-sm text-gray-400 italic">No model overrides</p>
        {/if}
      </div>

      <!-- Collection Overrides -->
      {@const collFields = data.collection_fields || []}
      <div class="mb-4">
        <button type="button" class="flex items-center gap-1 text-sm font-medium text-gray-900 mb-1 w-full text-left"
          onclick={() => toggle('collection_fields')}>
          <svg class="w-3 h-3 transition-transform {sections.collection_fields ? 'rotate-90' : ''}" fill="currentColor" viewBox="0 0 20 20">
            <path d="M6 4l8 6-8 6V4z"/>
          </svg>
          Collection Overrides ({collFields.length})
        </button>
        {#if sections.collection_fields && collFields.length > 0}
          <ul class="space-y-1 max-h-48 overflow-y-auto">
            {#each collFields as cf}
              <li class="text-sm text-gray-600 flex items-center">
                <span class="w-1.5 h-1.5 bg-green-400 rounded-full mr-2 flex-shrink-0"></span>
                {cf.collection_name} &rarr; {cf.field_name}
              </li>
            {/each}
          </ul>
        {:else if collFields.length === 0}
          <p class="text-sm text-gray-400 italic">No collection overrides</p>
        {/if}
      </div>

      <!-- Override note -->
      {#if modelFields.length > 0 || collFields.length > 0}
        <p class="text-xs text-gray-500 italic mb-4">Overrides will be cleared when this item is deleted</p>
      {/if}
      {/if}
    {/if}
  </div>
</div>
