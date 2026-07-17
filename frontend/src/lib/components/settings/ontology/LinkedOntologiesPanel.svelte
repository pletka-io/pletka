<script lang="ts">
  import type { PaneView, PaneGroup, PaneOntology, PaneAvailable } from '$lib/types/ontology-pane';
  import { confirmAction } from '$lib/stores/confirm';
  import BaseOntologyCard from './BaseOntologyCard.svelte';
  import AddOntologyModal from './AddOntologyModal.svelte';

  let { schemaUrl, projectId, lang = 'en', onmutate }: {
    schemaUrl: string;
    // projectId is retained only for the cascade-confirmation message
    // and a shell-level convenience prop. URLs come from the schema —
    // see .claude/rules/api-patterns.md.
    projectId: string;
    lang?: string;
    // Fires after any successful mutation (add / remove / enable /
    // disable / mark-primary). The settings page subscribes to this to
    // refetch its own schema so warnings ('add at least one ontology')
    // clear without a full page reload.
    onmutate?: () => void;
  } = $props();

  let view = $state<PaneView | null>(null);
  let loading = $state<boolean>(true);
  let error = $state<string>('');
  let showAddModal = $state<boolean>(false);

  $effect(() => {
    refresh();
  });

  async function refresh() {
    loading = true;
    error = '';
    try {
      const res = await fetch(schemaUrl);
      if (!res.ok) throw new Error(`Failed to load ontologies: ${res.status}`);
      view = await res.json() as PaneView;
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  async function enableExtension(target: PaneAvailable) {
    if (!target.enable_url) {
      error = 'Enable failed: this extension is read-only for the current viewer';
      return;
    }
    await mutate(async () => {
      const res = await fetch(target.enable_url!, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ontology_id: target.ontology_id,
          version_id: target.version_id,
          extensions: [],
        }),
      });
      if (!res.ok) throw new Error(`Enable failed: ${res.status}`);
    });
  }

  async function disableExtension(ext: PaneOntology) {
    if (!ext.delete_url) {
      error = 'Disable failed: this extension is read-only for the current viewer';
      return;
    }
    const ok = await confirmAction({
      title: 'Disable extension',
      message: 'Existing fields that reference this extension keep working, but no new connections to it can be created.',
      confirmLabel: 'Disable',
      danger: true,
    });
    if (!ok) return;
    await mutate(async () => {
      const res = await fetch(ext.delete_url!, {
        method: 'DELETE',
      });
      if (!res.ok && res.status !== 204) throw new Error(`Disable failed: ${res.status}`);
    });
  }

  async function markPrimary(item: PaneOntology) {
    if (!item.update_url) {
      error = 'Mark primary failed: this ontology is read-only for the current viewer';
      return;
    }
    await mutate(async () => {
      const res = await fetch(item.update_url!, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ is_primary: true }),
      });
      if (!res.ok) throw new Error(`Mark primary failed: ${res.status}`);
    });
  }

  /**
   * Find the group whose base has versionId, so we can show the user
   * which extensions will cascade. Returns the base name + a list of
   * extensions to be removed alongside.
   */
  function cascadeContext(versionId: string): { baseName: string; extensions: string[] } {
    if (!view) return { baseName: '', extensions: [] };
    for (const g of view.groups) {
      if (g.origin?.kind === 'own' && g.base?.version_id === versionId) {
        return {
          baseName: g.base.name || g.base.prefix || 'Unknown',
          extensions: g.extensions.map(e => e.name || e.prefix || 'extension'),
        };
      }
    }
    return { baseName: '', extensions: [] };
  }

  async function removeBase(base: PaneOntology) {
    if (!base.delete_url) {
      error = 'Remove failed: this ontology is read-only for the current viewer';
      return;
    }
    const ctx = cascadeContext(base.version_id);
    let message = `Remove ${ctx.baseName || 'this base ontology'} from the project.`;
    if (ctx.extensions.length > 0) {
      const list = ctx.extensions.join(', ');
      message += `\n\nThis also removes ${ctx.extensions.length} attached extension${ctx.extensions.length === 1 ? '' : 's'}: ${list}.`;
    }
    message += '\n\nFields and overrides that reference removed ontology terms will stop resolving until you re-link or replace them.';

    const ok = await confirmAction({
      title: 'Remove ontology',
      message,
      confirmLabel: 'Remove',
      danger: true,
    });
    if (!ok) return;
    await mutate(async () => {
      const res = await fetch(base.delete_url!, {
        method: 'DELETE',
      });
      if (!res.ok && res.status !== 204) throw new Error(`Remove failed: ${res.status}`);
    });
  }

  async function mutate(action: () => Promise<void>) {
    try {
      await action();
      await refresh();
      onmutate?.();
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function ownGroups(v: PaneView): PaneGroup[] {
    return (v.groups ?? []).filter(g => g.origin?.kind === 'own');
  }
  function inheritedGroups(v: PaneView): PaneGroup[] {
    return (v.groups ?? []).filter(g => g.origin?.kind === 'inherited');
  }
</script>

<div class="space-y-4">
  {#if loading}
    <div class="animate-pulse space-y-3">
      <div class="h-24 bg-gray-200 rounded"></div>
      <div class="h-24 bg-gray-200 rounded"></div>
    </div>
  {:else if error}
    <div class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800">
      {error}
    </div>
  {:else if view}
    {@const inherited = inheritedGroups(view)}
    {@const own = ownGroups(view)}

    {#if inherited.length > 0}
      <section>
        <h3 class="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2 flex items-center gap-1">
          <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
          </svg>
          Inherited from parent project
        </h3>
        <div class="space-y-3">
          {#each inherited as g (g.base_version_id || g.origin?.source_project_id)}
            <BaseOntologyCard
              group={g}
              {lang}
              readonly
            />
          {/each}
        </div>
      </section>
    {/if}

    <section>
      {#if inherited.length > 0}
        <h3 class="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2 flex items-center gap-1">
          <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          This project
        </h3>
      {/if}

      {#if own.length === 0 && inherited.length === 0}
        <div class="rounded-md border border-dashed border-gray-300 bg-gray-50 p-6 text-center">
          <p class="text-sm text-gray-600 mb-3">No ontologies linked yet.</p>
          <p class="text-xs text-gray-500">Add a base ontology (CIDOC-CRM, Linked Art, etc.) to start configuring this project.</p>
        </div>
      {:else}
        <div class="space-y-3">
          {#each own as g (g.base_version_id)}
            <BaseOntologyCard
              group={g}
              {lang}
              onMarkPrimary={markPrimary}
              onRemoveBase={removeBase}
              onEnableExtension={enableExtension}
              onDisableExtension={disableExtension}
            />
          {/each}
        </div>
      {/if}

      {#if view.actions?.create_url}
        <div class="mt-4">
          <button
            type="button"
            onclick={() => { showAddModal = true; }}
            class="inline-flex items-center px-3 py-1.5 border border-gray-300 shadow-sm text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-pletka-primary"
          >
            <svg class="w-4 h-4 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
            </svg>
            Add ontology
          </button>
        </div>
      {/if}
    </section>
  {/if}

  {#if view?.actions?.form_schema_url}
    <AddOntologyModal
      formSchemaUrl={view.actions.form_schema_url}
      bind:open={showAddModal}
      {lang}
      onsuccess={() => { refresh(); onmutate?.(); }}
    />
  {/if}
</div>
