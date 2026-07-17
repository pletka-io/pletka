<script lang="ts">
  /**
   * Generic schema-driven action button. Renders an ActionSchema:
   * a single button that POSTs to a server endpoint and shows the
   * returned ActionResultUI as a success banner (with optional link)
   * or an inline error.
   *
   * Domain-agnostic. No integration-specific logic — the schema tells
   * the component what to label the button, where to POST, whether to
   * confirm first, and how to render the result. Used by the
   * integrations hub for actions like "Upload to 3M", but reusable
   * anywhere a schema-driven action button is needed.
   */
  import type { ActionSchema, ActionResultUI } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import { confirmAction } from '$lib/stores/confirm';

  let { action, lang = 'en' }: { action: ActionSchema; lang?: string } = $props();

  let busy = $state(false);
  let result = $state<ActionResultUI | null>(null);
  let errorMessage = $state('');

  const isDanger = $derived(action.theme === 'danger');
  const labelText = $derived(tr(action.label, lang));
  const helpText = $derived(action.help ? tr(action.help, lang) : '');
  const disabledReason = $derived(
    action.disabled && action.disabled_reason ? tr(action.disabled_reason, lang) : ''
  );

  async function run() {
    if (action.disabled || busy) return;
    if (action.endpoint.confirm) {
      const c = action.endpoint.confirm;
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
      const res = await fetch(action.endpoint.url, {
        method: action.endpoint.method || 'POST',
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
</script>

<div class="space-y-2">
  <button
    type="button"
    onclick={run}
    disabled={action.disabled || busy}
    title={disabledReason || helpText}
    class="inline-flex items-center gap-2 rounded px-3 py-1.5 text-sm font-medium text-white shadow-sm focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 {isDanger
      ? 'bg-red-600 hover:bg-red-700 focus:ring-red-600'
      : 'bg-pletka-primary hover:bg-pletka-secondary focus:ring-pletka-primary'}"
  >
    {#if busy}
      <span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
    {/if}
    {labelText}
  </button>

  {#if action.disabled && disabledReason}
    <p class="text-xs text-gray-500">{disabledReason}</p>
  {:else if helpText}
    <p class="text-xs text-gray-500">{helpText}</p>
  {/if}

  {#if result && result.status === 'success'}
    <div class="rounded border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-900 flex items-start justify-between gap-3">
      <div class="min-w-0">
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
      <button
        type="button"
        aria-label="Dismiss"
        onclick={() => (result = null)}
        class="shrink-0 text-green-700 hover:text-green-900 font-bold leading-none px-1"
      >×</button>
    </div>
  {:else if result && result.status === 'error'}
    <div class="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-900 flex items-start justify-between gap-3">
      <div class="min-w-0">{tr(result.message, lang)}</div>
      <button
        type="button"
        aria-label="Dismiss"
        onclick={() => (result = null)}
        class="shrink-0 text-red-700 hover:text-red-900 font-bold leading-none px-1"
      >×</button>
    </div>
  {/if}

  {#if errorMessage}
    <div class="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-900 flex items-start justify-between gap-3">
      <div class="min-w-0">{errorMessage}</div>
      <button
        type="button"
        aria-label="Dismiss"
        onclick={() => (errorMessage = '')}
        class="shrink-0 text-red-700 hover:text-red-900 font-bold leading-none px-1"
      >×</button>
    </div>
  {/if}
</div>
