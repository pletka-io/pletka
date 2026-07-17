<script lang="ts">
  import { Background, Controls, MiniMap, SvelteFlow, type Edge, type Node } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';

  import type { ExportGraph, FieldBinding, PathElement, ResourceNode, VisualGroup } from '../export-graph';

  let { graph }: { graph: ExportGraph } = $props();

  let selectedKey = $state<string | null>(null);
  let hiddenGroupKeys = $state<Set<string>>(new Set());
  let fullscreen = $state(false);

  const graphNodes = $derived(graph.nodes ?? []);
  const graphGroups = $derived(graph.groups ?? []);
  const graphFields = $derived(graph.fields ?? []);
  const selectedNode = $derived(graphNodes.find((node) => node.key === selectedKey) ?? null);
  const selectedGroup = $derived(graphGroups.find((group) => group.key === selectedKey) ?? null);

  const visibleNodeKeys = $derived(computeVisibleNodeKeys(graph, hiddenGroupKeys));
  const flowNodes = $derived(toFlowNodes(graph, visibleNodeKeys, selectedKey));
  const flowEdges = $derived(toFlowEdges(graph, visibleNodeKeys));

  function labelFromTranslations(value?: Record<string, string>, fallback = ''): string {
    return value?.en || Object.values(value ?? {})[0] || fallback;
  }

  function pathLabel(element?: PathElement): string {
    if (!element) return 'unknown';
    if (element.uri) return element.uri;
    if (element.prefix && element.local_name) return `${element.prefix}:${element.local_name}`;
    return element.local_name || 'unknown';
  }

  function shortLabel(element?: PathElement): string {
    const label = pathLabel(element);
    return label.replace(/^https?:\/\/[^/]+\/[^/]+\//, '').replace(/^crm:/, '');
  }

  function groupLabel(group: VisualGroup): string {
    return labelFromTranslations(group.label, group.semantic_id || group.system_name || group.id || group.key);
  }

  function computeHiddenNodeKeys(source: ExportGraph, collapsed: Set<string>): Set<string> {
    const edgesBySource = new Map<string, string[]>();
    for (const node of source.nodes ?? []) {
      edgesBySource.set(node.key, (node.edges ?? []).map((edge) => edge.target_key));
    }

    const hidden = new Set<string>();
    for (const group of source.groups ?? []) {
      if (collapsed.has(group.key) && group.branch_key) {
        hideBranch(group.branch_key, source.root_key, edgesBySource, hidden);
      }
    }
    return hidden;
  }

  function hideBranch(branchKey: string, rootKey: string, edgesBySource: Map<string, string[]>, hidden: Set<string>) {
    const pending = [branchKey];
    const seen = new Set<string>();
    while (pending.length) {
      const key = pending.pop();
      if (!key || seen.has(key)) continue;
      seen.add(key);
      if (key !== rootKey) {
        hidden.add(key);
      }
      for (const next of edgesBySource.get(key) ?? []) {
        if (!seen.has(next)) {
          pending.push(next);
        }
      }
    }
  }

  function computeVisibleNodeKeys(source: ExportGraph, collapsed: Set<string>): Set<string> {
    const hiddenBranches = computeHiddenNodeKeys(source, collapsed);
    const out = new Set<string>();
    for (const node of source.nodes ?? []) {
      if (node.key === source.root_key || !hiddenBranches.has(node.key)) {
        out.add(node.key);
      }
    }
    return out;
  }

  function depthForKey(key: string): number {
    return Math.max(0, Math.floor(key.split('/').length / 2));
  }

  function toFlowNodes(source: ExportGraph, visible: Set<string>, activeKey: string | null): Node[] {
    const ordered = (source.nodes ?? []).filter((node) => visible.has(node.key));
    const rowsByDepth = new Map<number, number>();
    return ordered.map((node) => {
      const depth = depthForKey(node.key);
      const row = rowsByDepth.get(depth) ?? 0;
      rowsByDepth.set(depth, row + 1);
      const fields = node.fields?.length ?? 0;
      const groups = node.groups?.map((group) => group.kind).join(', ');
      return {
        id: node.key,
        position: { x: depth * 310, y: row * 150 },
        data: {
          label: `${shortLabel(node.class)}${fields ? `\n${fields} field${fields === 1 ? '' : 's'}` : ''}${groups ? `\n${groups}` : ''}`,
        },
        style: [
          `border: ${node.key === activeKey ? '2px solid #111827' : '1px solid #cbd5e1'}`,
          'border-radius: 14px',
          `background: ${node.key === source.root_key ? '#fff7ed' : '#ffffff'}`,
          'color: #111827',
          'padding: 10px',
          'width: 210px',
          'white-space: pre-line',
          `box-shadow: ${node.key === activeKey ? '0 18px 40px rgba(15, 23, 42, 0.18)' : '0 8px 20px rgba(15, 23, 42, 0.08)'}`,
          'font-size: 12px',
        ].join('; '),
      };
    });
  }

  function toFlowEdges(source: ExportGraph, visible: Set<string>): Edge[] {
    const edges: Edge[] = [];
    for (const node of source.nodes ?? []) {
      if (!visible.has(node.key)) continue;
      for (const edge of node.edges ?? []) {
        if (!visible.has(edge.target_key)) continue;
        edges.push({
          id: `${node.key}->${edge.target_key}:${pathLabel(edge.predicate)}`,
          source: node.key,
          target: edge.target_key,
          label: shortLabel(edge.predicate),
          type: 'smoothstep',
          animated: false,
        });
      }
    }
    return edges;
  }

  function onNodeClick({ node }: { node: Node }) {
    selectedKey = node.id;
  }

  function selectGroup(key: string) {
    selectedKey = key;
  }

  function toggleGroup(key: string) {
    const next = new Set(hiddenGroupKeys);
    if (next.has(key)) {
      next.delete(key);
    } else {
      next.add(key);
    }
    hiddenGroupKeys = next;
  }

  function nodeFields(node: ResourceNode | null): FieldBinding[] {
    return node?.fields ?? [];
  }

  function fieldIDsForNode(node: ResourceNode | null): string[] {
    const ids = new Set<string>();
    for (const field of node?.fields ?? []) {
      ids.add(field.semantic_id || field.id);
    }
    for (const literal of node?.literals ?? []) {
      ids.add(literal.field.semantic_id || literal.field.id);
    }
    for (const edge of node?.edges ?? []) {
      for (const id of edge.field_ids ?? []) {
        ids.add(id);
      }
    }
    for (const sourcePath of node?.source_paths ?? []) {
      if (sourcePath.field_id) ids.add(sourcePath.field_id);
    }
    return Array.from(ids);
  }

  $effect(() => {
    if (selectedKey == null) {
      selectedKey = graph.root_key;
    }
  });
</script>

<div class={fullscreen ? 'fixed inset-0 z-50 bg-slate-950 p-4' : 'space-y-3'}>
  <div class="flex items-center justify-between gap-3 rounded-lg border border-slate-200 bg-white px-3 py-2 shadow-sm">
    <div>
      <div class="text-sm font-semibold text-slate-900">Export Graph</div>
      <div class="text-xs text-slate-500">{graphNodes.length} resources · {graphGroups.length} groups · {graphFields.length} fields</div>
    </div>
    <button
      type="button"
      class="rounded-md border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50"
      onclick={() => fullscreen = !fullscreen}
    >
      {fullscreen ? 'Exit fullscreen' : 'Fullscreen'}
    </button>
  </div>

  <div class="grid gap-3 lg:grid-cols-[220px_minmax(0,1fr)_280px]">
    <aside class="max-h-[640px] overflow-auto rounded-lg border border-slate-200 bg-white p-2 shadow-sm">
      <div class="px-2 pb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">Visual groups</div>
      {#each graphGroups as group}
        <div class="mb-1 flex items-center gap-1">
          <button
            type="button"
            class="h-7 w-7 rounded text-xs text-slate-500 hover:bg-slate-100"
            onclick={() => toggleGroup(group.key)}
            title="Collapse or expand group branch"
          >
            {hiddenGroupKeys.has(group.key) ? '+' : '-'}
          </button>
          <button
            type="button"
            class="min-w-0 flex-1 rounded px-2 py-1 text-left text-xs {selectedKey === group.key ? 'bg-slate-900 text-white' : 'text-slate-700 hover:bg-slate-100'}"
            onclick={() => selectGroup(group.key)}
          >
            <span class="block truncate font-medium">{groupLabel(group)}</span>
            <span class="block truncate opacity-70">{group.kind}</span>
          </button>
        </div>
      {/each}
    </aside>

    <section class="h-[640px] overflow-hidden rounded-lg border border-slate-200 bg-slate-50 shadow-sm">
      <SvelteFlow
        nodes={flowNodes}
        edges={flowEdges}
        fitView
        minZoom={0.15}
        maxZoom={2}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable
        onnodeclick={onNodeClick}
      >
        <Background />
        <Controls />
        <MiniMap pannable zoomable />
      </SvelteFlow>
    </section>

    <aside class="max-h-[640px] overflow-auto rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      {#if selectedNode}
        <div class="space-y-4">
          <div>
            <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Resource</div>
            <div class="mt-1 text-sm font-semibold text-slate-900">{pathLabel(selectedNode.class)}</div>
            <div class="mt-1 break-all text-xs text-slate-500">{selectedNode.key}</div>
          </div>
          <div class="grid grid-cols-2 gap-2 text-xs">
            <div class="rounded bg-slate-50 p-2">
              <div class="text-slate-400">Identity</div>
              <div class="font-medium text-slate-800">{selectedNode.identity_source || 'structural'}</div>
            </div>
            <div class="rounded bg-slate-50 p-2">
              <div class="text-slate-400">Edges</div>
              <div class="font-medium text-slate-800">{selectedNode.edges?.length ?? 0}</div>
            </div>
          </div>
          <div class="rounded bg-slate-50 p-2 text-xs">
            <div class="text-slate-400">Class URI</div>
            <div class="break-all font-medium text-slate-800">{pathLabel(selectedNode.class)}</div>
          </div>
          {#if selectedNode.instance_ids?.length}
            <div>
              <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Instance IDs</div>
              <div class="mt-1 flex flex-wrap gap-1">
                {#each selectedNode.instance_ids as id}
                  <span class="rounded bg-amber-100 px-2 py-0.5 text-xs text-amber-800">{id}</span>
                {/each}
              </div>
            </div>
          {/if}
          {#if selectedNode.groups?.length}
            <div>
              <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Group IDs</div>
              <div class="mt-1 flex flex-wrap gap-1">
                {#each selectedNode.groups as group}
                  <span class="rounded bg-slate-100 px-2 py-0.5 text-xs text-slate-700">{group.kind}:{group.key}</span>
                {/each}
              </div>
            </div>
          {/if}
          <div>
            <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Source Field IDs</div>
            <div class="mt-1 flex flex-wrap gap-1">
              {#each fieldIDsForNode(selectedNode) as id}
                <span class="rounded bg-blue-50 px-2 py-0.5 text-xs text-blue-700">{id}</span>
              {:else}
                <span class="text-xs text-slate-400">No source fields.</span>
              {/each}
            </div>
          </div>
          {#if selectedNode.source_paths?.length}
            <div>
              <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Source Paths</div>
              <div class="mt-2 space-y-1">
                {#each selectedNode.source_paths as sourcePath}
                  <div class="rounded border border-slate-100 bg-slate-50 px-2 py-1 text-xs">
                    <div class="font-medium text-slate-800">{sourcePath.field_id || 'unknown field'}</div>
                    <div class="break-all text-[11px] text-slate-500">{sourcePath.path || 'no path'}</div>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
          {#if selectedNode.additional_types?.length}
            <div>
              <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Additional Types</div>
              <div class="mt-1 flex flex-wrap gap-1">
                {#each selectedNode.additional_types as typeRef}
                  <span class="rounded bg-orange-50 px-2 py-0.5 text-xs text-orange-800">{typeRef.prefix && typeRef.local_name ? `${typeRef.prefix}:${typeRef.local_name}` : typeRef.uri}</span>
                {/each}
              </div>
            </div>
          {/if}
          <div>
            <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Fields</div>
            <div class="mt-2 space-y-1">
              {#each nodeFields(selectedNode) as field}
                <div class="rounded border border-slate-100 bg-slate-50 px-2 py-1">
                  <div class="truncate text-xs font-medium text-slate-800">{labelFromTranslations(field.label, field.semantic_id || field.id)}</div>
                  <div class="truncate text-[11px] text-slate-500">{field.semantic_id || field.id}</div>
                  {#if field.relative_path}
                    <div class="truncate text-[11px] text-slate-400">{field.relative_path}</div>
                  {/if}
                  {#if field.group_keys?.length}
                    <div class="mt-1 flex flex-wrap gap-1">
                      {#each field.group_keys as groupKey}
                        <span class="rounded bg-white px-1.5 py-0.5 text-[10px] text-slate-500">{groupKey}</span>
                      {/each}
                    </div>
                  {/if}
                </div>
              {:else}
                <div class="text-xs text-slate-400">No direct fields on this node.</div>
              {/each}
            </div>
          </div>
          {#if selectedNode.literals?.length}
            <div>
              <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Literals</div>
              <div class="mt-2 space-y-1">
                {#each selectedNode.literals as literal}
                  <div class="rounded border border-slate-100 bg-slate-50 px-2 py-1 text-xs">
                    <div class="break-all font-medium text-slate-800">{shortLabel(literal.predicate)}</div>
                    <div class="text-[11px] text-slate-500">{literal.field.semantic_id || literal.field.id}</div>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
        </div>
      {:else if selectedGroup}
        <div class="space-y-3">
          <div>
            <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Group</div>
            <div class="mt-1 text-sm font-semibold text-slate-900">{groupLabel(selectedGroup)}</div>
            <div class="mt-1 text-xs text-slate-500">{selectedGroup.kind}</div>
          </div>
          <div class="rounded bg-slate-50 p-2 text-xs">
            <div class="text-slate-400">Branch key</div>
            <div class="break-all font-medium text-slate-800">{selectedGroup.branch_key || 'root / presentation only'}</div>
          </div>
          <div class="rounded bg-slate-50 p-2 text-xs">
            <div class="text-slate-400">Group key</div>
            <div class="break-all font-medium text-slate-800">{selectedGroup.key}</div>
          </div>
          {#if selectedGroup.parent_key}
            <div class="rounded bg-slate-50 p-2 text-xs">
              <div class="text-slate-400">Parent key</div>
              <div class="break-all font-medium text-slate-800">{selectedGroup.parent_key}</div>
            </div>
          {/if}
          <div class="rounded bg-slate-50 p-2 text-xs">
            <div class="text-slate-400">Fields</div>
            <div class="font-medium text-slate-800">{selectedGroup.field_ids?.length ?? 0}</div>
          </div>
          {#if selectedGroup.field_ids?.length}
            <div>
              <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Field IDs</div>
              <div class="mt-1 flex flex-wrap gap-1">
                {#each selectedGroup.field_ids as id}
                  <span class="rounded bg-blue-50 px-2 py-0.5 text-xs text-blue-700">{id}</span>
                {/each}
              </div>
            </div>
          {/if}
          {#if selectedGroup.path_prefix?.length}
            <div>
              <div class="text-xs font-semibold uppercase tracking-wide text-slate-400">Path Prefix</div>
              <div class="mt-2 space-y-1">
                {#each selectedGroup.path_prefix as element}
                  <div class="break-all rounded bg-slate-50 px-2 py-1 text-xs text-slate-700">{pathLabel(element)}</div>
                {/each}
              </div>
            </div>
          {/if}
        </div>
      {:else}
        <div class="text-sm text-slate-400">Select a node or group.</div>
      {/if}
    </aside>
  </div>
</div>
