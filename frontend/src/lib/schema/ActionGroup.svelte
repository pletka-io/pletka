<script lang="ts">
  /**
   * Schema-driven action with multiple targets. When the action's
   * targets array has length 1, renders as a plain ActionButton (no
   * dropdown noise on single-config projects). When > 1, renders a
   * target select + single action button — operator picks the
   * destination explicitly before clicking, eliminating the
   * "which button does what?" problem when many configs exist.
   *
   * Domain-agnostic. Reads the endpoint URL straight from the chosen
   * target — no URL construction in the frontend.
   */
  import ActionButton from './ActionButton.svelte';
  import type { ActionGroupSchema, ActionResultUI, ActionSchema } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import { confirmAction } from '$lib/stores/confirm';

  let { group, lang = 'en' }: { group: ActionGroupSchema; lang?: string } = $props();

  // Selected target index — defaults to the first target. Re-syncs
  // whenever the group's target list changes (e.g. operator enables a
  // new config and the list-schema refetches).
  let selectedIdx = $state(0);
  $effect(() => {
    if (selectedIdx >= group.targets.length) selectedIdx = 0;
  });

  let busy = $state(false);
  let result = $state<ActionResultUI | null>(null);
  let errorMessage = $state('');

  const isDanger = $derived(group.theme === 'danger');
  const labelText = $derived(tr(group.label, lang));
  const helpText = $derived(group.help ? tr(group.help, lang) : '');
  const selectedTarget = $derived(group.targets[selectedIdx]);
  const targetID = $derived(`action-group-${safePart(group.id)}-target`);

  // Build a synthetic single-target ActionSchema for the
  // ActionButton fallback path.
  const singleAction = $derived<ActionSchema>({
    kind: 'action',
    id: group.id,
    label: group.label,
    help: group.help,
    theme: group.theme,
    endpoint: group.targets[0]?.endpoint ?? { method: 'POST', url: '' },
  });

  async function run() {
    if (!selectedTarget || busy) return;
    const ep = selectedTarget.endpoint;
    if (ep.confirm) {
      const c = ep.confirm;
      const ok = await confirmAction({
        title: tr(c.title, lang),
        message: tr(c.message, lang),
        confirmLabel: tr(c.confirm_label, lang),
        danger: isDanger,
      });
      if (!ok) return;
    }
    busy = true;
    result = null;
    errorMessage = '';
    try {
      const res = await fetch(ep.url, {
        method: ep.method || 'POST',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/json',
          'X-Requested-With': 'XMLHttpRequest',
        },
      });
      const body: ActionResultUI | { error?: string } = await res.json().catch(() => ({}));
      if (!res.ok) {
        errorMessage = (body as any)?.error || `Request failed (${res.status})`;
        return;
      }
      result = body as ActionResultUI;
    } catch (e: any) {
      errorMessage = e?.message ?? 'Network error';
    } finally {
      busy = false;
    }
  }

  function safePart(value: string): string {
    return value
      .trim()
      .replace(/[^a-zA-Z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'target';
  }
</script>

{#if group.targets.length <= 1}
  <ActionButton action={singleAction} {lang} />
{:else}
  <div class="space-y-2">
    <div class="flex items-center gap-2">
      <label for={targetID} class="text-xs font-medium text-gray-600">
        Target
        <select
          id={targetID}
          name={targetID}
          bind:value={selectedIdx}
          title={selectedTarget?.label ?? ''}
          class="ml-1 max-w-[16rem] truncate rounded border border-gray-300 bg-white px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-pletka-primary"
        >
          {#each group.targets as t, i (t.id)}
            <option value={i}>{t.label || t.id}</option>
          {/each}
        </select>
      </label>
      <button
        type="button"
        onclick={run}
        disabled={busy}
        title={helpText}
        class="inline-flex items-center gap-2 rounded px-3 py-1.5 text-sm font-medium text-white shadow-sm focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 {isDanger
          ? 'bg-red-600 hover:bg-red-700 focus:ring-red-600'
          : 'bg-pletka-primary hover:bg-pletka-secondary focus:ring-pletka-primary'}"
      >
        {#if busy}
          <span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
        {/if}
        {labelText}
      </button>
    </div>

    {#if helpText}
      <p class="text-xs text-gray-500">{helpText}</p>
    {/if}

    {#if result && result.status === 'success'}
      <div class="rounded border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-900">
        <p>{tr(result.message, lang)}</p>
        {#if result.link_url}
          <a
            href={result.link_url}
            target="_blank"
            rel="noopener noreferrer"
            class="mt-1 inline-block text-pletka-primary underline hover:text-pletka-secondary"
          >
            {result.link_label ? tr(result.link_label, lang) : result.link_url}
          </a>
        {/if}
      </div>
    {:else if result && result.status === 'error'}
      <div class="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-900">
        {tr(result.message, lang)}
      </div>
    {/if}

    {#if errorMessage}
      <div class="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-900">
        {errorMessage}
      </div>
    {/if}
  </div>
{/if}
