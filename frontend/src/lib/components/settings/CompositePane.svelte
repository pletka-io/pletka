<script lang="ts">
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';
  import ListManager from '$lib/components/list/ListManager.svelte';
  import LinkedOntologiesPanel from '$lib/components/settings/ontology/LinkedOntologiesPanel.svelte';
  import IntegrationsPanel from '$lib/components/settings/IntegrationsPanel.svelte';
  import type { CompositePaneSchema } from '$lib/types/composite-pane-schema';
  import { tr } from '$lib/types/form-schema';

  let { schemaUrl, lang = 'en', projectId = '', onmutate }: {
    schemaUrl: string;
    lang?: string;
    projectId?: string;
    // Fires whenever a child panel reports a successful mutation. The
    // settings page uses this to refetch its own schema so derived UI
    // (e.g. 'add an ontology' warning, NeedsAttention dot on the
    // sidebar) reflects the new state without a full reload.
    onmutate?: () => void;
  } = $props();

  let pane = $state<CompositePaneSchema | null>(null);
  let childKinds = $state<Record<string, 'form' | 'list'>>({});
  let error = $state<string>('');
  let loading = $state<boolean>(true);
  // Bumped on any child-form success. List panels remount on it so they
  // refetch — saving a new parent project should immediately show the
  // updated inherited ontologies in the sibling list.
  let refreshNonce = $state(0);

  $effect(() => {
    load(schemaUrl);
  });

  async function probeChildKind(url: string): Promise<'form' | 'list'> {
    try {
      const res = await fetch(url);
      if (!res.ok) return 'form';
      const data = await res.json();
      if (data?.kind === 'list-schema' || Array.isArray(data?.columns)) {
        return 'list';
      }
      return 'form';
    } catch {
      return 'form';
    }
  }

  async function load(url: string) {
    loading = true;
    error = '';
    try {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`Failed to load pane: ${res.status}`);
      const data = await res.json();
      pane = data as CompositePaneSchema;

      // Only probe panels that don't declare an explicit kind. Panels
      // routed to a dedicated component (e.g. linked-ontologies) own
      // their own data fetching.
      const kinds: Record<string, 'form' | 'list'> = {};
      await Promise.all(
        pane.panels.map(async (panel) => {
          if (panel.kind) return;
          kinds[panel.id] = await probeChildKind(panel.schema_url);
        }),
      );
      childKinds = kinds;
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }
</script>

{#if loading}
  <div class="animate-pulse space-y-4">
    <div class="h-8 bg-gray-200 rounded w-1/3"></div>
    <div class="h-24 bg-gray-200 rounded"></div>
  </div>
{:else if error}
  <div class="text-red-600">Failed to load pane: {error}</div>
{:else if pane}
  {#each pane.panels as panel (panel.id)}
    <section class="mb-8" data-panel-id={panel.id}>
      {#if panel.label}
        <h2 class="text-lg font-semibold text-gray-900 mb-3">{tr(panel.label, lang)}</h2>
      {/if}
      {#if panel.kind === 'linked-ontologies'}
        {#key `${panel.id}-${refreshNonce}`}
          <LinkedOntologiesPanel
            schemaUrl={panel.schema_url}
            {projectId}
            {lang}
            onmutate={() => onmutate?.()}
          />
        {/key}
      {:else if panel.kind === 'integrations'}
        {#key `${panel.id}-${refreshNonce}`}
          <IntegrationsPanel schemaUrl={panel.schema_url} {lang} />
        {/key}
      {:else if panel.kind === 'list' || childKinds[panel.id] === 'list'}
        {#key `${panel.id}-${refreshNonce}`}
          <ListManager
            schemaUrl={panel.schema_url}
            embedded={!!panel.label}
            onmutate={() => { refreshNonce += 1; onmutate?.(); }}
          />
        {/key}
      {:else if panel.kind === 'form' || childKinds[panel.id] === 'form'}
        <FormRenderer
          schemaUrl={panel.schema_url}
          onSuccessAction="none"
          onsuccess={() => { refreshNonce += 1; onmutate?.(); }}
        />
      {/if}
    </section>
  {/each}
{/if}
