<script lang="ts">
  import { tr, type Translations } from '$lib/types/form-schema';

  let { url, lang, oncancel }: { url: string; lang: string; oncancel?: () => void } = $props();
  type Value = { id: number; field_id: string; label: Translations; slot_path: string; occurrence_index: number; kind: string; unresolved: boolean; payload: Record<string, unknown> | null; text_value?: string; number_value?: string; date_value?: string; uri_value?: string; concept_uri?: string; linked_example_id?: string; linked_url?: string };
  type Example = { id: string; version: string; title: Translations; description: Translations; status: string; target_id: string; target_name: Translations; target_url: string; values: Value[] };
  let value = $state<Example | null>(null);
  let error = $state('');
  let loading = $state(true);
  $effect(() => {
    const controller = new AbortController();
    value = null; error = ''; loading = true;
    async function load() {
      try {
        if (!url) throw new Error('Example view URL is missing');
        const response = await fetch(url, { signal: controller.signal });
        if (!response.ok) {
          const body = await response.json().catch(() => ({}));
          throw new Error(body.message || body.error || `Unable to load example (${response.status})`);
        }
        value = await response.json();
      } catch (e) {
        if (!controller.signal.aborted) error = e instanceof Error ? e.message : 'Unable to load example';
      } finally { if (!controller.signal.aborted) loading = false; }
    }
    void load();
    return () => controller.abort();
  });
  function display(row: Value): string {
    const payload = row.payload ?? {};
    if (payload.concept_uri) return [payload.concept_label, payload.concept_uri].filter(Boolean).join(' · ');
    if (payload.example_id) return [payload.target_label, payload.example_id].filter(Boolean).join(' · ');
    for (const key of ['string_value', 'number_value', 'bool_value', 'date_value', 'uri_value']) {
      if (payload[key] !== undefined && payload[key] !== null) return String(payload[key]);
    }
    const recorded = row.text_value ?? row.number_value ?? row.date_value ?? row.uri_value ?? row.concept_uri ?? row.linked_example_id;
    if (recorded !== undefined && recorded !== null) return String(recorded);
    return Object.keys(payload).some((key) => key !== 'kind') ? JSON.stringify(payload) : '—';
  }
</script>

<div class="space-y-5">
  <button type="button" onclick={oncancel} class="text-sm text-pletka-primary hover:underline">Back to examples</button>
  {#if loading}
    <p class="text-sm text-gray-500" role="status">Loading example…</p>
  {:else if error}
    <p class="text-sm text-red-700" role="alert">{error}</p>
  {:else if value}
    <header class="space-y-2">
      <h2 class="text-xl font-semibold text-gray-900">{tr(value.title, lang) || value.id}</h2>
      <p class="text-sm text-gray-600">{tr(value.description, lang)}</p>
      <p class="text-sm"><a href={value.target_url} class="text-pletka-primary hover:underline">{tr(value.target_name, lang) || value.target_id}</a> · {value.status}{#if value.version} · Release {value.version}{/if}</p>
      <p class="text-xs text-gray-500">Recorded values and status. This view does not run validation again.</p>
    </header>
    {#if value.values.length === 0}
      <p class="text-sm text-gray-500">No values recorded.</p>
    {:else}
      <dl class="divide-y divide-gray-200 rounded-md border border-gray-200 bg-white px-4">
        {#each value.values as row (row.id)}
          <div class="py-4 sm:grid sm:grid-cols-3 sm:gap-4">
            <dt class="text-sm font-medium text-gray-900">{tr(row.label, lang) || row.field_id}<span class="mt-1 block font-normal text-xs text-gray-500">Occurrence {row.occurrence_index + 1}{#if row.unresolved} · Field no longer placed{/if}</span></dt>
            <dd class="mt-2 whitespace-pre-wrap break-words text-sm text-gray-700 sm:col-span-2 sm:mt-0">{display(row)}{#if row.linked_url}<a href={row.linked_url} class="mt-2 block text-pletka-primary hover:underline">Open linked example</a>{/if}</dd>
          </div>
        {/each}
      </dl>
    {/if}
  {/if}
</div>
