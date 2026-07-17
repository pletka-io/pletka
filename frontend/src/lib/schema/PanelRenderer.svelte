<script lang="ts">
  /**
   * Generic schema-driven panel renderer. ActionResultUI.panel ships
   * { kind, data } where kind picks a layout and data shape is
   * documented per kind in the backend that emits it. Add a new kind
   * here as integrations introduce new card shapes — frontend stays
   * dispatch-only, no per-integration logic in this file.
   *
   * Known kinds:
   *   status         { reachable, ontologies, graphs, published_graphs,
   *                    ontology_classes, resources, oauth_applications,
   *                    oauth_access_tokens_live, ... }  (archessql)
   *   audit_tail     { records: [{ ts, application_name, project,
   *                    status, ontology_rows, graph_rows,
   *                    duration_ms, bytes_in }] }       (archessql)
   *   wipe_result    { policy, counts: {...}, duration_ms }
   *   publish_result { models: string[], duration_ms }
   */

  let {
    panel,
    actionURLs,
    onActionDone,
  }: {
    panel: { kind: string; data: any } | undefined;
    /** action_id -> POST URL map. Backend rows declare which action
     *  applies to them (entry.actions[]); frontend resolves the URL
     *  here. Pass from AdminPanel which already holds ActionSchema. */
    actionURLs?: Record<string, string>;
    /** Fired after a row-level action succeeds. Parents typically
     *  rebind to refresh sibling panels (status, audit) so the
     *  effect is visible without a page reload. */
    onActionDone?: () => void;
  } = $props();

  let runningKey = $state<string>('');
  let rowError = $state<string>('');
  let rowSuccess = $state<string>('');

  // Generic per-row action invoker. Looks up action_id in actionURLs,
  // appends entry-supplied params as query string, POSTs. Backend
  // owns the decision of "which action goes on which row"; we just
  // dispatch. New per-row ops (verify, delete, view manifest) plug
  // in without frontend changes.
  async function runEntryAction(entryName: string, action: { action_id: string; label: string; params?: Record<string, string> }) {
    const url = actionURLs?.[action.action_id];
    if (!url) {
      rowError = `No URL registered for action "${action.action_id}"`;
      return;
    }
    if (!confirm(`${action.label} "${entryName}"?\n\nThis runs against the live instance. Active sessions may break.`)) return;
    const key = entryName + ':' + action.action_id;
    runningKey = key;
    rowError = '';
    rowSuccess = '';
    try {
      const qs = new URLSearchParams(action.params ?? {}).toString();
      const sep = url.includes('?') ? '&' : '?';
      const target = qs ? `${url}${sep}${qs}` : url;
      const res = await fetch(target, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        rowError = (body as any)?.error || `HTTP ${res.status}`;
        return;
      }
      rowSuccess = (body as any)?.message?.en ?? `${action.label} ${entryName} OK`;
      onActionDone?.();
    } catch (e: any) {
      rowError = e?.message ?? 'Network error';
    } finally {
      runningKey = '';
    }
  }

  function num(v: any): string {
    if (typeof v !== 'number') return '—';
    return v.toLocaleString();
  }

  function ts(v: any): string {
    if (typeof v !== 'string') return '';
    const d = new Date(v);
    if (isNaN(d.getTime())) return v;
    return d.toISOString().replace('T', ' ').slice(0, 19);
  }
</script>

{#if !panel}
  {''}
{:else if panel.kind === 'status'}
  {@const s = panel.data?.db ?? {}}
  <div class="rounded border border-gray-200 bg-white">
    <div class="border-b border-gray-100 px-4 py-2 flex items-center justify-between">
      <h4 class="text-sm font-semibold text-gray-900">Status</h4>
      <span
        class="inline-flex items-center px-2 py-0.5 rounded-full text-xs {s.reachable
          ? 'bg-green-100 text-green-800'
          : 'bg-red-100 text-red-800'}"
      >
        {s.reachable ? 'DB reachable' : 'DB unreachable'}
      </span>
    </div>
    <div class="grid grid-cols-2 md:grid-cols-4 gap-3 p-4 text-sm">
      <div>
        <div class="text-xs uppercase text-gray-500">Graphs</div>
        <div class="text-lg font-mono">{num(s.graphs)}</div>
      </div>
      <div>
        <div class="text-xs uppercase text-gray-500">Published</div>
        <div class="text-lg font-mono">{num(s.published_graphs)}</div>
      </div>
      <div>
        <div class="text-xs uppercase text-gray-500">Ontologies</div>
        <div class="text-lg font-mono">{num(s.ontologies)}</div>
      </div>
      <div>
        <div class="text-xs uppercase text-gray-500">Classes</div>
        <div class="text-lg font-mono">{num(s.ontology_classes)}</div>
      </div>
      <div>
        <div class="text-xs uppercase text-gray-500">Resources</div>
        <div class="text-lg font-mono">{num(s.resources)}</div>
      </div>
      <div>
        <div class="text-xs uppercase text-gray-500">OAuth apps</div>
        <div class="text-lg font-mono">{num(s.oauth_applications)}</div>
      </div>
      <div>
        <div class="text-xs uppercase text-gray-500">Live tokens</div>
        <div class="text-lg font-mono">{num(s.oauth_access_tokens_live)}</div>
      </div>
    </div>
  </div>
{:else if panel.kind === 'audit_tail'}
  {@const records = (panel.data?.records ?? []) as any[]}
  <div class="rounded border border-gray-200 bg-white">
    <div class="border-b border-gray-100 px-4 py-2 flex items-center justify-between">
      <h4 class="text-sm font-semibold text-gray-900">Audit (last {records.length})</h4>
      {#if panel.data?.path}
        <span class="text-xs text-gray-500 truncate" title={panel.data.path}>
          {panel.data.path}
        </span>
      {/if}
    </div>
    {#if records.length === 0}
      <p class="px-4 py-3 text-xs text-gray-500 italic">No audit records yet.</p>
    {:else}
      <div class="overflow-x-auto">
        <table class="min-w-full text-xs">
          <thead class="bg-gray-50">
            <tr class="text-left text-gray-600">
              <th class="px-3 py-2 font-semibold">Time</th>
              <th class="px-3 py-2 font-semibold">Application</th>
              <th class="px-3 py-2 font-semibold">Project</th>
              <th class="px-3 py-2 font-semibold">Status</th>
              <th class="px-3 py-2 font-semibold text-right">Ont</th>
              <th class="px-3 py-2 font-semibold text-right">Graph</th>
              <th class="px-3 py-2 font-semibold text-right">ms</th>
              <th class="px-3 py-2 font-semibold text-right">Bytes</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100">
            {#each records as r}
              <tr>
                <td class="px-3 py-1.5 font-mono">{ts(r?.ts)}</td>
                <td class="px-3 py-1.5">{r?.application_name ?? '—'}</td>
                <td class="px-3 py-1.5">{r?.project ?? '—'}</td>
                <td class="px-3 py-1.5">
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded text-xs font-mono {r?.status >= 200 &&
                    r?.status < 300
                      ? 'bg-green-100 text-green-800'
                      : 'bg-red-100 text-red-800'}"
                  >
                    {r?.status ?? '?'}
                  </span>
                </td>
                <td class="px-3 py-1.5 font-mono text-right">{num(r?.ontology_rows)}</td>
                <td class="px-3 py-1.5 font-mono text-right">{num(r?.graph_rows)}</td>
                <td class="px-3 py-1.5 font-mono text-right">{num(r?.duration_ms)}</td>
                <td class="px-3 py-1.5 font-mono text-right">{num(r?.bytes_in)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
{:else if panel.kind === 'wipe_result'}
  {@const w = panel.data}
  <div class="rounded border border-amber-200 bg-amber-50 p-4 text-sm">
    <h4 class="font-semibold text-amber-900 mb-2">Wipe applied: {w?.policy ?? '?'}</h4>
    <div class="grid grid-cols-2 md:grid-cols-4 gap-2 text-xs">
      {#each Object.entries((w?.counts ?? {}) as Record<string, number>) as [k, v]}
        <div class="bg-white rounded px-2 py-1 border border-amber-200">
          <div class="text-gray-500">{k}</div>
          <div class="font-mono text-amber-900">{num(v)}</div>
        </div>
      {/each}
    </div>
    {#if w?.duration_ms != null}
      <p class="mt-2 text-xs text-amber-800">in {num(w.duration_ms)} ms</p>
    {/if}
  </div>
{:else if panel.kind === 'healthcheck'}
  {@const comps = (panel.data?.components ?? {}) as Record<string, { ok: boolean; detail?: string; latency_ms?: number }>}
  <div class="rounded border border-gray-200 bg-white">
    <div class="border-b border-gray-100 px-4 py-2 flex items-center justify-between">
      <h4 class="text-sm font-semibold text-gray-900">Healthcheck</h4>
      <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs {panel.data?.ok ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'}">
        {panel.data?.ok ? 'All OK' : 'Degraded'}
      </span>
    </div>
    <div class="grid grid-cols-1 md:grid-cols-3 gap-2 p-4 text-sm">
      {#each Object.entries(comps) as [name, c]}
        <div class="border border-gray-100 rounded px-3 py-2">
          <div class="flex items-center justify-between">
            <span class="font-medium text-gray-900">{name}</span>
            <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs {c.ok ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'}">
              {c.ok ? 'ok' : 'down'}
            </span>
          </div>
          <div class="text-xs text-gray-500 font-mono">{c.latency_ms ?? 0}ms</div>
          {#if c.detail}
            <div class="text-xs text-gray-700 mt-1">{c.detail}</div>
          {/if}
        </div>
      {/each}
    </div>
  </div>
{:else if panel.kind === 'graph_counts'}
  {@const rows = (panel.data?.rows ?? []) as any[]}
  <div class="rounded border border-gray-200 bg-white">
    <div class="border-b border-gray-100 px-4 py-2">
      <h4 class="text-sm font-semibold text-gray-900">Graph counts ({panel.data?.total ?? 0})</h4>
    </div>
    <div class="overflow-x-auto">
      <table class="min-w-full text-xs">
        <thead class="bg-gray-50">
          <tr class="text-left text-gray-600">
            <th class="px-3 py-2 font-semibold">Name</th>
            <th class="px-3 py-2 font-semibold">Kind</th>
            <th class="px-3 py-2 font-semibold text-right">Nodes</th>
            <th class="px-3 py-2 font-semibold text-right">Edges</th>
            <th class="px-3 py-2 font-semibold text-right">Cards</th>
            <th class="px-3 py-2 font-semibold text-right">Resources</th>
            <th class="px-3 py-2 font-semibold">Published</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100">
          {#each rows as r}
            <tr>
              <td class="px-3 py-1.5">{r.name?.replace(/[{}"]/g, '').replace(/en:/, '') || r.graphid?.slice(0, 8)}</td>
              <td class="px-3 py-1.5 text-gray-600">{r.isresource ? 'resource' : 'branch'}</td>
              <td class="px-3 py-1.5 font-mono text-right">{num(r.nodes)}</td>
              <td class="px-3 py-1.5 font-mono text-right">{num(r.edges)}</td>
              <td class="px-3 py-1.5 font-mono text-right">{num(r.cards)}</td>
              <td class="px-3 py-1.5 font-mono text-right">{num(r.resources)}</td>
              <td class="px-3 py-1.5">
                <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs {r.has_publication ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-700'}">
                  {r.has_publication ? `pub (${num(r.published_graphs)})` : '—'}
                </span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{:else if panel.kind === 'reindex_result'}
  {@const r = panel.data}
  <div class="rounded border border-blue-200 bg-blue-50 p-4 text-sm">
    <h4 class="font-semibold text-blue-900 mb-2">Reindex complete: {r?.prefix} ({(r?.targets ?? []).join(', ')}) in {num(r?.duration_ms)} ms</h4>
    {#if r?.stdout_tail}
      <pre class="text-xs text-blue-900 bg-white border border-blue-200 rounded p-2 overflow-auto max-h-40 whitespace-pre-wrap">{r.stdout_tail}</pre>
    {/if}
  </div>
{:else if panel.kind === 'backups_list'}
  {@const entries = (panel.data?.entries ?? []) as any[]}
  {#if rowError}
    <div class="rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800 mb-2">{rowError}</div>
  {/if}
  {#if rowSuccess}
    <div class="rounded border border-green-200 bg-green-50 px-3 py-2 text-xs text-green-800 mb-2">{rowSuccess}</div>
  {/if}
  <div class="rounded border border-gray-200 bg-white">
    <div class="border-b border-gray-100 px-4 py-2 flex items-center justify-between">
      <h4 class="text-sm font-semibold text-gray-900">Backups ({panel.data?.total ?? 0})</h4>
      {#if panel.data?.dir}
        <span class="text-xs text-gray-500 truncate font-mono" title={panel.data.dir}>{panel.data.dir}</span>
      {/if}
    </div>
    {#if entries.length === 0}
      <p class="px-4 py-3 text-xs text-gray-500 italic">No backups yet. Run "Backup Postgres" or "Export pack".</p>
    {:else}
      <div class="overflow-x-auto">
        <table class="min-w-full text-xs">
          <thead class="bg-gray-50 text-left text-gray-600">
            <tr>
              <th class="px-3 py-2 font-semibold">Name</th>
              <th class="px-3 py-2 font-semibold">Kind</th>
              <th class="px-3 py-2 font-semibold text-right">Size</th>
              <th class="px-3 py-2 font-semibold">Modified</th>
              <th class="px-3 py-2 font-semibold"></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100">
            {#each entries as e}
              <tr>
                <td class="px-3 py-1.5 font-mono">{e.name}</td>
                <td class="px-3 py-1.5">
                  <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs {e.kind === 'pg_backup' ? 'bg-blue-100 text-blue-800' : e.kind === 'pack' ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-700'}">{e.kind}</span>
                </td>
                <td class="px-3 py-1.5 font-mono text-right">{num(e.bytes)}</td>
                <td class="px-3 py-1.5 font-mono">{ts(e.mtime)}</td>
                <td class="px-3 py-1.5 text-right whitespace-nowrap">
                  {#if e.download_url}
                    <a
                      href={e.download_url}
                      class="inline-flex items-center px-2 py-0.5 rounded text-xs bg-pletka-primary text-white hover:bg-pletka-secondary mr-1"
                      target="_blank"
                      rel="noopener noreferrer"
                      download={e.name}
                    >Download</a>
                  {/if}
                  {#each (e.actions ?? []) as a}
                    {#if actionURLs?.[a.action_id]}
                      <button
                        type="button"
                        onclick={() => runEntryAction(e.name, a)}
                        disabled={runningKey !== ''}
                        class="inline-flex items-center px-2 py-0.5 rounded text-xs border border-red-500 text-red-700 hover:bg-red-50 disabled:opacity-50 mr-1"
                      >
                        {runningKey === e.name + ':' + a.action_id ? a.label + '…' : a.label}
                      </button>
                    {/if}
                  {/each}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
{:else if panel.kind === 'install_result'}
  {@const r = panel.data}
  <div class="rounded border border-green-200 bg-green-50 p-4 text-sm">
    <h4 class="font-semibold text-green-900">Installed {num(r?.installed)} SQL file(s) in profile {r?.profile} ({num(r?.duration_ms)} ms)</h4>
  </div>
{:else if panel.kind === 'publish_result'}
  {@const p = panel.data}
  <div class="rounded border border-green-200 bg-green-50 p-4 text-sm">
    <h4 class="font-semibold text-green-900 mb-2">
      Published {num(p?.models?.length ?? 0)} graph(s) in {num(p?.duration_ms)} ms
    </h4>
    {#if p?.models?.length}
      <ul class="list-disc list-inside text-xs text-green-900 space-y-0.5 max-h-40 overflow-y-auto">
        {#each p.models as m}
          <li>{m}</li>
        {/each}
      </ul>
    {/if}
  </div>
{:else}
  <pre class="rounded border border-gray-200 bg-gray-50 p-3 text-xs overflow-auto">{JSON.stringify(panel, null, 2)}</pre>
{/if}
