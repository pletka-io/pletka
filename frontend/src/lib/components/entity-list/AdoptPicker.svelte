<script lang="ts">
  /*
   * AdoptPicker — side-panel picker that lists adoptable entities from
   * the project's ancestor chain. Lazy-loads from the
   * options URL, narrows by search + source-project filter, POSTs the
   * adoption row on confirm, and emits `onadopted` so the host can
   * refresh the list.
   *
   * Layout mirrors OverrideSidebar.svelte: fixed right column at
   * z-50, scrollable middle, sticky footer with primary Adopt button.
  */
  import { tr } from '$lib/types/form-schema';
  import type { AdoptableOption, AdoptableSource, EntityListCapabilities } from '$lib/types/entity-list-schema';
  import { controlID } from './control-id';

  let {
    cap,
    entityType,
    lang,
    onclose,
    onadopted,
  }: {
    cap: NonNullable<EntityListCapabilities['adopt']>;
    entityType: string;
    lang: string;
    onclose: () => void;
    /**
     * Fires after a successful POST. The label is the adopted entity's
     * display name in the current UI language; the host uses it to
     * compose a toast such as "Adopted Activity (LAM.1)" without
     * baking domain wording into AdoptPicker itself.
     */
    onadopted: (label: string) => void;
  } = $props();

  let loading = $state(true);
  let saving = $state(false);
  let errorMessage = $state('');
  let options = $state<AdoptableOption[]>([]);
  let sources = $state<AdoptableSource[]>([]);
  let selectedID = $state('');
  let query = $state('');
  let sourceFilter = $state('');
  const controlPrefix = $derived(controlID('entity-list', entityType, 'adopt'));
  const searchID = $derived(controlID(controlPrefix, 'search'));
  const sourceFilterID = $derived(controlID(controlPrefix, 'source-project'));
  const targetName = $derived(controlID(controlPrefix, 'target'));
  const versionID = $derived(controlID(controlPrefix, 'source-version'));

  // Fetch options on mount.
  $effect(() => {
    void load();
  });

  /*
   * Loads candidates. Each ancestor is read at one of ITS OWN live
   * releases — never at the draft, and never at the adopting project's
   * version. Passing a project and version pins that one ancestor to a
   * release the curator chose; everything else stays on its newest.
   */
  async function load(pinProject = '', pinVersion = '') {
    loading = true;
    errorMessage = '';
    try {
      const url = new URL(cap.options_url, window.location.origin);
      if (pinProject && pinVersion) {
        url.searchParams.set('source_project_id', pinProject);
        url.searchParams.set('source_version', pinVersion);
      }
      const res = await fetch(url.toString());
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { options?: AdoptableOption[]; sources?: AdoptableSource[] };
      options = data.options ?? [];
      sources = data.sources ?? [];
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : String(err);
    } finally {
      loading = false;
    }
  }

  /*
   * Driven by `sources`, not by the options, so an ancestor that has
   * published nothing still appears — with a reason. Deriving it from
   * the options would hide exactly the case a curator needs told.
   */
  const sourceProjects = $derived(
    sources.map((s) => ({
      id: s.project_id,
      name: s.project_name ?? s.project_id,
      unreleased: s.unreleased === true,
    })),
  );

  const activeSource = $derived(sources.find((s) => s.project_id === sourceFilter));

  async function pickVersion(version: string) {
    if (!sourceFilter || !version) return;
    selectedID = '';
    await load(sourceFilter, version);
  }

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return options.filter((o) => {
      if (sourceFilter && o.source_project_id !== sourceFilter) return false;
      if (!q) return true;
      const label = tr(o.label, lang).toLowerCase();
      return (
        label.includes(q)
        || o.value.toLowerCase().includes(q)
        || (o.semantic_id ?? '').toLowerCase().includes(q)
        || (o.ontology_scope ?? '').toLowerCase().includes(q)
      );
    });
  });

  async function adopt() {
    if (!selectedID || saving) return;
    const target = options.find((o) => o.value === selectedID);
    if (!target) return;
    saving = true;
    errorMessage = '';
    try {
      const res = await fetch(cap.url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          entity_type: entityType,
          source_project_id: target.source_project_id,
          source_entity_id: target.value,
          // The release this candidate was listed at. The server refuses
          // an adopt without it: an adoption records what it depends on.
          source_version: target.source_version,
        }),
      });
      if (!res.ok) {
        const body = await res.text();
        throw new Error(body || `HTTP ${res.status}`);
      }
      const label = tr(target.label, lang) || target.semantic_id || target.value;
      onadopted(label);
      onclose();
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : String(err);
    } finally {
      saving = false;
    }
  }
</script>

<aside class="adopt-picker fixed top-0 right-0 h-full w-[420px] bg-white border-l border-gray-200 shadow-xl z-50 flex flex-col">
  <header class="px-5 py-4 border-b border-gray-200 flex items-center gap-3">
    <div class="flex-1 min-w-0">
      <div class="text-[10px] uppercase tracking-widest text-pletka-primary font-medium">Adopt · {entityType}</div>
      <h3 class="text-base font-semibold text-gray-900 truncate">{tr(cap.label, lang)}</h3>
    </div>
    <button
      type="button"
      class="text-gray-400 hover:text-gray-700 text-lg"
      title="Close (Esc)"
      onclick={onclose}
    >✕</button>
  </header>

  <div class="px-5 py-3 border-b border-gray-100 space-y-2">
    {#if cap.destination_label}
      <p class="text-xs text-gray-500">{tr(cap.destination_label, lang)}</p>
    {/if}
    <input
      id={searchID}
      name={searchID}
      type="search"
      aria-label="Search adoptable {entityType}"
      placeholder="Search name, ID, scope…"
      bind:value={query}
      class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
    />
    {#if sourceProjects.length > 1}
      <select
        id={sourceFilterID}
        name={sourceFilterID}
        aria-label="Filter adoptable {entityType} by source project"
        bind:value={sourceFilter}
        class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
      >
        <option value="">All source projects</option>
        {#each sourceProjects as sp (sp.id)}
          <option value={sp.id}>{sp.name}{sp.unreleased ? ' — nothing published' : ''}</option>
        {/each}
      </select>

      {#if activeSource && !activeSource.unreleased && activeSource.versions.length > 0}
        <!--
          Which release to adopt from. The newest is the default, not the only
          option: a curator may deliberately depend on an older, stable release
          rather than whatever was published last.
        -->
        <label class="block text-xs font-medium text-gray-600 mt-3" for={versionID}>
          Adopt from release
        </label>
        <select
          id={versionID}
          name={versionID}
          aria-label="Release of {activeSource.project_name ?? activeSource.project_id} to adopt from"
          value={activeSource.selected_version ?? activeSource.versions[0]}
          onchange={(e) => pickVersion((e.currentTarget as HTMLSelectElement).value)}
          class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
        >
          {#each activeSource.versions as v, i (v)}
            <option value={v}>{v}{i === 0 ? ' (latest)' : ''}</option>
          {/each}
        </select>
        <p class="text-xs text-gray-500 mt-1">
          The adoption records this release, so the project keeps following
          {activeSource.selected_version ?? activeSource.versions[0]} until you re-adopt.
        </p>
      {/if}

      {#if activeSource?.unreleased}
        <!--
          Said, rather than shown as an empty list: "no candidates" does not
          tell the curator that the fix is for the source project to publish.
        -->
        <p class="mt-3 rounded-md bg-amber-50 border border-amber-200 px-3 py-2 text-xs text-amber-800">
          {activeSource.project_name ?? activeSource.project_id} has not published a release yet,
          so there is nothing to adopt from it. An adoption always points at a release, never at a
          draft — once they publish one it will appear here.
        </p>
      {/if}
    {/if}
  </div>

  <div class="flex-1 overflow-y-auto">
    {#if loading}
      <div class="animate-pulse space-y-3 px-5 py-4">
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
      </div>
    {:else if errorMessage}
      <div class="m-5 rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{errorMessage}</div>
    {:else if filtered.length === 0}
      <div class="m-5 rounded border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-500">
        Nothing left to adopt. Either everything from ancestor projects is already in this project, or this project has no ancestors.
      </div>
    {:else}
      <ul class="divide-y divide-gray-100">
        {#each filtered as opt (opt.value)}
          <li>
            <label class="block px-5 py-3 cursor-pointer hover:bg-gray-50 {selectedID === opt.value ? 'bg-pletka-primary/5' : ''}">
              <div class="flex items-start gap-3">
                <input
                  id={controlID(targetName, opt.value)}
                  type="radio"
                  name={targetName}
                  value={opt.value}
                  bind:group={selectedID}
                  class="mt-1 text-pletka-primary focus:ring-pletka-primary"
                />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-gray-900 truncate">{tr(opt.label, lang)}</div>
                  <div class="text-xs text-gray-500 truncate">
                    {opt.semantic_id ?? opt.value}
                    {#if opt.ontology_scope}
                      · <span class="font-mono">{opt.ontology_scope}</span>
                    {/if}
                  </div>
                  <div class="text-xs text-amber-700 mt-0.5">from {opt.source_project_name ?? opt.source_project_id} · {opt.source_version}</div>
                </div>
              </div>
            </label>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <footer class="px-5 py-3 border-t border-gray-200 bg-gray-50 flex items-center justify-end gap-2">
    <button
      type="button"
      class="px-3 py-2 rounded-md text-sm font-medium text-gray-700 hover:bg-gray-100"
      onclick={onclose}
      disabled={saving}
    >Cancel</button>
    <button
      type="button"
      class="px-4 py-2 rounded-md bg-pletka-primary text-white text-sm font-medium hover:bg-pletka-secondary disabled:opacity-50 focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
      onclick={adopt}
      disabled={!selectedID || saving}
    >{saving ? 'Adopting…' : 'Adopt'}</button>
  </footer>
</aside>

<button
  type="button"
  class="adopt-backdrop fixed inset-0 bg-black/20 z-40"
  aria-label="Close adopt picker"
  onclick={onclose}
></button>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') onclose(); }} />
