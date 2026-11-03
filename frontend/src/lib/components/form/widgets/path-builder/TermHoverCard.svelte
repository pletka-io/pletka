<script lang="ts">
  import type { TermDescription } from '$lib/types/ontology-types';

  let { url, lang = 'en' }: {
    /** Resolved describe URL ({project_id}/{qname} already substituted). */
    url: string;
    lang?: string;
  } = $props();

  // Module-level cache so re-hovering the same term is instant and we never
  // refetch across pills pointing at the same qname.
  const cache = new Map<string, TermDescription>();

  let term = $state<TermDescription | null>(null);

  $effect(() => {
    const cached = cache.get(url);
    if (cached) { term = cached; return; }
    let cancelled = false;
    fetch(url, { headers: { 'X-Requested-With': 'XMLHttpRequest' } })
      .then((r) => (r.ok ? (r.json() as Promise<TermDescription>) : null))
      .then((d) => { if (d && !cancelled) { cache.set(url, d); term = d; } })
      .catch(() => {});
    return () => { cancelled = true; };
  });

  function tr(t: Record<string, string> | undefined): string {
    return t?.[lang] || t?.en || '';
  }
  function truncate(s: string, n: number): string {
    return s.length > n ? s.slice(0, n) + '…' : s;
  }
  const isClass = $derived(term?.type === 'class');
</script>

<div
  class="absolute left-0 top-full z-20 mt-1 w-72 rounded-md bg-gray-900 p-2.5 text-xs text-gray-100 shadow-lg"
  role="tooltip"
>
  {#if term}
    <div class="font-semibold text-white">{tr(term.label) || term.qname}</div>
    <div class="font-mono text-[11px] text-gray-400 break-all">{term.qname}</div>
    <div class="mt-1 flex flex-wrap gap-1">
      <span class="rounded bg-gray-700 px-1.5 py-0.5">
        {isClass ? 'Class' : term.property_type === 'ObjectProperty' ? 'Object property' : term.property_type === 'DatatypeProperty' ? 'Datatype property' : 'Property'}
      </span>
      {#if term.ontology}
        <span class="rounded bg-indigo-600 px-1.5 py-0.5">{term.ontology}{term.version ? ` ${term.version}` : ''}</span>
      {/if}
    </div>
    {#if tr(term.comment)}
      <div class="mt-1.5 leading-relaxed text-gray-300">{truncate(tr(term.comment), 180)}</div>
    {/if}
    <div class="mt-1.5 border-t border-gray-700 pt-1.5 text-[11px] text-gray-400">Click for full details</div>
  {:else}
    <div class="text-gray-400">Loading…</div>
  {/if}
</div>
