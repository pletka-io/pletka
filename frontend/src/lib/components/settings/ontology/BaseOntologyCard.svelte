<script lang="ts">
  import type { PaneGroup, PaneOntology, PaneAvailable } from '$lib/types/ontology-pane';
  import ExtensionRow from './ExtensionRow.svelte';
  import AvailableExtensionRow from './AvailableExtensionRow.svelte';

  let {
    group,
    lang = 'en',
    readonly = false,
    onMarkPrimary,
    onRemoveBase,
    onEnableExtension,
    onDisableExtension,
  }: {
    group: PaneGroup;
    lang?: string;
    readonly?: boolean;
    // Callbacks receive the full row object so the caller fetches
    // item.update_url / item.delete_url / item.enable_url directly.
    // The schema-driven contract means a missing URL = no button.
    onMarkPrimary?: (item: PaneOntology) => void;
    onRemoveBase?: (item: PaneOntology) => void;
    onEnableExtension?: (avail: PaneAvailable) => void;
    onDisableExtension?: (ext: PaneOntology) => void;
  } = $props();
</script>

<div class="border border-gray-200 rounded-lg overflow-hidden bg-white">
  <!-- Base ontology header -->
  {#if group.base}
    <div class="flex items-start justify-between p-4 bg-gray-50 border-b border-gray-200">
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2 flex-wrap">
          <span class="font-semibold text-gray-900">{group.base.name || group.base.prefix || 'Unknown ontology'}</span>
          {#if group.base.is_primary}
            <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-pletka-primary text-white">
              Primary
            </span>
          {/if}
          <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800">
            <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
              <path d="M10 1l9 5v8l-9 5-9-5V6l9-5zm0 2.236L3 7v6l7 3.882L17 13V7l-7-3.764z"/>
            </svg>
            Base
          </span>
          {#if readonly}
            <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800">
              Inherited{#if group.origin?.source_project_label} from {group.origin.source_project_label}{/if}
            </span>
          {/if}
        </div>
        <div class="mt-1 text-sm text-gray-600 flex items-center gap-2">
          <span class="font-mono text-xs">{group.base.prefix || '—'}</span>
          <span>•</span>
          <span>v{group.base.version_string || '—'}</span>
        </div>
        {#if group.base.usage_notes}
          <div class="mt-2 text-sm text-gray-600">{group.base.usage_notes}</div>
        {/if}
      </div>

      {#if !readonly}
        <div class="ml-4 flex flex-shrink-0 items-center gap-3">
          {#if !group.base.is_primary && onMarkPrimary && group.base.update_url}
            <button
              type="button"
              onclick={() => onMarkPrimary?.(group.base!)}
              class="text-sm text-pletka-primary hover:text-pletka-secondary"
            >Mark Primary</button>
          {/if}
          {#if onRemoveBase && group.base.delete_url}
            <button
              type="button"
              onclick={() => onRemoveBase?.(group.base!)}
              class="text-sm text-red-600 hover:text-red-800"
            >Remove</button>
          {/if}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Enabled extensions -->
  {#if (group.extensions ?? []).length > 0}
    <div class="px-4 py-2 bg-white">
      <div class="text-xs font-medium text-gray-500 uppercase tracking-wider mb-1.5">
        Extensions ({group.extensions!.length})
      </div>
      <div class="divide-y divide-gray-100">
        {#each group.extensions! as ext (ext.version_id)}
          <ExtensionRow
            extension={ext}
            {lang}
            {readonly}
            onDisable={onDisableExtension}
          />
        {/each}
      </div>
    </div>
  {/if}

  <!-- Available extensions to enable -->
  {#if !readonly && group.available_extensions && group.available_extensions.length > 0}
    <div class="px-4 py-3 bg-gray-50 border-t border-gray-200">
      <div class="text-xs font-medium text-gray-500 uppercase tracking-wider mb-1.5">
        Available extensions
      </div>
      <div class="divide-y divide-gray-100">
        {#each group.available_extensions as avail (avail.version_id)}
          <AvailableExtensionRow
            available={avail}
            {lang}
            onEnable={onEnableExtension}
          />
        {/each}
      </div>
    </div>
  {/if}
</div>
