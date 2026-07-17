<script lang="ts">
  /**
   * Slide-in admin panel for an integration config row. Composes:
   *   - Status (auto-fired panel action on mount)
   *   - Audit (auto-fired panel action on mount)
   *   - Operations row (inline-refresh + danger actions: reset, publish)
   *
   * Refresh contract:
   *   - All panel actions (result === 'panel') are fired in parallel
   *     when this component mounts AND whenever Refresh is clicked.
   *   - Inline-refresh actions (result === 'inline-refresh') run via
   *     the same ActionButton path then trigger refreshPanels().
   *
   * No hard-coded action IDs — the panel iterates whatever the schema
   * says is Category="admin", so the same component handles future
   * admin actions (export, reindex, restart) without changes.
   */
  import type { ActionSchema, ActionResultUI } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import { confirmAction } from '$lib/stores/confirm';
  import PanelRenderer from './PanelRenderer.svelte';

  let {
    actions,
    lang = 'en',
    onBack,
  }: {
    actions: ActionSchema[];
    lang?: string;
    onBack: () => void;
  } = $props();

  const adminActions = $derived(actions.filter((a) => a.category === 'admin'));
  const panelActions = $derived(adminActions.filter((a) => a.result === 'panel'));
  const opActions = $derived(adminActions.filter((a) => a.result === 'inline-refresh'));

  // Resolve per-row Restore URLs for the Backups panel. The backups
  // action emits {kind: pg_backup|pack} per entry; we map each kind to
  // the matching restore action's endpoint URL (if available). When
  // either action isn't in the schema (e.g. instance-scoped admin
  // surface that ships only a subset), the corresponding Restore
  // button just doesn't render.
  // Build a flat action_id -> endpoint URL map so PanelRenderer can
  // resolve per-row actions backend rows declare (entry.actions[]).
  // Indexed by both the raw id ("pg_restore") AND its suffix when
  // the id is fleet-prefixed ("arches.fleet.<inst>.pg_restore") so
  // a backend-emitted action_id of "pg_restore" works against both
  // project-scoped + instance-scoped admin surfaces.
  const actionURLs = $derived((() => {
    const m: Record<string, string> = {};
    for (const a of adminActions) {
      if (!a.endpoint?.url) continue;
      m[a.id] = a.endpoint.url;
      const dot = a.id.lastIndexOf('.');
      if (dot >= 0) m[a.id.slice(dot + 1)] = a.endpoint.url;
    }
    return m;
  })());

  // Result keyed by action id; null while loading; ActionResultUI on
  // success/error; string for transport-level errors.
  let panelResults = $state<Record<string, ActionResultUI | null>>({});
  let panelErrors = $state<Record<string, string>>({});
  let opBusy = $state<Record<string, boolean>>({});
  let opResults = $state<Record<string, ActionResultUI | null>>({});
  let opErrors = $state<Record<string, string>>({});
  // Per-action elapsed-time ticker. Pure UX polish: long actions
  // (push_models, restore_pack, reindex) take 10-30s + the operator
  // would otherwise stare at a frozen spinner. Increment every 500ms
  // while the action runs; reset on completion.
  let opElapsed = $state<Record<string, number>>({});
  const opTickers: Record<string, ReturnType<typeof setInterval>> = {};
  let refreshingAll = $state(false);

  async function fireAction(a: ActionSchema): Promise<ActionResultUI | string> {
    try {
      const res = await fetch(a.endpoint.url, {
        method: a.endpoint.method || 'GET',
        credentials: 'same-origin',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        return (body as any)?.error || `HTTP ${res.status}`;
      }
      return body as ActionResultUI;
    } catch (e: any) {
      return e?.message ?? 'Network error';
    }
  }

  async function refreshPanels() {
    // Render each panel as soon as its own fetch resolves rather than
    // blocking on the slowest. graph_counts / status can take seconds
    // on populated DBs; cheaper panels (healthcheck, audit_tail) want
    // to paint in <300ms. refreshingAll stays true until every panel
    // has reported, so the "Refresh all" button doesn't re-fire under
    // a still-in-flight wave.
    refreshingAll = true;
    panelResults = Object.fromEntries(panelActions.map((a) => [a.id, null]));
    panelErrors = {};
    let remaining = panelActions.length;
    if (remaining === 0) {
      refreshingAll = false;
      return;
    }
    panelActions.forEach((a) => {
      void fireAction(a).then((r) => {
        if (typeof r === 'string') {
          panelErrors[a.id] = r;
        } else {
          panelResults[a.id] = r;
        }
        remaining -= 1;
        if (remaining === 0) refreshingAll = false;
      });
    });
  }

  async function runOp(a: ActionSchema) {
    if (opBusy[a.id]) return;
    if (a.endpoint.confirm) {
      const c = a.endpoint.confirm;
      const ok = await confirmAction({
        title: tr(c.title, lang),
        message: tr(c.message, lang),
        confirmLabel: tr(c.confirm_label, lang),
        danger: a.theme === 'danger',
      });
      if (!ok) return;
    }
    opBusy[a.id] = true;
    opResults[a.id] = null;
    opErrors[a.id] = '';
    opElapsed[a.id] = 0;
    const start = performance.now();
    opTickers[a.id] = setInterval(() => {
      opElapsed[a.id] = Math.floor((performance.now() - start) / 1000);
    }, 500);
    const r = await fireAction(a);
    opBusy[a.id] = false;
    clearInterval(opTickers[a.id]);
    delete opTickers[a.id];
    if (typeof r === 'string') {
      opErrors[a.id] = r;
    } else {
      opResults[a.id] = r;
      // Refresh status + audit so the panel reflects the new state.
      refreshPanels();
    }
  }

  $effect(() => {
    void actions; // re-fire if schema regenerates
    refreshPanels();
  });
</script>

<div class="bg-white">
  <div class="flex items-center justify-between border-b border-gray-200 px-4 py-3">
    <button
      type="button"
      onclick={onBack}
      class="inline-flex items-center gap-1 rounded border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50"
    >
      <span aria-hidden="true">←</span> Back
    </button>
    <h3 class="text-sm font-semibold text-gray-900">Admin</h3>
    <button
      type="button"
      onclick={refreshPanels}
      disabled={refreshingAll}
      class="inline-flex items-center gap-1 rounded bg-pletka-primary px-3 py-1.5 text-sm font-medium text-white hover:bg-pletka-secondary disabled:opacity-50"
    >
      {#if refreshingAll}
        <span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
      {/if}
      Refresh
    </button>
  </div>

  <div class="space-y-4 p-4">
    {#each panelActions as a (a.id)}
      <div>
        {#if panelResults[a.id] === null}
          <div class="rounded border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-500">
            Loading {tr(a.label, lang).toLowerCase()}…
          </div>
        {:else if panelErrors[a.id]}
          <div class="rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900">
            <span class="font-semibold">{tr(a.label, lang)}:</span>
            {panelErrors[a.id]}
          </div>
        {:else if panelResults[a.id]?.panel}
          <PanelRenderer
            panel={panelResults[a.id]!.panel!}
            {actionURLs}
            onActionDone={refreshPanels}
          />
        {:else if panelResults[a.id]}
          <div class="rounded border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-700">
            {tr(panelResults[a.id]!.message, lang)}
          </div>
        {/if}
      </div>
    {/each}

    {#if opActions.length > 0}
      <div class="rounded border border-gray-200 bg-white">
        <div class="border-b border-gray-100 px-4 py-2">
          <h4 class="text-sm font-semibold text-gray-900">Operations</h4>
        </div>
        <div class="flex flex-wrap items-start gap-2 p-4">
          {#each opActions as a (a.id)}
            <button
              type="button"
              onclick={() => runOp(a)}
              disabled={opBusy[a.id]}
              title={a.help ? tr(a.help, lang) : undefined}
              class="inline-flex items-center gap-2 rounded px-3 py-1.5 text-sm font-medium text-white shadow-sm focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 {a.theme ===
              'danger'
                ? 'bg-red-600 hover:bg-red-700 focus:ring-red-600'
                : 'bg-pletka-primary hover:bg-pletka-secondary focus:ring-pletka-primary'}"
            >
              {#if opBusy[a.id]}
                <span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
              {/if}
              {tr(a.label, lang)}
              {#if opBusy[a.id] && opElapsed[a.id] > 0}
                <span class="text-xs opacity-80 font-mono">({opElapsed[a.id]}s)</span>
              {/if}
            </button>
          {/each}
        </div>
        <!-- Per-action result banners (success/error). Sit below the row
             so each banner can be full-width, dismissable, and visually
             match the green/red feedback the existing ActionButton uses
             on the primary row. -->
        {#each opActions as a (a.id)}
          {#if opErrors[a.id]}
            <div class="border-t border-red-200 bg-red-50 px-4 py-2 text-sm text-red-900 flex items-start justify-between gap-3">
              <div class="min-w-0">
                <span class="font-semibold">{tr(a.label, lang)}:</span>
                <span class="ml-1">{opErrors[a.id]}</span>
              </div>
              <button
                type="button"
                aria-label="Dismiss"
                onclick={() => (opErrors[a.id] = '')}
                class="shrink-0 text-red-700 hover:text-red-900 font-bold leading-none px-1"
              >×</button>
            </div>
          {:else if opResults[a.id]?.status === 'success'}
            <div class="border-t border-green-200 bg-green-50 px-4 py-2 text-sm text-green-900 flex items-start justify-between gap-3">
              <div class="min-w-0">
                <span class="font-semibold">{tr(a.label, lang)}:</span>
                <span class="ml-1">{tr(opResults[a.id]!.message, lang)}</span>
                {#if opResults[a.id]?.link_url}
                  <a
                    href={opResults[a.id]!.link_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="ml-2 underline hover:text-green-700"
                  >{opResults[a.id]?.link_label ? tr(opResults[a.id]!.link_label!, lang) : 'open'}</a>
                {/if}
              </div>
              <button
                type="button"
                aria-label="Dismiss"
                onclick={() => (opResults[a.id] = null)}
                class="shrink-0 text-green-700 hover:text-green-900 font-bold leading-none px-1"
              >×</button>
            </div>
          {:else if opResults[a.id]?.status === 'error'}
            <div class="border-t border-red-200 bg-red-50 px-4 py-2 text-sm text-red-900 flex items-start justify-between gap-3">
              <div class="min-w-0">
                <span class="font-semibold">{tr(a.label, lang)}:</span>
                <span class="ml-1">{tr(opResults[a.id]!.message, lang)}</span>
              </div>
              <button
                type="button"
                aria-label="Dismiss"
                onclick={() => (opResults[a.id] = null)}
                class="shrink-0 text-red-700 hover:text-red-900 font-bold leading-none px-1"
              >×</button>
            </div>
          {/if}
        {/each}
      </div>
    {/if}
  </div>
</div>
