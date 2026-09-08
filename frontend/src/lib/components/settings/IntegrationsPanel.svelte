<script lang="ts">
  /**
   * Integrations settings panel. Per-integration cards list every
   * config the project has set up. An "Add config" button on each
   * card seeds a new ULID-keyed row via POST; clicking Configure
   * opens an inline FormRenderer pointing at the integration's
   * config-schema URL. Domain-agnostic — the schema tells the
   * renderer what fields to show, where to PUT them, and what the
   * success message reads.
   */
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';
  import ActionButton from '$lib/schema/ActionButton.svelte';
  import AdminPanel from '$lib/schema/AdminPanel.svelte';
  import { tr, type ActionSchema, type Translations } from '$lib/types/form-schema';
  import { confirmAction } from '$lib/stores/confirm';

  interface IntegrationConfig {
    id: string;
    label: string;
    enabled: boolean;
    config_schema_url: string;
    config_url: string;
    enable_url: string;
    remove_url: string;
    actions?: ActionSchema[];
    managed_instance_id?: string;
    managed_instance_label?: string;
  }

  interface IntegrationItem {
    id: string;
    display_name?: Translations;
    description?: Translations;
    icon?: string;
    add_config_url: string;
    configs: IntegrationConfig[];
  }

  interface IntegrationsListResponse {
    integrations: IntegrationItem[];
  }

  let { schemaUrl, lang = 'en' }: { schemaUrl: string; lang?: string } = $props();

  let data = $state<IntegrationsListResponse | null>(null);
  let loading = $state(true);
  let errorMessage = $state('');
  // Track which config row is currently expanded for editing — keyed
  // by `${integrationID}:${configID}` so it can identify across
  // integrations.
  let expandedConfigKey = $state<string | null>(null);
  // Tracks which integration card has the inline "new config" label
  // input open. Keyed by integration ID.
  let addingFor = $state<string | null>(null);
  let newLabel = $state('');
  let refreshNonce = $state(0);
  // Track which config row's Admin panel is open (slide-and-replace).
  // Keyed by `${integID}:${configID}`. Mutually exclusive with edit:
  // entering admin closes the edit form and vice versa.
  let adminConfigKey = $state<string | null>(null);

  function primaryActions(cfg: IntegrationConfig): ActionSchema[] {
    return (cfg.actions ?? []).filter((a) => !a.category);
  }
  function hasAdminActions(cfg: IntegrationConfig): boolean {
    return (cfg.actions ?? []).some((a) => a.category === 'admin');
  }

  async function load() {
    loading = true;
    errorMessage = '';
    try {
      const res = await fetch(schemaUrl);
      if (!res.ok) {
        throw new Error(`Failed to load integrations (HTTP ${res.status})`);
      }
      data = (await res.json()) as IntegrationsListResponse;
    } catch (e: any) {
      errorMessage = e?.message ?? 'Network error';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void schemaUrl;
    void refreshNonce;
    load();
  });

  function configKey(integID: string, configID: string): string {
    return `${integID}:${configID}`;
  }

  function toggle(integID: string, configID: string) {
    const k = configKey(integID, configID);
    expandedConfigKey = expandedConfigKey === k ? null : k;
  }

  async function addConfig(item: IntegrationItem) {
    const label = newLabel.trim();
    if (!label) return;
    try {
      const res = await fetch(item.add_config_url, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ label }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        errorMessage = body?.error || `Failed to add config (HTTP ${res.status})`;
        return;
      }
      const created = (await res.json()) as IntegrationConfig;
      // Open the new row's config form immediately so the operator
      // fills it without an extra click.
      newLabel = '';
      addingFor = null;
      refreshNonce += 1;
      // Defer expanding until after the refresh completes so the new
      // row is in `data`.
      queueMicrotask(() => {
        expandedConfigKey = configKey(item.id, created.id);
      });
    } catch (e: any) {
      errorMessage = e?.message ?? 'Network error';
    }
  }

  async function setEnabled(cfg: IntegrationConfig, enabled: boolean) {
    try {
      const res = await fetch(cfg.enable_url, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        errorMessage = body?.error || `Failed to update (HTTP ${res.status})`;
        return;
      }
      refreshNonce += 1;
    } catch (e: any) {
      errorMessage = e?.message ?? 'Network error';
    }
  }

  async function removeConfig(cfg: IntegrationConfig) {
    const ok = await confirmAction({
      title: `Remove "${cfg.label || cfg.id}"?`,
      message: 'This cannot be undone.',
      confirmLabel: 'Remove',
      danger: true,
    });
    if (!ok) return;
    try {
      const res = await fetch(cfg.remove_url, {
        method: 'DELETE',
        credentials: 'same-origin',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (!res.ok && res.status !== 204) {
        const body = await res.json().catch(() => ({}));
        errorMessage = body?.error || `Failed to remove (HTTP ${res.status})`;
        return;
      }
      refreshNonce += 1;
    } catch (e: any) {
      errorMessage = e?.message ?? 'Network error';
    }
  }
</script>

{#if loading}
  <div class="animate-pulse space-y-3">
    <div class="h-16 bg-gray-200 rounded"></div>
    <div class="h-16 bg-gray-200 rounded"></div>
  </div>
{:else if errorMessage}
  <div class="rounded border border-red-200 bg-red-50 p-3 text-sm text-red-800">
    {errorMessage}
  </div>
{:else if data && data.integrations.length === 0}
  <div class="rounded border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
    No integrations are available on this server. An administrator must enable them in the server build.
  </div>
{:else if data}
  <div class="space-y-4">
    {#each data.integrations as item (item.id)}
      <div class="rounded border border-gray-200 bg-white">
        <div class="flex items-start justify-between gap-4 p-4 border-b border-gray-100">
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-gray-900">{tr(item.display_name, lang) || item.id}</h3>
            {#if item.description}
              <p class="mt-1 text-xs text-gray-600">{tr(item.description, lang)}</p>
            {/if}
          </div>
          <button
            type="button"
            onclick={() => (addingFor = addingFor === item.id ? null : item.id)}
            class="shrink-0 rounded bg-pletka-primary px-3 py-1.5 text-sm font-medium text-white hover:bg-pletka-secondary focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
          >
            {addingFor === item.id ? 'Cancel' : 'Add config'}
          </button>
        </div>

        {#if addingFor === item.id}
          <div class="border-b border-gray-100 bg-gray-50 px-4 py-3 flex items-center gap-2">
            <input
              type="text"
              bind:value={newLabel}
              placeholder="Config label (e.g. production, staging)"
              class="flex-1 rounded border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-pletka-primary"
            />
            <button
              type="button"
              onclick={() => addConfig(item)}
              disabled={!newLabel.trim()}
              class="rounded bg-pletka-primary px-3 py-1.5 text-sm font-medium text-white hover:bg-pletka-secondary disabled:cursor-not-allowed disabled:opacity-50"
            >
              Create
            </button>
          </div>
        {/if}

        {#if item.configs.length === 0}
          <p class="px-4 py-3 text-xs text-gray-500 italic">No configs yet. Click "Add config" to create one.</p>
        {:else}
          <div class="divide-y divide-gray-100">
            {#each item.configs as cfg (cfg.id)}
              <div>
                <div class="flex items-start justify-between gap-3 px-4 py-3">
                  <div class="min-w-0">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="text-sm font-medium text-gray-900">{cfg.label || cfg.id}</span>
                      <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs {cfg.enabled ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-700'}">
                        {cfg.enabled ? 'Enabled' : 'Disabled'}
                      </span>
                      {#if cfg.managed_instance_id}
                        <a
                          href={`/admin/arches/instances/${cfg.managed_instance_id}`}
                          class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-blue-100 text-blue-800 hover:bg-blue-200"
                          title="Managed by the Arches fleet — click to drill down"
                        >
                          managed · {cfg.managed_instance_label || cfg.managed_instance_id.slice(0, 8) + '…'}
                        </a>
                      {/if}
                    </div>
                  </div>
                  <div class="flex shrink-0 items-center gap-2">
                    <button
                      type="button"
                      onclick={() => setEnabled(cfg, !cfg.enabled)}
                      class="rounded border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50"
                    >
                      {cfg.enabled ? 'Disable' : 'Enable'}
                    </button>
                    <button
                      type="button"
                      onclick={() => removeConfig(cfg)}
                      class="rounded border border-red-300 bg-white px-3 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50"
                    >
                      Remove
                    </button>
                    <button
                      type="button"
                      onclick={() => toggle(item.id, cfg.id)}
                      class="rounded bg-pletka-primary px-3 py-1.5 text-xs font-medium text-white hover:bg-pletka-secondary"
                    >
                      {expandedConfigKey === configKey(item.id, cfg.id) ? 'Close' : 'Configure'}
                    </button>
                  </div>
                </div>
                {#if cfg.enabled && cfg.actions && cfg.actions.length > 0}
                  {@const k = configKey(item.id, cfg.id)}
                  {@const adminOpen = adminConfigKey === k}
                  <div class="relative overflow-hidden">
                    <!-- Primary row + admin slide together: two siblings
                         translateX between -100% and 0 on a single
                         transition so they read as a swap. -->
                    <div
                      class="grid transition-transform duration-300 ease-in-out"
                      style="grid-template-columns: 50% 50%; width: 200%; transform: translateX({adminOpen ? '-50%' : '0%'});"
                    >
                      <div class="border-t border-gray-100 bg-gray-50 px-4 py-3">
                        <div class="flex flex-wrap items-start justify-between gap-3">
                          <div class="flex flex-col gap-3 min-w-0">
                            {#each primaryActions(cfg) as action (action.id)}
                              <ActionButton {action} {lang} />
                            {/each}
                          </div>
                          {#if hasAdminActions(cfg)}
                            <button
                              type="button"
                              onclick={() => (adminConfigKey = k)}
                              class="shrink-0 inline-flex items-center gap-1 rounded border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50"
                            >
                              Admin <span aria-hidden="true">▸</span>
                            </button>
                          {/if}
                        </div>
                      </div>
                      <div class="border-t border-gray-100 bg-gray-50">
                        {#if adminOpen}
                          <AdminPanel
                            actions={cfg.actions ?? []}
                            {lang}
                            onBack={() => (adminConfigKey = null)}
                          />
                        {/if}
                      </div>
                    </div>
                  </div>
                {/if}

                {#if expandedConfigKey === configKey(item.id, cfg.id)}
                  <div class="border-t border-gray-100 bg-gray-50 px-4 py-3">
                    {#key `${cfg.id}-${refreshNonce}`}
                      <FormRenderer
                        schemaUrl={cfg.config_schema_url}
                        onSuccessAction="none"
                        onsuccess={() => {
                          expandedConfigKey = null;
                          refreshNonce += 1;
                        }}
                        oncancel={() => {
                          expandedConfigKey = null;
                        }}
                      />
                    {/key}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  </div>
{/if}
