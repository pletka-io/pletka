<script lang="ts">
  type RestorePreview = {
    snapshot_path: string;
    project_id: string;
    module_path?: string;
    has_lockfile: boolean;
    self_contained: boolean;
    parent_dependencies?: RestoreParentDependency[];
    ontology_dependencies?: RestoreOntologyDependency[];
    counts: RestoreCounts;
    operation_counts?: RestoreOperationCount[];
  };

  type RestoreParentDependency = {
    project_id: string;
    is_primary: boolean;
    canonical_order: number;
    source_mode: string;
    source_version?: string;
    module?: string;
  };

  type RestoreOntologyDependency = {
    ontology_id: string;
    ontology_version_id?: string;
    version?: string;
    module?: string;
  };

  type RestoreCounts = {
    categories: number;
    fields: number;
    models: number;
    collections: number;
    base_overrides: number;
    model_overrides: number;
    collection_overrides: number;
    adoption_receipts: number;
    fork_receipts: number;
    vendored_projects: number;
    vendored_ontologies: number;
    operations: number;
  };

  type RestoreOperationCount = {
    kind: string;
    count: number;
  };

  type RestoreJob = {
    id: string;
    snapshot_path: string;
    source_project_id: string;
    target_project_id: string;
    requested_by_id: string;
    status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
    current_phase: string;
    error_message?: string;
    preview?: RestorePreview;
    created_at: string;
    started_at?: string;
    finished_at?: string;
  };

  let snapshotPath = $state('');
  let targetProjectId = $state('');
  let preview = $state<RestorePreview | null>(null);
  let jobs = $state<RestoreJob[]>([]);
  let selectedJob = $state<RestoreJob | null>(null);
  let loadingPreview = $state(false);
  let loadingJobs = $state(false);
  let creatingJob = $state(false);
  let runningJobId = $state('');
  let errorMessage = $state('');
  let successMessage = $state('');

  const entityCountRows = $derived(
    preview
      ? [
          ['Categories', preview.counts.categories],
          ['Fields', preview.counts.fields],
          ['Models', preview.counts.models],
          ['Collections', preview.counts.collections],
          ['Overrides', preview.counts.base_overrides + preview.counts.model_overrides + preview.counts.collection_overrides],
          ['Adoptions', preview.counts.adoption_receipts],
          ['Forks', preview.counts.fork_receipts],
          ['Operations', preview.counts.operations],
        ]
      : []
  );

  $effect(() => {
    loadJobs();
  });

  async function previewSnapshot() {
    loadingPreview = true;
    errorMessage = '';
    successMessage = '';
    preview = null;
    try {
      const res = await fetch('/admin/git-restore/preview', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
        body: JSON.stringify({ snapshot_path: snapshotPath }),
      });
      if (!res.ok) throw new Error(await responseError(res));
      preview = await res.json();
      if (!targetProjectId && preview?.project_id) {
        targetProjectId = preview.project_id;
      }
      successMessage = 'Snapshot preview loaded.';
    } catch (e: any) {
      errorMessage = e.message || 'Preview failed.';
    } finally {
      loadingPreview = false;
    }
  }

  async function createJob() {
    creatingJob = true;
    errorMessage = '';
    successMessage = '';
    try {
      const res = await fetch('/admin/git-restore/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
        body: JSON.stringify({ snapshot_path: snapshotPath, target_project_id: targetProjectId }),
      });
      if (!res.ok) throw new Error(await responseError(res));
      selectedJob = await res.json();
      successMessage = 'Restore job created.';
      await loadJobs();
    } catch (e: any) {
      errorMessage = e.message || 'Create job failed.';
    } finally {
      creatingJob = false;
    }
  }

  async function runJob(job: RestoreJob) {
    runningJobId = job.id;
    errorMessage = '';
    successMessage = '';
    try {
      const res = await fetch(`/admin/git-restore/jobs/${encodeURIComponent(job.id)}/run`, {
        method: 'POST',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      const body = await res.json().catch(() => null);
      if (!res.ok) throw new Error(errorText(body, `Run failed: ${res.status}`));
      selectedJob = body;
      successMessage = body.status === 'completed' ? 'Restore completed.' : 'Restore job updated.';
      await loadJobs();
    } catch (e: any) {
      errorMessage = e.message || 'Run job failed.';
      await loadJobs();
    } finally {
      runningJobId = '';
    }
  }

  async function loadJobs() {
    loadingJobs = true;
    try {
      const res = await fetch('/admin/git-restore/jobs?limit=25');
      if (!res.ok) throw new Error(await responseError(res));
      const body = await res.json();
      jobs = body.items || [];
    } catch (e: any) {
      errorMessage = e.message || 'Failed to load restore jobs.';
    } finally {
      loadingJobs = false;
    }
  }

  function selectJob(job: RestoreJob) {
    selectedJob = job;
    preview = job.preview ?? preview;
    snapshotPath = job.snapshot_path;
    targetProjectId = job.target_project_id;
  }

  async function responseError(res: Response): Promise<string> {
    const body = await res.json().catch(() => null);
    return errorText(body, `Request failed: ${res.status}`);
  }

  function errorText(body: any, fallback: string): string {
    if (body?.errors) {
      const first = Object.values(body.errors).flat()[0];
      if (typeof first === 'string') return first;
    }
    return body?.error || body?.message || fallback;
  }

  function statusClass(status: RestoreJob['status']): string {
    switch (status) {
      case 'completed':
        return 'bg-emerald-50 text-emerald-700 ring-emerald-200';
      case 'failed':
        return 'bg-red-50 text-red-700 ring-red-200';
      case 'running':
        return 'bg-blue-50 text-blue-700 ring-blue-200';
      case 'cancelled':
        return 'bg-gray-100 text-gray-600 ring-gray-200';
      default:
        return 'bg-amber-50 text-amber-700 ring-amber-200';
    }
  }

  function formatDate(value?: string): string {
    if (!value) return '-';
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(new Date(value));
  }
</script>

<div class="space-y-6">
  <div class="rounded-md border border-gray-200 bg-white">
    <div class="border-b border-gray-200 px-5 py-4">
      <h2 class="text-base font-semibold text-gray-900">Git Restore</h2>
      <p class="mt-1 text-sm text-gray-500">Validate a materialized project snapshot and restore it as a tracked admin job.</p>
    </div>

    <div class="grid gap-5 p-5 lg:grid-cols-[minmax(0,1fr)_320px]">
      <div class="space-y-4">
        <label class="block">
          <span class="text-sm font-medium text-gray-700">Snapshot path</span>
          <input
            class="mt-1 block w-full rounded-md border-gray-300 text-sm shadow-sm focus:border-pletka-primary focus:ring-pletka-primary"
            bind:value={snapshotPath}
            placeholder="/tmp/pletka-export/TPC"
          />
        </label>

        <label class="block">
          <span class="text-sm font-medium text-gray-700">Target project ID</span>
          <input
            class="mt-1 block w-full rounded-md border-gray-300 text-sm shadow-sm focus:border-pletka-primary focus:ring-pletka-primary"
            bind:value={targetProjectId}
            placeholder="TPC"
          />
        </label>

        <div class="flex flex-wrap gap-3">
          <button
            type="button"
            class="inline-flex items-center rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-50"
            disabled={loadingPreview || !snapshotPath.trim()}
            onclick={previewSnapshot}
          >
            {loadingPreview ? 'Previewing...' : 'Preview'}
          </button>
          <button
            type="button"
            class="inline-flex items-center rounded-md bg-pletka-primary px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-pletka-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
            disabled={creatingJob || !snapshotPath.trim() || !targetProjectId.trim()}
            onclick={createJob}
          >
            {creatingJob ? 'Creating...' : 'Create Job'}
          </button>
          <button
            type="button"
            class="inline-flex items-center rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
            onclick={loadJobs}
          >
            Refresh Jobs
          </button>
        </div>

        {#if errorMessage}
          <div class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{errorMessage}</div>
        {/if}
        {#if successMessage}
          <div class="rounded-md border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{successMessage}</div>
        {/if}
      </div>

      <div class="rounded-md border border-gray-200 bg-gray-50 p-4">
        {#if preview}
          <div class="space-y-3">
            <div>
              <p class="text-xs font-medium uppercase text-gray-500">Snapshot</p>
              <p class="mt-1 text-sm font-semibold text-gray-900">{preview.project_id}</p>
              <p class="break-all text-xs text-gray-500">{preview.module_path || preview.snapshot_path}</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <span class="rounded-full bg-white px-2 py-1 text-xs font-medium text-gray-700 ring-1 ring-gray-200">
                {preview.self_contained ? 'Self-contained' : 'Repo snapshot'}
              </span>
              <span class="rounded-full bg-white px-2 py-1 text-xs font-medium text-gray-700 ring-1 ring-gray-200">
                {preview.has_lockfile ? 'Lockfile' : 'No lockfile'}
              </span>
            </div>
            <dl class="grid grid-cols-2 gap-2 text-sm">
              {#each entityCountRows as row}
                <div class="rounded bg-white px-3 py-2 ring-1 ring-gray-200">
                  <dt class="text-xs text-gray-500">{row[0]}</dt>
                  <dd class="font-semibold text-gray-900">{row[1]}</dd>
                </div>
              {/each}
            </dl>
          </div>
        {:else}
          <p class="text-sm text-gray-500">Run a preview to inspect the snapshot before creating a restore job.</p>
        {/if}
      </div>
    </div>
  </div>

  <div class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
    <div class="rounded-md border border-gray-200 bg-white">
      <div class="flex items-center justify-between border-b border-gray-200 px-5 py-4">
        <h3 class="text-sm font-semibold text-gray-900">Restore Jobs</h3>
        {#if loadingJobs}
          <span class="text-xs text-gray-500">Loading...</span>
        {/if}
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 text-sm">
          <thead class="bg-gray-50 text-left text-xs font-medium uppercase text-gray-500">
            <tr>
              <th class="px-4 py-3">Job</th>
              <th class="px-4 py-3">Target</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3">Created</th>
              <th class="px-4 py-3"></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 bg-white">
            {#each jobs as job (job.id)}
              <tr class:selected={selectedJob?.id === job.id}>
                <td class="px-4 py-3">
                  <button class="text-left" type="button" onclick={() => selectJob(job)}>
                    <span class="block font-medium text-gray-900">{job.source_project_id}</span>
                    <span class="block max-w-[24rem] truncate text-xs text-gray-500">{job.snapshot_path}</span>
                  </button>
                </td>
                <td class="px-4 py-3 font-medium text-gray-700">{job.target_project_id}</td>
                <td class="px-4 py-3">
                  <span class={`inline-flex rounded-full px-2 py-1 text-xs font-medium ring-1 ${statusClass(job.status)}`}>
                    {job.status}
                  </span>
                </td>
                <td class="px-4 py-3 text-gray-500">{formatDate(job.created_at)}</td>
                <td class="px-4 py-3 text-right">
                  <button
                    type="button"
                    class="rounded-md border border-gray-300 px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
                    disabled={runningJobId === job.id || job.status === 'running' || job.status === 'completed'}
                    onclick={() => runJob(job)}
                  >
                    {runningJobId === job.id ? 'Running...' : 'Run'}
                  </button>
                </td>
              </tr>
            {:else}
              <tr>
                <td colspan="5" class="px-4 py-8 text-center text-sm text-gray-500">No restore jobs yet.</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

    <aside class="rounded-md border border-gray-200 bg-white">
      <div class="border-b border-gray-200 px-5 py-4">
        <h3 class="text-sm font-semibold text-gray-900">Job Detail</h3>
      </div>
      {#if selectedJob}
        <div class="space-y-4 p-5 text-sm">
          <div>
            <p class="text-xs font-medium uppercase text-gray-500">Job ID</p>
            <p class="break-all font-mono text-xs text-gray-700">{selectedJob.id}</p>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <p class="text-xs text-gray-500">Source</p>
              <p class="font-medium text-gray-900">{selectedJob.source_project_id}</p>
            </div>
            <div>
              <p class="text-xs text-gray-500">Target</p>
              <p class="font-medium text-gray-900">{selectedJob.target_project_id}</p>
            </div>
          </div>
          <div>
            <p class="text-xs text-gray-500">Current phase</p>
            <p class="font-medium text-gray-900">{selectedJob.current_phase || '-'}</p>
          </div>
          {#if selectedJob.error_message}
            <div class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-red-700">
              {selectedJob.error_message}
            </div>
          {/if}
          <dl class="grid grid-cols-2 gap-3 text-xs text-gray-600">
            <div>
              <dt>Started</dt>
              <dd class="font-medium text-gray-900">{formatDate(selectedJob.started_at)}</dd>
            </div>
            <div>
              <dt>Finished</dt>
              <dd class="font-medium text-gray-900">{formatDate(selectedJob.finished_at)}</dd>
            </div>
          </dl>
        </div>
      {:else}
        <p class="p-5 text-sm text-gray-500">Select a restore job to inspect status and phase details.</p>
      {/if}
    </aside>
  </div>
</div>

<style>
  tr.selected {
    background: rgb(249 250 251);
  }
</style>
