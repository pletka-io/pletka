<script lang="ts">
  /**
   * Footer build-identity pill. Fetches /version, renders a compact
   * monospace label, copies a markdown-formatted build report to the
   * clipboard on click. The copied block is paste-ready into a GitHub
   * or Linear issue so user-test bug reports come with a bisect
   * fingerprint attached.
   */
  import { onMount } from 'svelte';
  import { addToast } from '$lib/stores/toast';

  interface VersionPayload {
    version: string;
    commit: string;
    branch?: string;
    dirty: boolean;
    built_at: string;
    migration: number;
    frontend: string;
    instance?: string;
    go?: string;
    reported_at: string;
  }

  let info = $state<VersionPayload | null>(null);
  let loading = $state(true);
  let copied = $state(false);

  onMount(async () => {
    try {
      const res = await fetch('/version', { credentials: 'same-origin' });
      if (!res.ok) {
        loading = false;
        return;
      }
      info = await res.json() as VersionPayload;
    } catch {
      // swallow — failing to load build info should never break the footer
    } finally {
      loading = false;
    }
  });

  function shortLabel(v: VersionPayload): string {
    const commit = v.commit + (v.dirty ? '.dirty' : '');
    return `${v.version} · ${commit}`;
  }

  function clipboardText(v: VersionPayload): string {
    const reportedAt = new Date().toISOString();
    const lines = [
      '```',
      'Pletka build info',
      `Version:   ${v.version}`,
      `Commit:    ${v.commit}${v.dirty ? ' (dirty)' : ''}` + (v.branch ? ` on ${v.branch}` : ''),
      `Built:     ${v.built_at}`,
      `Migration: ${v.migration}`,
      `Frontend:  ${v.frontend}`,
    ];
    if (v.instance) lines.push(`Instance:  ${v.instance}`);
    if (v.go) lines.push(`Go:        ${v.go}`);
    lines.push(`Reported:  ${reportedAt}`);
    lines.push('```');
    return lines.join('\n');
  }

  async function copy(): Promise<void> {
    if (!info) return;
    try {
      await navigator.clipboard.writeText(clipboardText(info));
      copied = true;
      addToast('success', 'Build info copied — paste into the issue', 3000);
      setTimeout(() => { copied = false; }, 2000);
    } catch {
      addToast('error', 'Could not copy to clipboard');
    }
  }
</script>

{#if !loading && info}
  <button
    type="button"
    class="build-badge"
    onclick={copy}
    title="Click to copy build info — paste into bug reports for bisect"
  >
    <span class="dot" class:dot-dirty={info.dirty}></span>
    <span class="text">{shortLabel(info)}</span>
    {#if copied}
      <svg viewBox="0 0 20 20" fill="currentColor" class="icon">
        <path fill-rule="evenodd" d="M16.704 5.29a1 1 0 010 1.42l-7.5 7.5a1 1 0 01-1.42 0l-3.5-3.5a1 1 0 111.42-1.42l2.79 2.79 6.79-6.79a1 1 0 011.42 0z" clip-rule="evenodd"/>
      </svg>
    {:else}
      <svg viewBox="0 0 20 20" fill="currentColor" class="icon">
        <path d="M8 2a2 2 0 00-2 2v8a2 2 0 002 2h6a2 2 0 002-2V6.414A2 2 0 0015.414 5L13 2.586A2 2 0 0011.586 2H8z"/>
        <path d="M4 6a2 2 0 00-2 2v8a2 2 0 002 2h6a2 2 0 002-2v-1H8a3 3 0 01-3-3V6H4z"/>
      </svg>
    {/if}
  </button>
{/if}

<style>
  .build-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.5rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.7rem;
    line-height: 1;
    color: #6b7280;
    background: #f3f4f6;
    border: 1px solid #e5e7eb;
    border-radius: 9999px;
    cursor: pointer;
    transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
  }
  .build-badge:hover {
    background: #e5e7eb;
    border-color: #d1d5db;
    color: #374151;
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 9999px;
    background: #10b981;
    flex-shrink: 0;
  }
  .dot-dirty {
    background: #f59e0b;
  }
  .icon {
    width: 0.85rem;
    height: 0.85rem;
    opacity: 0.7;
  }
  .text {
    white-space: nowrap;
  }
</style>
