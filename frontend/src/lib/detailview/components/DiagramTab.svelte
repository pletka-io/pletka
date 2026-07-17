<script lang="ts">
  import {
    getDerivativeDiagram,
    getDerivativeText,
    getDerivativeJSON,
  } from '$lib/api/client';
  import ExportGraphExplorer from './ExportGraphExplorer.svelte';
  import CodeDerivativeView from './CodeDerivativeView.svelte';
  import ActionGroup from '$lib/schema/ActionGroup.svelte';
  import type { ExportGraph } from '../export-graph';
  import type { DerivativesCap } from '../types';

  let {
    entityId,
    entityType,
    projectId,
    derivatives,
  }: {
    entityId: string;
    entityType: string;
    projectId: string;
    derivatives?: DerivativesCap;
  } = $props();

  type SubView =
    | 'diagram'
    | 'graph'
    | 'rdf'
    | 'shacl'
    | 'sparql'
    | 'x3ml'
    | 'researchspace'
    | 'arches'
    | 'snapshot'
    | 'ascii_tree'
    | 'csv';
  type DiagramMode = 'ontology' | 'instance';
  type RdfFormat = 'turtle' | 'jsonld';
  type X3MLForm = 'a' | 'b';

  let subView = $state<SubView>('diagram');
  let diagramMode = $state<DiagramMode>('ontology');
  let rdfFormat = $state<RdfFormat>('turtle');

  // SPARQL controls. Limit defaults to 100 (matches the legacy Python
  // pipeline + the server handler default). Count toggle swaps SELECT
  // for COUNT(?value) projection. Both flow through the URL as query
  // params; CodeDerivativeView re-fetches automatically when the URL
  // string changes.
  let sparqlLimit = $state<number>(100);
  let sparqlCount = $state<boolean>(false);
  // Pending input reflects the typed value while the user is editing.
  // The committed `sparqlLimit` only updates after a 300ms idle so we
  // don't fire a request on every keystroke.
  let sparqlLimitInput = $state<string>('100');
  let sparqlLimitTimer: ReturnType<typeof setTimeout> | null = null;

  // X3ML internal form toggle. Default A — matches the standalone
  // mapping document customers feed into 3M Editor.
  let x3mlForm = $state<X3MLForm>('a');

  // Collapsed by default. The X3ML controls strip exposes a single
  // "Integrations" toggle; expanding reveals one ActionButton per
  // configured integration. Keeps the noise out of the X3ML pane until
  // the operator asks for it.
  let integrationsOpen = $state(false);

  // Diagram state
  let mermaidSource = $state('');
  let svgContent = $state('');
  let diagramLoading = $state(false);
  let diagramError = $state<string | null>(null);
  let zoom = $state(100);
  let diagramContainer = $state<HTMLDivElement | null>(null);
  let diagramLoaded = $state(false);

  // Export graph state
  let exportGraph = $state<ExportGraph | null>(null);
  let graphLoading = $state(false);
  let graphError = $state<string | null>(null);
  let graphLoaded = $state(false);

  // RDF + SHACL state lives inside CodeDerivativeView now (per-URL
  // cache, copy buffer, Shiki output). DiagramTab only tracks the
  // active rdf format toggle so the user's choice persists across
  // sub-tab switches.

  let mermaidRenderCount = 0;

  const MIN_ZOOM = 10;
  const MAX_ZOOM = 500;
  const ZOOM_STEP = 25;
  const WHEEL_ZOOM_STEP = 10; // smaller per-tick, smoother

  // Original SVG dimensions captured after each render so zoom scales
  // the SVG element directly (instead of CSS transform — which doesn't
  // grow the layout box and breaks overflow-auto in the container).
  let svgBaseWidth = 0;
  let svgBaseHeight = 0;
  // Drag-to-pan state. Tracked imperatively so a click that doesn't
  // move past a small threshold still falls through to normal hit
  // testing on SVG nodes.
  let isPanning = false;
  let panStartX = 0;
  let panStartY = 0;
  let panStartScrollLeft = 0;
  let panStartScrollTop = 0;

  // Derivatives are supported when the server emitted the URL block
  // for this entity. No client-side hardcoded entity-type allowlist —
  // the server is the source of truth (api-patterns.md: schema is the
  // complete contract).
  const supportsEntity = $derived(!!derivatives);

  function sanitizeMermaidIdSegment(value: string): string {
    return value.replace(/[^A-Za-z0-9_-]+/g, '-');
  }

  async function loadDiagram() {
    if (diagramLoaded) return;
    if (!derivatives?.diagram_url) return;

    diagramLoading = true;
    diagramError = null;
    try {
      const resp = await getDerivativeDiagram(derivatives.diagram_url, diagramMode);
      if (!resp.success || !resp.mermaid) {
        diagramError = resp.error || 'No diagram data available';
        return;
      }
      mermaidSource = resp.mermaid;
      await renderMermaid(resp.mermaid);
      diagramLoaded = true;
    } catch (e) {
      diagramError = e instanceof Error ? e.message : 'Failed to load diagram';
    } finally {
      diagramLoading = false;
    }
  }

  async function renderMermaid(source: string) {
    try {
      const mermaid = await import('https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs' as string);
      mermaid.default.initialize({
        startOnLoad: false,
        theme: 'default',
        securityLevel: 'loose',
      });
      mermaidRenderCount += 1;
      const renderId = [
        'mermaid-entity-diagram',
        sanitizeMermaidIdSegment(entityType),
        sanitizeMermaidIdSegment(entityId),
        sanitizeMermaidIdSegment(diagramMode),
        String(mermaidRenderCount),
      ].join('-');
      const { svg } = await mermaid.default.render(renderId, source);
      svgContent = svg;
      // Capture mermaid's natural SVG dimensions once. Subsequent zoom
      // operations resize the SVG element directly so the layout box
      // grows; that lets overflow-auto give us scrollbars on both axes
      // (the previous transform: scale approach kept the layout box at
      // 100% and only one scrollbar reached past the centered pivot).
      //
      // Polling: a single requestAnimationFrame fires before mermaid's
      // SVG is laid out, so viewBox.baseVal reads 0/0 and applyZoom
      // bails. Retry until intrinsic dimensions resolve, capped at 20
      // frames (~330ms at 60fps) so a render that genuinely produces
      // an empty SVG doesn't busy-loop.
      void awaitSvgDimensionsThenFit();
    } catch (e) {
      const message = `Mermaid render error: ${e instanceof Error ? e.message : String(e)}`;
      diagramError = message;
      throw new Error(message);
    }
  }

  async function loadGraph() {
    if (graphLoaded) return;
    if (!derivatives?.exportgraph_url) return;

    graphLoading = true;
    graphError = null;
    try {
      exportGraph = await getDerivativeJSON<ExportGraph>(derivatives.exportgraph_url);
      graphLoaded = true;
    } catch (e) {
      graphError = e instanceof Error ? e.message : 'Failed to load export graph';
    } finally {
      graphLoading = false;
    }
  }

  function switchSubView(view: SubView) {
    subView = view;
    if (view === 'diagram' && !diagramLoaded && !diagramLoading) {
      loadDiagram();
    }
    if (view === 'graph' && !graphLoaded && !graphLoading) {
      loadGraph();
    }
    // rdf / shacl / sparql / x3ml / researchspace all live inside
    // CodeDerivativeView; they fetch lazily on URL change so no
    // explicit prime-load is needed here.
  }

  function switchRdfFormat(format: RdfFormat) {
    if (rdfFormat === format) return;
    rdfFormat = format;
    // CodeDerivativeView reactively re-fetches when the URL prop
    // changes, which it will via the `rdfFormat`-driven $derived.
  }

  // ─── SPARQL controls ────────────────────────────────────────────
  // Build the query-string variant of the base sparql URL. Limit is
  // skipped when count is on; the server omits LIMIT for COUNT
  // queries so the client mirrors that.
  const sparqlURL = $derived.by(() => {
    if (!derivatives?.sparql_url) return undefined;
    const params = new URLSearchParams();
    if (sparqlCount) {
      params.set('count', '1');
    } else if (sparqlLimit > 0 && sparqlLimit !== 100) {
      params.set('limit', String(sparqlLimit));
    }
    const qs = params.toString();
    if (!qs) return derivatives.sparql_url;
    const sep = derivatives.sparql_url.includes('?') ? '&' : '?';
    return `${derivatives.sparql_url}${sep}${qs}`;
  });

  function onSparqlLimitInput(e: Event) {
    const value = (e.target as HTMLInputElement).value;
    sparqlLimitInput = value;
    if (sparqlLimitTimer) clearTimeout(sparqlLimitTimer);
    sparqlLimitTimer = setTimeout(() => {
      const parsed = Number.parseInt(value, 10);
      if (Number.isFinite(parsed) && parsed >= 0) {
        sparqlLimit = parsed;
      }
    }, 300);
  }

  function onSparqlCountToggle(e: Event) {
    sparqlCount = (e.target as HTMLInputElement).checked;
  }

  // ─── X3ML form toggle ──────────────────────────────────────────
  function switchX3MLForm(form: X3MLForm) {
    if (x3mlForm === form) return;
    x3mlForm = form;
  }

  const x3mlURL = $derived(
    x3mlForm === 'a' ? derivatives?.x3ml_a_url : derivatives?.x3ml_b_url,
  );
  // ZIP bundle — mapping + each linked ontology's raw RDFS file.
  const x3mlZipURL = $derived(
    x3mlForm === 'a' ? derivatives?.x3ml_a_zip_url : derivatives?.x3ml_b_zip_url,
  );

  // ─── Download filename helpers ─────────────────────────────────
  // Stable stems for downloads so consumers get a sensible filename
  // (entity systemname is exposed via the entity meta block one level
  // up — DiagramTab itself only knows entityId, but using that as the
  // stem keeps filenames unique when copied across multiple entities).
  function downloadStem(): string {
    return entityId.replace(/[^A-Za-z0-9._-]+/g, '_');
  }

  function switchDiagramMode(mode: DiagramMode) {
    if (diagramMode === mode) return;
    diagramMode = mode;
    diagramLoaded = false;
    mermaidSource = '';
    svgContent = '';
    if (subView === 'diagram' && !diagramLoading) {
      loadDiagram();
    }
  }

  // awaitSvgDimensionsThenFit polls until mermaid's SVG has measurable
  // intrinsic dimensions, then captures + fits. Mermaid sets viewBox
  // synchronously inside render(), but the SVG isn't yet attached to
  // the DOM when the await resolves; capture reads 0/0 on the very
  // first tick and applyZoom bails, leaving the user to click Fit
  // manually.
  //
  // setTimeout (not requestAnimationFrame) because hidden tabs throttle
  // RAF to ~0Hz — if the user backgrounds the tab during load the
  // diagram never auto-fits and they're stuck at mermaid's intrinsic
  // size. setTimeout fires (even if throttled to ~1s on hidden tabs)
  // so the eventual return-to-tab still finds a fitted diagram.
  //
  // Cap at 20 ticks × 50ms = 1s so a genuinely empty render doesn't
  // busy-loop. On give-up the SVG is still visible at intrinsic size;
  // zoom + Fit buttons stay functional.
  async function awaitSvgDimensionsThenFit() {
    for (let i = 0; i < 20; i++) {
      await new Promise<void>((resolve) => setTimeout(resolve, 50));
      if (!diagramContainer) return;
      const svg = diagramContainer.querySelector<SVGSVGElement>('svg');
      if (!svg) continue;
      const vb = svg.viewBox?.baseVal;
      if (vb && vb.width > 0 && vb.height > 0) {
        captureSvgBaseDimensions();
        applyZoom();
        fitToView();
        return;
      }
    }
  }

  // Capture mermaid's natural SVG dimensions. Mermaid output sometimes
  // has explicit width/height attributes and sometimes relies on
  // viewBox + max-width:100% — handle both.
  function captureSvgBaseDimensions() {
    if (!diagramContainer) return;
    const svg = diagramContainer.querySelector('svg');
    if (!svg) return;
    // Prefer viewBox (intrinsic, unaffected by previous CSS sizing).
    const vb = svg.viewBox?.baseVal;
    if (vb && vb.width > 0 && vb.height > 0) {
      svgBaseWidth = vb.width;
      svgBaseHeight = vb.height;
      return;
    }
    // Fallback: clear inline sizing then read getBBox.
    svg.style.width = '';
    svg.style.height = '';
    const bb = svg.getBoundingClientRect();
    if (bb.width > 0 && bb.height > 0) {
      svgBaseWidth = bb.width;
      svgBaseHeight = bb.height;
    }
  }

  // Set the SVG element's CSS width/height to baseDims * zoom%. Layout
  // grows accordingly and overflow-auto on the container gives proper
  // both-axis scrollbars (the original transform: scale approach kept
  // the layout box at 100%).
  function applyZoom() {
    if (!diagramContainer) return;
    const svg = diagramContainer.querySelector<SVGSVGElement>('svg');
    if (!svg) return;
    if (svgBaseWidth <= 0 || svgBaseHeight <= 0) {
      captureSvgBaseDimensions();
      if (svgBaseWidth <= 0 || svgBaseHeight <= 0) return;
    }
    const factor = zoom / 100;
    svg.style.width = `${svgBaseWidth * factor}px`;
    svg.style.height = `${svgBaseHeight * factor}px`;
    svg.style.maxWidth = 'none';
  }

  // Zoom while keeping the world point under (clientX, clientY) fixed
  // relative to the viewport. Adjusts scrollLeft/scrollTop to compensate
  // for the new SVG dimensions. Used by both the toolbar buttons (anchor
  // = container center) and wheel zoom (anchor = cursor).
  function zoomAt(newZoom: number, clientX: number, clientY: number) {
    if (!diagramContainer) {
      zoom = clamp(newZoom, MIN_ZOOM, MAX_ZOOM);
      return;
    }
    const oldZoom = zoom;
    const next = clamp(newZoom, MIN_ZOOM, MAX_ZOOM);
    if (next === oldZoom) return;

    const rect = diagramContainer.getBoundingClientRect();
    const offsetX = clientX - rect.left;
    const offsetY = clientY - rect.top;
    // World coordinates of the cursor at the current zoom level.
    const worldX = (diagramContainer.scrollLeft + offsetX) / (oldZoom / 100);
    const worldY = (diagramContainer.scrollTop + offsetY) / (oldZoom / 100);

    zoom = next;
    applyZoom();

    // After zoom, scroll so the same world point sits under the cursor.
    diagramContainer.scrollLeft = worldX * (next / 100) - offsetX;
    diagramContainer.scrollTop = worldY * (next / 100) - offsetY;
  }

  function clamp(v: number, lo: number, hi: number): number {
    return Math.max(lo, Math.min(hi, v));
  }

  function containerCenter(): { x: number; y: number } {
    if (!diagramContainer) return { x: 0, y: 0 };
    const rect = diagramContainer.getBoundingClientRect();
    return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
  }

  function zoomIn() {
    const c = containerCenter();
    zoomAt(zoom + ZOOM_STEP, c.x, c.y);
  }

  function zoomOut() {
    const c = containerCenter();
    zoomAt(zoom - ZOOM_STEP, c.x, c.y);
  }

  function resetZoom() {
    zoom = 100;
    applyZoom();
    if (diagramContainer) {
      diagramContainer.scrollLeft = 0;
      diagramContainer.scrollTop = 0;
    }
  }

  function fitToView() {
    if (!diagramContainer) return;
    if (svgBaseWidth <= 0 || svgBaseHeight <= 0) captureSvgBaseDimensions();
    if (svgBaseWidth <= 0 || svgBaseHeight <= 0) return;
    const padding = 32;
    const containerWidth = diagramContainer.clientWidth - padding;
    const containerHeight = diagramContainer.clientHeight - padding;
    const fitW = (containerWidth / svgBaseWidth) * 100;
    const fitH = (containerHeight / svgBaseHeight) * 100;
    const fit = Math.min(fitW, fitH);
    zoom = clamp(Math.round(fit), MIN_ZOOM, MAX_ZOOM);
    applyZoom();
    if (diagramContainer) {
      diagramContainer.scrollLeft = 0;
      diagramContainer.scrollTop = 0;
    }
  }

  // Ctrl+wheel zoom (cursor-anchored). Plain wheel scrolls the
  // container as normal — keeps page-scroll behaviour predictable when
  // a user has the mouse over a diagram while reading down the page.
  function handleWheel(e: WheelEvent) {
    if (!e.ctrlKey && !e.metaKey) return;
    e.preventDefault();
    const direction = e.deltaY < 0 ? 1 : -1;
    zoomAt(zoom + direction * WHEEL_ZOOM_STEP, e.clientX, e.clientY);
  }

  // Drag-to-pan via scrollLeft/scrollTop. Engaging on plain mousedown
  // (left button) is fine because clicking the SVG nodes doesn't have
  // its own behaviour today — if/when node click-handlers land, gate
  // this on middle-click or modifier.
  function handlePanStart(e: MouseEvent) {
    if (e.button !== 0) return;
    if (!diagramContainer) return;
    isPanning = true;
    panStartX = e.clientX;
    panStartY = e.clientY;
    panStartScrollLeft = diagramContainer.scrollLeft;
    panStartScrollTop = diagramContainer.scrollTop;
    diagramContainer.style.cursor = 'grabbing';
    e.preventDefault();
  }

  function handlePanMove(e: MouseEvent) {
    if (!isPanning || !diagramContainer) return;
    diagramContainer.scrollLeft = panStartScrollLeft - (e.clientX - panStartX);
    diagramContainer.scrollTop = panStartScrollTop - (e.clientY - panStartY);
  }

  function handlePanEnd() {
    if (!isPanning) return;
    isPanning = false;
    if (diagramContainer) diagramContainer.style.cursor = '';
  }

  function downloadSvg() {
    if (!svgContent) return;
    const blob = new Blob([svgContent], { type: 'image/svg+xml' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${entityType}-${entityId}-diagram.svg`;
    a.click();
    URL.revokeObjectURL(url);
  }

  // Copy / download for derivative formats lives inside
  // CodeDerivativeView. Copy buffers, Shiki output, and per-URL
  // caches all stay scoped to that component so DiagramTab no longer
  // needs the hand-rolled highlighters or per-format clipboard
  // bookkeeping.

  // Load diagram on mount
  $effect(() => {
    loadDiagram();
  });
</script>

<div class="space-y-4">
  <!-- Sub-view toggle -->
  <div class="flex items-center space-x-1 bg-gray-100 rounded-lg p-1 w-fit">
    <button
      type="button"
      class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'diagram' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
      onclick={() => switchSubView('diagram')}
    >
      Ontology Diagram
    </button>
    {#if derivatives?.exportgraph_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'graph' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('graph')}
      >
        Graph
      </button>
    {/if}
    <button
      type="button"
      class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'rdf' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
      onclick={() => switchSubView('rdf')}
    >
      RDF Source
    </button>
    {#if derivatives?.shacl_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'shacl' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('shacl')}
      >
        SHACL
      </button>
    {/if}
    {#if derivatives?.sparql_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'sparql' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('sparql')}
      >
        SPARQL
      </button>
    {/if}
    {#if derivatives?.x3ml_a_url || derivatives?.x3ml_b_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'x3ml' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('x3ml')}
      >
        X3ML
      </button>
    {/if}
    {#if derivatives?.researchspace_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'researchspace' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('researchspace')}
      >
        ResearchSpace
      </button>
    {/if}
    {#if derivatives?.arches_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'arches' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('arches')}
        title="Super-admin only: Arches 7.6 Resource Graph export"
      >
        Arches
      </button>
    {/if}
    {#if derivatives?.snapshot_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'snapshot' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('snapshot')}
        title="Super-admin only: renderer-neutral Snapshot (tree + path_node ids)"
      >
        Snapshot
      </button>
    {/if}
    {#if derivatives?.ascii_tree_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'ascii_tree' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('ascii_tree')}
        title="Super-admin only: ASCII tree of the Snapshot for curator review"
      >
        Tree
      </button>
    {/if}
    {#if derivatives?.csv_url}
      <button
        type="button"
        class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {subView === 'csv' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
        onclick={() => switchSubView('csv')}
      >
        CSV
      </button>
    {/if}
  </div>

  {#if !supportsEntity}
    <!-- Unsupported entity type -->
    <div class="flex items-center justify-center h-64 bg-gray-50 border border-gray-200 rounded-lg">
      <div class="text-center">
        <svg class="mx-auto h-10 w-10 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 9.75l4.5 4.5m0-4.5l-4.5 4.5M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <p class="mt-2 text-sm text-gray-500">Diagram not available for {entityType}s</p>
      </div>
    </div>
  {:else if subView === 'diagram'}
    <!-- Diagram panel -->
    <div class="bg-white border border-gray-200 rounded-lg overflow-hidden">
      <!-- Diagram toolbar -->
      <div class="flex items-center justify-between px-4 py-2 border-b border-gray-200 bg-gray-50">
        <div class="flex items-center gap-3">
          <span class="text-xs font-medium text-gray-500">
            {diagramMode === 'ontology' ? 'Ontology Diagram' : 'Instance Diagram'}
          </span>
          <div class="flex items-center space-x-1 bg-white border border-gray-200 rounded-md p-0.5">
            <button
              type="button"
              class="px-2 py-1 text-xs rounded {diagramMode === 'ontology' ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-100'}"
              onclick={() => switchDiagramMode('ontology')}
            >Ontology</button>
            <button
              type="button"
              class="px-2 py-1 text-xs rounded {diagramMode === 'instance' ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-100'}"
              onclick={() => switchDiagramMode('instance')}
            >Instance</button>
          </div>
        </div>
        <div class="flex items-center space-x-2">
          <button
            type="button"
            class="px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded disabled:opacity-40"
            onclick={zoomOut}
            disabled={zoom <= MIN_ZOOM}
          >-</button>
          <span class="text-xs text-gray-500 w-10 text-center">{zoom}%</span>
          <button
            type="button"
            class="px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded disabled:opacity-40"
            onclick={zoomIn}
            disabled={zoom >= MAX_ZOOM}
          >+</button>
          <button
            type="button"
            class="px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded"
            onclick={resetZoom}
          >Reset</button>
          <button
            type="button"
            class="px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded disabled:opacity-40"
            onclick={fitToView}
            disabled={!svgContent}
          >Fit</button>
          <div class="w-px h-4 bg-gray-300"></div>
          <button
            type="button"
            class="px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded disabled:opacity-40"
            onclick={downloadSvg}
            disabled={!svgContent}
          >
            <svg class="inline-block w-3.5 h-3.5 mr-1 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
            SVG
          </button>
        </div>
      </div>

      <!-- Diagram area. Drag-to-pan via scrollLeft/scrollTop;
           Ctrl+wheel zoom anchored on cursor; plain wheel scrolls the
           container natively. cursor: grab signals the drag affordance.
           60vh gives more room than the old fixed 500px while still
           leaving toolbar + below-the-fold context visible. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        role="application"
        tabindex="0"
        aria-label="Diagram viewport"
        class="h-[60vh] min-h-[400px] overflow-auto bg-white p-4 select-none"
        style="cursor: grab;"
        bind:this={diagramContainer}
        onwheel={handleWheel}
        onmousedown={handlePanStart}
        onmousemove={handlePanMove}
        onmouseup={handlePanEnd}
        onmouseleave={handlePanEnd}
      >
        {#if diagramLoading}
          <div class="flex items-center justify-center h-full">
            <div class="text-center">
              <svg class="animate-spin h-8 w-8 text-blue-500 mx-auto" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
              </svg>
              <p class="mt-2 text-sm text-gray-500">Loading diagram...</p>
            </div>
          </div>
        {:else if diagramError}
          <div class="flex items-center justify-center h-full">
            <div class="text-center">
              <svg class="mx-auto h-12 w-12 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <p class="mt-2 text-sm text-red-600">{diagramError}</p>
            </div>
          </div>
        {:else if svgContent}
          <!-- No CSS transform — applyZoom() resizes the SVG element
               itself so the layout box grows and overflow-auto on the
               container produces both scrollbars. inline-block lets
               the SVG dictate its own width/height inside the
               scrollable region without a flex parent forcing centring
               that the old transform: scale tripped over. -->
          <div class="inline-block">
            {@html svgContent}
          </div>
        {:else}
          <div class="flex items-center justify-center h-full">
            <p class="text-sm text-gray-400">No diagram available</p>
          </div>
        {/if}
      </div>

      <!-- Mermaid source toggle -->
      {#if mermaidSource && !diagramLoading}
        <details class="border-t border-gray-200">
          <summary class="px-4 py-2 text-xs text-gray-500 cursor-pointer hover:bg-gray-50">
            View Mermaid source
          </summary>
          <pre class="px-4 py-2 text-xs bg-gray-50 overflow-x-auto max-h-32">{mermaidSource}</pre>
        </details>
      {/if}
    </div>

  {:else if subView === 'graph'}
    <div class="rounded-lg border border-gray-200 bg-white p-3">
      {#if graphLoading}
        <div class="flex h-[500px] items-center justify-center">
          <div class="text-center">
            <svg class="mx-auto h-8 w-8 animate-spin text-blue-500" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
            </svg>
            <p class="mt-2 text-sm text-gray-500">Loading export graph...</p>
          </div>
        </div>
      {:else if graphError}
        <div class="flex h-[500px] items-center justify-center">
          <div class="text-center">
            <svg class="mx-auto h-12 w-12 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
            <p class="mt-2 text-sm text-red-600">{graphError}</p>
          </div>
        </div>
      {:else if exportGraph}
        <ExportGraphExplorer graph={exportGraph} />
      {:else}
        <div class="flex h-[500px] items-center justify-center">
          <p class="text-sm text-gray-400">No export graph available</p>
        </div>
      {/if}
    </div>

  {:else if subView === 'rdf'}
    <CodeDerivativeView
      url={rdfFormat === 'turtle' ? derivatives?.turtle_url : derivatives?.jsonld_url}
      lang={rdfFormat === 'turtle' ? 'turtle' : 'json'}
      filename="{downloadStem()}.{rdfFormat === 'turtle' ? 'ttl' : 'jsonld'}"
      headerLabel="RDF Source"
      errorLabel="RDF source"
      emptyLabel="No RDF source available"
    >
      {#snippet controls()}
        <div class="flex items-center space-x-1 bg-white border border-gray-200 rounded-md p-0.5">
          <button
            type="button"
            class="px-2 py-1 text-xs rounded {rdfFormat === 'turtle' ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-100'}"
            onclick={() => switchRdfFormat('turtle')}
          >Turtle</button>
          <button
            type="button"
            class="px-2 py-1 text-xs rounded {rdfFormat === 'jsonld' ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-100'}"
            onclick={() => switchRdfFormat('jsonld')}
          >JSON-LD</button>
        </div>
      {/snippet}
    </CodeDerivativeView>

  {:else if subView === 'sparql'}
    <CodeDerivativeView
      url={sparqlURL}
      lang="sparql"
      filename="{downloadStem()}{sparqlCount ? '_count' : ''}.rq"
      headerLabel={sparqlCount ? 'SPARQL (count)' : 'SPARQL'}
      errorLabel="SPARQL"
      emptyLabel="No SPARQL query available"
      helpText="Each field's SELECT runs as its own statement; the full
        document is one query per field separated by a blank line."
    >
      {#snippet controls()}
        <div class="flex items-center gap-3">
          <label class="flex items-center gap-1 text-xs text-gray-600">
            <span>Limit</span>
            <input
              type="number"
              min="0"
              step="10"
              value={sparqlLimitInput}
              oninput={onSparqlLimitInput}
              disabled={sparqlCount}
              class="w-20 rounded border border-gray-300 px-2 py-0.5 text-xs disabled:bg-gray-100 disabled:text-gray-400"
            />
          </label>
          <label class="flex items-center gap-1 text-xs text-gray-600">
            <input
              type="checkbox"
              checked={sparqlCount}
              onchange={onSparqlCountToggle}
              class="rounded border-gray-300"
            />
            Count results only
          </label>
        </div>
      {/snippet}
    </CodeDerivativeView>

  {:else if subView === 'x3ml'}
    <CodeDerivativeView
      url={x3mlURL}
      lang="xml"
      filename="{downloadStem()}.{x3mlForm}.x3ml"
      headerLabel={x3mlForm === 'a' ? 'X3ML — Form A' : 'X3ML — Form B'}
      errorLabel="X3ML"
      emptyLabel="No X3ML mapping available"
      helpText={x3mlForm === 'a'
        ? 'Standalone mapping document — single <mapping> with full-path links.'
        : 'Spine + tail mappings — primary <mapping> on the root with first-property links, secondary <mapping>s grouped by spine class.'}
    >
      {#snippet controls()}
        <div class="flex items-center space-x-2">
          <div class="flex items-center space-x-1 bg-white border border-gray-200 rounded-md p-0.5">
            <button
              type="button"
              class="px-2 py-1 text-xs rounded {x3mlForm === 'a' ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-100'}"
              onclick={() => switchX3MLForm('a')}
              disabled={!derivatives?.x3ml_a_url}
            >Form A</button>
            <button
              type="button"
              class="px-2 py-1 text-xs rounded {x3mlForm === 'b' ? 'bg-gray-900 text-white' : 'text-gray-600 hover:bg-gray-100'}"
              onclick={() => switchX3MLForm('b')}
              disabled={!derivatives?.x3ml_b_url}
            >Form B</button>
          </div>
          {#if x3mlZipURL}
            <a
              href={x3mlZipURL}
              download
              class="px-2 py-1 text-xs rounded border border-gray-200 bg-white text-gray-700 hover:bg-gray-100"
              title="Download the mapping plus each linked ontology's RDFS schema file as a ZIP"
            >Download ZIP</a>
          {/if}
          {#if derivatives?.integration_actions && derivatives.integration_actions.length > 0}
            <button
              type="button"
              onclick={() => (integrationsOpen = !integrationsOpen)}
              aria-expanded={integrationsOpen}
              class="px-2 py-1 text-xs rounded border border-gray-200 bg-white text-gray-700 hover:bg-gray-100 inline-flex items-center gap-1"
              title="Per-project integration actions for this mapping"
            >
              Integrations
              <span class="text-gray-400">{integrationsOpen ? '▴' : '▾'}</span>
            </button>
          {/if}
        </div>
      {/snippet}
      {#snippet belowHeader()}
        {#if integrationsOpen && derivatives?.integration_actions && derivatives.integration_actions.length > 0}
          <div class="border-b border-gray-200 bg-gray-50 px-4 py-3 space-y-3">
            {#each derivatives.integration_actions as group (group.id)}
              <ActionGroup {group} />
            {/each}
          </div>
        {/if}
      {/snippet}
    </CodeDerivativeView>

  {:else if subView === 'researchspace'}
    <CodeDerivativeView
      url={derivatives?.researchspace_url}
      lang="yaml"
      filename="{downloadStem()}.yml"
      headerLabel="ResearchSpace"
      errorLabel="ResearchSpace YAML"
      emptyLabel="No ResearchSpace config available"
      helpText="YAML config with one embedded SPARQL SELECT per resolved field."
    />

  {:else if subView === 'arches'}
    <CodeDerivativeView
      url={derivatives?.arches_url}
      lang="json"
      filename="{downloadStem()}.json"
      headerLabel="Arches Resource Graph"
      errorLabel="Arches export"
      emptyLabel="No Arches export available"
      helpText="Arches 7.6 Resource Graph JSON for this model. Super-admin preview while the contract is in development."
    />

  {:else if subView === 'snapshot'}
    <CodeDerivativeView
      url={derivatives?.snapshot_url}
      lang="json"
      filename="{downloadStem()}.snapshot.json"
      headerLabel="Generator Snapshot"
      errorLabel="Snapshot"
      emptyLabel="No snapshot available"
      helpText="The renderer-neutral Snapshot every generator consumes — the tree plus each path element's generated path_node and path_node_id. Super-admin verification view."
    />

  {:else if subView === 'ascii_tree'}
    <CodeDerivativeView
      url={derivatives?.ascii_tree_url}
      lang="plaintext"
      filename="{downloadStem()}.tree.txt"
      headerLabel="ASCII Tree"
      errorLabel="ASCII tree"
      emptyLabel="No ASCII tree available"
      helpText="Human-readable ASCII tree of the Snapshot keyed by PathNodeID/InstanceID. One line per semantic intermediate, one line per leaf field — curator review surface for how the generator names every node."
    />

  {:else if subView === 'csv'}
    <!-- CSV download panel -->
    <div class="bg-white border border-gray-200 rounded-lg p-6">
      <div class="flex items-start gap-4">
        <div class="rounded-lg bg-emerald-50 p-3">
          <svg class="h-6 w-6 text-emerald-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
        </div>
        <div class="flex-1">
          <h3 class="text-sm font-semibold text-gray-900">Download as CSV</h3>
          <p class="mt-1 text-sm text-gray-600">
            One-row-per-field CSV export of this {entityType}, including the
            ontology path elements and the structured ontology scope. Same
            shape the project verification dump uses, restricted to this
            single entity.
          </p>
          <a
            href={derivatives?.csv_url ?? ''}
            class="mt-4 inline-flex items-center gap-2 px-3 py-2 text-sm font-medium text-white bg-pletka-primary hover:bg-pletka-secondary rounded-md focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
            Download CSV
          </a>
        </div>
      </div>
    </div>

  {:else}
    <CodeDerivativeView
      url={derivatives?.shacl_url}
      lang="turtle"
      filename="{downloadStem()}.shacl.ttl"
      headerLabel="SHACL Shapes (Turtle)"
      errorLabel="SHACL"
      emptyLabel="No SHACL shapes available"
    />
  {/if}
</div>
