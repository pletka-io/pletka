<script lang="ts">
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';
  import type { ProjectReleaseTabSchema } from '$lib/types/project-page';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import Badge from '$lib/components/shared/Badge.svelte';

  let { schema }: { schema: ProjectReleaseTabSchema } = $props();

  const lang = getUILang();
  let showCreate = $state(false);

  function formatDate(value: string): string {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return new Intl.DateTimeFormat(lang, {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(date);
  }
</script>

<div class="space-y-6">
  <div class="flex flex-wrap items-start justify-between gap-4 rounded-lg border border-gray-200 bg-white p-5">
    <div>
      <h2 class="text-lg font-semibold text-gray-900">Releases</h2>
      <p class="mt-1 text-sm text-gray-600">
        Frozen project snapshots. Open one to browse the project in read-only release mode.
      </p>
    </div>
    <div class="flex items-center gap-3">
      {#if schema.current_version}
        <a
          href={schema.draft_url}
          class="inline-flex items-center rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          Return to draft
        </a>
      {/if}
      {#if schema.can_create && schema.create_form_schema_url}
        <button
          type="button"
          class="inline-flex items-center rounded-md bg-pletka-primary px-4 py-2 text-sm font-medium text-white hover:bg-pletka-secondary"
          onclick={() => { showCreate = !showCreate; }}
        >
          {showCreate ? 'Close' : 'Create release'}
        </button>
      {/if}
    </div>
  </div>

  {#if !schema.current_version && schema.draft_parent_dependencies?.length}
    <div class="rounded-lg border border-amber-200 bg-amber-50 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-sm font-semibold text-amber-900">Parent dependencies still follow draft</h3>
          <p class="mt-1 text-sm text-amber-800">
            {tr(schema.create_blocked_message, lang, 'Pin every parent dependency to a named release before creating a child release.')}
          </p>
          <div class="mt-3 flex flex-wrap gap-2">
            {#each schema.draft_parent_dependencies as dep (dep.parent_project_id)}
              <Badge variant={dep.primary ? 'blue' : 'amber'} size="sm">
                {tr(dep.label, lang, dep.parent_project_id)}{dep.primary ? ' · primary' : ''}
              </Badge>
            {/each}
          </div>
        </div>
        {#if schema.dependency_settings_url}
          <a
            href={schema.dependency_settings_url}
            class="inline-flex items-center rounded-md border border-amber-300 bg-white px-3 py-2 text-sm font-medium text-amber-900 hover:bg-amber-100"
          >
            Pin parent releases
          </a>
        {/if}
      </div>
    </div>
  {/if}

  {#if showCreate && schema.create_form_schema_url}
    <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm">
      <h3 class="mb-4 text-base font-semibold text-gray-900">Create release</h3>
      <FormRenderer
        schemaUrl={schema.create_form_schema_url}
        oncancel={() => { showCreate = false; }}
      />
    </div>
  {/if}

  {#if schema.items.length === 0}
    <div class="rounded-lg border border-dashed border-gray-300 bg-gray-50 p-8 text-sm text-gray-600">
      {tr(schema.empty_message, lang, 'No releases yet.')}
    </div>
  {:else}
    <div class="space-y-3">
      {#each schema.items as item (item.version)}
        <a
          href={item.view_url}
          class="block rounded-lg border border-gray-200 bg-white p-5 transition hover:border-blue-300 hover:shadow-sm"
        >
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-base font-semibold text-gray-900">
                  {item.title || tr(item.label, lang, item.version)}
                </h3>
                <span class="font-mono text-sm text-gray-500">{item.version}</span>
                {#if item.active}
                  <Badge variant="blue" size="sm">current</Badge>
                {/if}
              </div>
              {#if item.description}
                <p class="mt-1 text-sm text-gray-600">{item.description}</p>
              {/if}
            </div>
            <div class="text-right text-sm text-gray-500">
              <div>{formatDate(item.created_at)}</div>
              {#if item.created_by_id}
                <div class="mt-1 font-mono text-xs text-gray-400">{item.created_by_id}</div>
              {/if}
            </div>
          </div>
        </a>
      {/each}
    </div>
  {/if}
</div>
