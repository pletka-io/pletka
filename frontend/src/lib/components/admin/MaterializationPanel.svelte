<!-- frontend/src/lib/components/admin/MaterializationPanel.svelte -->
<!--
  Super-admin viewer for git materialization runs + performance. Reads the
  read-only /admin/materialization endpoint (materializationadmin slice), which
  reports the materialized_* columns the gitmaterializer records on
  weave_change_set. No mutations — purely observational.
-->
<script lang="ts">
  type Summary = {
    window_hours: number;
    runs: number;
    committed: number;
    noop: number;
    failed: number;
    p50_ms?: number;
    p95_ms?: number;
    max_ms?: number;
    queue_depth: number;
    lag_seconds: number;
  };
  type Run = {
    id: number;
    project_id: string;
    actor_name: string;
    started_at: string;
    processed_at?: string;
    outcome?: string; // committed | noop | failed | undefined (pending)
    duration_ms?: number;
    files?: number;
    changed?: number;
    commit_sha?: string;
    error?: string;
  };

  let loading = $state(true);
  let error = $state<string | null>(null);
  let summary = $state<Summary | null>(null);
  let runs = $state<Run[]>([]);
  let windowHours = $state(168);

  async function load() {
    loading = true;
    error = null;
    try {
      const res = await fetch(`/admin/materialization?limit=100&window_hours=${windowHours}`);
      if (!res.ok) throw new Error(`Failed to load: ${res.status}`);
      const body = await res.json();
      summary = body.summary ?? null;
      runs = body.runs ?? [];
    } catch (e: any) {
      error = e?.message || String(e);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void windowHours; // reload when the window changes
    void load();
  });

  function ms(v?: number): string {
    if (v === undefined || v === null) return '—';
    if (v < 1000) return `${v} ms`;
    return `${(v / 1000).toFixed(2)} s`;
  }
  function outcomeClass(o?: string): string {
    switch (o) {
      case 'committed': return 'bg-green-50 text-green-700';
      case 'noop': return 'bg-gray-100 text-gray-600';
      case 'failed': return 'bg-red-50 text-red-700';
      default: return 'bg-amber-50 text-amber-700'; // pending
    }
  }
  function outcomeLabel(o?: string): string {
    return o ?? 'pending';
  }
  function shortSha(sha?: string): string {
    return sha ? sha.slice(0, 8) : '—';
  }
  function when(ts?: string): string {
    if (!ts) return '—';
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
  }
  function lag(seconds: number): string {
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
    return `${Math.floor(seconds / 3600)}h`;
  }
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <div>
      <h2 class="text-xl font-semibold text-gray-900">Git Materialization</h2>
      <p class="mt-1 text-sm text-gray-500">
        Async save-to-git runs recorded on each change set, with performance over the selected window.
      </p>
    </div>
    <div class="flex items-center gap-2">
      <select
        bind:value={windowHours}
        class="rounded-md border border-gray-300 px-2 py-1.5 text-sm"
        aria-label="Window"
      >
        <option value={24}>Last 24h</option>
        <option value={168}>Last 7 days</option>
        <option value={720}>Last 30 days</option>
      </select>
      <button
        type="button"
        onclick={load}
        class="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50"
      >
        Refresh
      </button>
    </div>
  </div>

  {#if error}
    <div class="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700">{error}</div>
  {/if}

  {#if summary}
    <!-- Live gauges (window-independent) -->
    <div class="grid grid-cols-2 gap-4 sm:grid-cols-5">
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <p class="text-xs uppercase tracking-wide text-gray-500">Queue depth</p>
        <p class="mt-1 text-2xl font-semibold {summary.queue_depth > 0 ? 'text-amber-700' : 'text-gray-900'}">{summary.queue_depth}</p>
        <p class="text-xs text-gray-400">unmaterialized sets</p>
      </div>
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <p class="text-xs uppercase tracking-wide text-gray-500">Oldest lag</p>
        <p class="mt-1 text-2xl font-semibold {summary.lag_seconds > 60 ? 'text-amber-700' : 'text-gray-900'}">{lag(summary.lag_seconds)}</p>
        <p class="text-xs text-gray-400">oldest pending</p>
      </div>
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <p class="text-xs uppercase tracking-wide text-gray-500">p50 / p95</p>
        <p class="mt-1 text-2xl font-semibold text-gray-900">{ms(summary.p50_ms)}</p>
        <p class="text-xs text-gray-400">p95 {ms(summary.p95_ms)} · max {ms(summary.max_ms)}</p>
      </div>
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <p class="text-xs uppercase tracking-wide text-gray-500">Runs</p>
        <p class="mt-1 text-2xl font-semibold text-gray-900">{summary.runs}</p>
        <p class="text-xs text-gray-400">in window</p>
      </div>
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <p class="text-xs uppercase tracking-wide text-gray-500">Outcomes</p>
        <p class="mt-1 text-sm">
          <span class="font-semibold text-green-700">{summary.committed}</span> committed ·
          <span class="text-gray-600">{summary.noop}</span> noop ·
          <span class="font-semibold {summary.failed > 0 ? 'text-red-700' : 'text-gray-600'}">{summary.failed}</span> failed
        </p>
      </div>
    </div>
  {/if}

  <!-- Recent runs -->
  <div class="overflow-x-auto rounded-lg border border-gray-200">
    <table class="min-w-full divide-y divide-gray-200 text-sm">
      <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
        <tr>
          <th class="px-3 py-2">Change set</th>
          <th class="px-3 py-2">Project</th>
          <th class="px-3 py-2">Actor</th>
          <th class="px-3 py-2">Outcome</th>
          <th class="px-3 py-2">Duration</th>
          <th class="px-3 py-2">Files / changed</th>
          <th class="px-3 py-2">Commit</th>
          <th class="px-3 py-2">Processed</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 bg-white">
        {#if loading && runs.length === 0}
          <tr><td colspan="8" class="px-3 py-6 text-center text-gray-400">Loading…</td></tr>
        {:else if runs.length === 0}
          <tr><td colspan="8" class="px-3 py-6 text-center text-gray-400">No materialization runs recorded.</td></tr>
        {:else}
          {#each runs as r (r.id)}
            <tr class="hover:bg-gray-50">
              <td class="px-3 py-2 font-mono text-xs text-gray-500">#{r.id}</td>
              <td class="px-3 py-2 font-mono text-xs">{r.project_id}</td>
              <td class="px-3 py-2 text-gray-700">{r.actor_name || '—'}</td>
              <td class="px-3 py-2">
                <span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium {outcomeClass(r.outcome)}">
                  {outcomeLabel(r.outcome)}
                </span>
              </td>
              <td class="px-3 py-2 font-mono text-xs {r.duration_ms && r.duration_ms > 1000 ? 'text-amber-700' : 'text-gray-700'}">{ms(r.duration_ms)}</td>
              <td class="px-3 py-2 font-mono text-xs text-gray-500">
                {r.files ?? '—'}{r.changed !== undefined ? ` / ${r.changed}` : ''}
              </td>
              <td class="px-3 py-2 font-mono text-xs text-gray-500" title={r.commit_sha ?? ''}>{shortSha(r.commit_sha)}</td>
              <td class="px-3 py-2 text-xs text-gray-500" title={r.error ?? ''}>{when(r.processed_at)}</td>
            </tr>
          {/each}
        {/if}
      </tbody>
    </table>
  </div>
</div>
