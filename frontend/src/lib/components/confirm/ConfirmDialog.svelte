<script lang="ts">
  import { confirmRequest, registerConfirmDialog, resolveConfirm } from '$lib/stores/confirm';

  // Subscribe via Svelte 5 $state + manual $effect so the singleton can
  // mount once and react to every confirmAction() call from anywhere
  // in the app — same lifecycle pattern as ToastContainer.
  //
  // The register/teardown pair lets the store detect "no dialog
  // mounted" callers and loud-fail instead of hanging the promise.
  let req = $state<{
    id: number;
    title: string;
    message: string;
    confirmLabel?: string;
    cancelLabel?: string;
    danger?: boolean;
    requireType?: string;
  } | null>(null);

  let typedValue = $state('');

  $effect(() => {
    const unregister = registerConfirmDialog();
    const unsub = confirmRequest.subscribe((value) => {
      req = value;
      typedValue = '';
    });
    return () => {
      unsub();
      unregister();
    };
  });

  let confirmDisabled = $derived(!!req?.requireType && typedValue !== req.requireType);

  function ok() {
    if (!req) return;
    if (confirmDisabled) return;
    resolveConfirm(req.id, true);
  }
  function cancel() {
    if (req) resolveConfirm(req.id, false);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!req) return;
    if (e.key === 'Escape') cancel();
    if (e.key === 'Enter' && !confirmDisabled) ok();
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if req}
  <!-- Backdrop -->
  <div
    class="fixed inset-0 z-50 bg-gray-500/75 transition-opacity"
    role="presentation"
    onclick={cancel}
  ></div>

  <!-- Dialog -->
  <div
    class="fixed inset-0 z-50 overflow-y-auto"
    role="dialog"
    aria-modal="true"
    aria-labelledby="confirm-dialog-title"
  >
    <div class="flex min-h-full items-center justify-center p-4 text-center sm:p-0">
      <div
        class="relative transform overflow-hidden rounded-lg bg-white px-4 pb-4 pt-5 text-left shadow-xl transition-all sm:my-8 sm:w-full sm:max-w-lg sm:p-6"
        role="presentation"
      >
        <div class="sm:flex sm:items-start">
          {#if req.danger}
            <div class="mx-auto flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-red-100 sm:mx-0">
              <svg class="h-6 w-6 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
          {:else}
            <div class="mx-auto flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-blue-100 sm:mx-0">
              <svg class="h-6 w-6 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
          {/if}
          <div class="mt-3 text-center sm:ml-4 sm:mt-0 sm:text-left">
            <h3 id="confirm-dialog-title" class="text-base font-semibold leading-6 text-gray-900">
              {req.title}
            </h3>
            <div class="mt-2">
              <p class="text-sm text-gray-600 whitespace-pre-line">{req.message}</p>
            </div>
            {#if req.requireType}
              <div class="mt-4">
                <label for="confirm-dialog-require-type" class="block text-sm font-medium text-gray-700">
                  Type <span class="font-mono text-gray-900">{req.requireType}</span> to confirm
                </label>
                <input
                  id="confirm-dialog-require-type"
                  type="text"
                  autocomplete="off"
                  bind:value={typedValue}
                  class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-pletka-primary focus:ring-pletka-primary sm:text-sm font-mono"
                />
              </div>
            {/if}
          </div>
        </div>
        <div class="mt-5 sm:mt-4 sm:flex sm:flex-row-reverse">
          <button
            type="button"
            onclick={ok}
            disabled={confirmDisabled}
            class="inline-flex w-full justify-center rounded-md px-3 py-2 text-sm font-semibold text-white shadow-sm sm:ml-3 sm:w-auto disabled:opacity-50 disabled:cursor-not-allowed
              {req.danger ? 'bg-red-600 hover:bg-red-700' : 'bg-pletka-primary hover:bg-pletka-secondary'}"
          >
            {req.confirmLabel ?? 'Confirm'}
          </button>
          <button
            type="button"
            onclick={cancel}
            class="mt-3 inline-flex w-full justify-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 sm:mt-0 sm:w-auto"
          >
            {req.cancelLabel ?? 'Cancel'}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
