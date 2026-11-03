<script lang="ts">
  import type { TermDescription } from '$lib/types/ontology-types';

  let { url, lang = 'en', onClose }: {
    /** Fully-resolved describe URL (schema template with {project_id}/{qname} already substituted). */
    url: string;
    lang?: string;
    onClose: () => void;
  } = $props();

  let term = $state<TermDescription | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  $effect(() => {
    let cancelled = false;
    loading = true;
    error = null;
    term = null;
    fetch(url, { headers: { 'X-Requested-With': 'XMLHttpRequest' } })
      .then(async (res) => {
        if (!res.ok) throw new Error(res.status === 404 ? 'Term not found' : `Request failed (${res.status})`);
        return res.json() as Promise<TermDescription>;
      })
      .then((data) => { if (!cancelled) term = data; })
      .catch((e) => { if (!cancelled) error = e instanceof Error ? e.message : 'Describe failed'; })
      .finally(() => { if (!cancelled) loading = false; });
    return () => { cancelled = true; };
  });

  const isClass = $derived(term?.type === 'class');

  /** Resolve a referenced qname to its label (falls back to the qname). */
  function termLabel(q: string): string {
    const m = term?.labels?.[q];
    return m?.[lang] || m?.en || q;
  }
  function tr(t: Record<string, string> | undefined): string {
    return t?.[lang] || t?.en || '';
  }

  // Sections: [title, qnames]. Built reactively from the loaded term.
  const sections = $derived.by<Array<[string, string[]]>>(() => {
    if (!term) return [];
    if (isClass) {
      return [
        ['Broader (superclasses)', term.superclasses ?? []],
        ['Narrower (subclasses)', term.subclasses ?? []],
        ['Direct properties', term.as_domain_of ?? []],
        ['Inherited properties', term.inherited_properties ?? []],
      ];
    }
    return [
      ['Domain', term.domains ?? []],
      ['Range', term.ranges ?? []],
      ['Super-properties', term.super_properties ?? []],
      ['Used on', term.as_range_of ?? []],
    ];
  });

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose();
  }
</script>

<svelte:window on:keydown={onKeydown} />

<!-- Backdrop -->
<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
  role="button"
  tabindex="-1"
  aria-label="Close"
  onclick={onClose}
  onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') onClose(); }}
>
  <!-- Panel -->
  <div
    class="w-full max-w-lg max-h-[85vh] overflow-y-auto rounded-lg bg-white shadow-xl"
    role="dialog"
    aria-modal="true"
    aria-label="Ontology term details"
    tabindex="0"
    onclick={(e) => e.stopPropagation()}
    onkeydown={(e) => e.stopPropagation()}
  >
    {#if loading}
      <div class="p-6 text-sm text-gray-500">Loading term…</div>
    {:else if error}
      <div class="p-6">
        <div class="text-sm text-red-600">{error}</div>
        <button type="button" class="mt-3 text-sm text-pletka-primary hover:underline" onclick={onClose}>Close</button>
      </div>
    {:else if term}
      <div class="sticky top-0 border-b border-gray-200 bg-white px-5 py-4">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="text-lg font-semibold text-gray-900 text-balance">
              {tr(term.label) || term.qname}
            </div>
            <div class="mt-0.5 font-mono text-xs text-gray-500 break-all">{term.qname}</div>
          </div>
          <button
            type="button"
            class="shrink-0 rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700"
            aria-label="Close"
            onclick={onClose}
          >&times;</button>
        </div>
        <div class="mt-2 flex flex-wrap gap-1.5 text-xs">
          <span class="rounded bg-gray-100 px-1.5 py-0.5 text-gray-700">
            {isClass ? 'Class' : term.property_type === 'ObjectProperty' ? 'Object property' : term.property_type === 'DatatypeProperty' ? 'Datatype property' : 'Property'}
          </span>
          {#if term.ontology}
            <span class="rounded bg-indigo-50 px-1.5 py-0.5 text-indigo-700">
              {term.ontology}{term.version ? ` ${term.version}` : ''}
            </span>
          {/if}
        </div>
      </div>

      <div class="space-y-4 px-5 py-4">
        {#if tr(term.comment)}
          <p class="whitespace-pre-line text-sm leading-relaxed text-gray-700">{tr(term.comment)}</p>
        {/if}

        {#each sections as [title, qnames]}
          {#if qnames.length}
            <div>
              <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
                {title} <span class="font-normal text-gray-400">({qnames.length})</span>
              </div>
              <div class="flex flex-wrap gap-1">
                {#each qnames as q}
                  <span
                    class="inline-flex items-center rounded-full border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs text-gray-700"
                    title={q}
                  >{termLabel(q)}</span>
                {/each}
              </div>
            </div>
          {/if}
        {/each}

        {#if term.uri}
          <div class="border-t border-gray-100 pt-3">
            <span class="font-mono text-[11px] text-gray-400 break-all">{term.uri}</span>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
