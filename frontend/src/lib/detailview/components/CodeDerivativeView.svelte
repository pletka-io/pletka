<!--
  Shared text-derivative panel: fetches a server-emitted URL, runs
  Shiki against the response, surfaces copy + download buttons, and
  exposes optional toolbar / control slots for per-format widgets
  (e.g. SPARQL limit/count, X3ML form toggle).

  Caches per-URL response so re-activating the tab doesn't re-fetch.
  Reactive on `url` so caller-driven URL changes (e.g. SPARQL adding
  ?count=1) trigger a fresh fetch automatically.

  Consumers pass:
    - url:        derivatives.<format>_url
    - lang:       Shiki language identifier (turtle, sparql, xml, yaml…)
    - filename:   download filename (entity_systemname.ttl etc.)
    - emptyLabel: shown when the URL is missing or the response was empty
    - errorLabel: prefix for fetch errors

  The header strip is fixed-height; control slot lives left of the
  Copy / Download buttons.
-->
<script lang="ts">
  import { getDerivativeText } from '$lib/api/client';
  import type { DerivativeLang } from '../highlight';
  // Dynamic import keeps Shiki out of the entity-view entry bundle —
  // it only loads when a derivative tab using this component is
  // actually opened.
  const highlightModule = () => import('../highlight');

  let {
    url,
    lang,
    filename,
    emptyLabel = 'No content available',
    errorLabel = 'derivative',
    headerLabel = '',
    controls,
    belowHeader,
    helpText = '',
  }: {
    url: string | undefined;
    lang: DerivativeLang;
    filename: string;
    emptyLabel?: string;
    errorLabel?: string;
    headerLabel?: string;
    controls?: import('svelte').Snippet;
    // Optional slot rendered between the header strip and the code
    // body. Used for collapsible panels (e.g. X3ML integration actions)
    // that should sit visually inside the panel but above the code.
    belowHeader?: import('svelte').Snippet;
    helpText?: string;
  } = $props();

  let source = $state('');
  let highlighted = $state('');
  let loading = $state(false);
  let error = $state<string | null>(null);
  let copied = $state(false);

  // Per-URL cache so toggling sub-tabs doesn't re-fetch the same
  // payload; re-fetches automatically when the URL string changes
  // (e.g. SPARQL ?limit=50 → ?limit=100).
  const cache = new Map<string, string>();
  let lastUrl: string | undefined;

  async function load() {
    if (!url) {
      source = '';
      highlighted = '';
      error = null;
      return;
    }
    if (cache.has(url)) {
      source = cache.get(url) ?? '';
      const { highlight } = await highlightModule();
      highlighted = await highlight(source, lang);
      return;
    }
    loading = true;
    error = null;
    try {
      const text = await getDerivativeText(url, errorLabel);
      // JSON pretty-print so Shiki produces a readable formatted
      // tree even when the server emitted compact JSON. Idempotent
      // for already-pretty input. Other languages pass through.
      const formatted = lang === 'json' ? prettyJson(text) : text;
      cache.set(url, formatted);
      source = formatted;
      const { highlight } = await highlightModule();
      highlighted = await highlight(source, lang);
    } catch (e) {
      error = e instanceof Error ? e.message : `Failed to load ${errorLabel}`;
      source = '';
      highlighted = '';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (url !== lastUrl) {
      lastUrl = url;
      load();
    }
  });

  async function copy() {
    if (!source) return;
    try {
      await navigator.clipboard.writeText(source);
      copied = true;
      setTimeout(() => {
        copied = false;
      }, 2000);
    } catch (e) {
      console.warn('copy failed', e);
    }
  }

  function prettyJson(text: string): string {
    try {
      return JSON.stringify(JSON.parse(text), null, 2);
    } catch {
      return text;
    }
  }

  function download() {
    if (!source) return;
    const blob = new Blob([source], { type: 'text/plain;charset=utf-8' });
    const objectURL = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = objectURL;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(objectURL);
  }
</script>

<div class="bg-white border border-gray-200 rounded-lg overflow-hidden">
  <div class="flex items-center justify-between px-4 py-2 border-b border-gray-200 bg-gray-50">
    <div class="flex items-center gap-3 flex-wrap">
      {#if headerLabel}
        <span class="text-xs font-medium text-gray-500">{headerLabel}</span>
      {/if}
      {#if controls}
        {@render controls()}
      {/if}
    </div>
    {#if source}
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="flex items-center px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded"
          onclick={copy}
        >
          {#if copied}
            <svg class="inline-block w-3.5 h-3.5 mr-1 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
            </svg>
            Copied!
          {:else}
            <svg class="inline-block w-3.5 h-3.5 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
            Copy
          {/if}
        </button>
        <button
          type="button"
          class="flex items-center px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded"
          onclick={download}
          title="Download {filename}"
        >
          <svg class="inline-block w-3.5 h-3.5 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
          </svg>
          Download
        </button>
      </div>
    {/if}
  </div>

  {#if belowHeader}
    {@render belowHeader()}
  {/if}

  <div class="h-[500px] overflow-auto bg-gray-50">
    {#if loading}
      <div class="flex items-center justify-center h-full">
        <div class="text-center">
          <svg class="animate-spin h-8 w-8 text-blue-500 mx-auto" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          <p class="mt-2 text-sm text-gray-500">Loading…</p>
        </div>
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-full">
        <div class="text-center">
          <svg class="mx-auto h-12 w-12 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <p class="mt-2 text-sm text-red-600">{error}</p>
        </div>
      </div>
    {:else if highlighted}
      <div class="code-derivative-pane p-4 text-xs">{@html highlighted}</div>
      {#if helpText}
        <div class="px-4 pb-3 text-[11px] text-gray-500 italic">{helpText}</div>
      {/if}
    {:else}
      <div class="flex items-center justify-center h-full">
        <p class="text-sm text-gray-400">{emptyLabel}</p>
      </div>
    {/if}
  </div>
</div>

<style>
  /* Shiki emits a <pre> with theme-driven inline styles; we add tweak
     classes here so derivative panels share spacing + word-wrap rules
     across all formats without reimplementing the highlighter CSS. */
  :global(.code-derivative-pane pre.shiki),
  :global(.code-derivative-pane pre.shiki-plain) {
    margin: 0;
    background: transparent !important;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
  }
</style>
