<script lang="ts">
  import { toasts, dismissToast, registerToastContainer, addToast, type Toast } from '$lib/stores/toast';
  import { fly, fade } from 'svelte/transition';

  // Register / teardown so addToast() can detect "no container
  // mounted" and loud-fail instead of silently dropping toasts.
  $effect(() => registerToastContainer());

  // Replay a pending toast persisted across a hard navigation (used by
  // the Adapt flow in DetailView when the server returns a redirect to
  // the newly-created entity). One-shot — read and clear.
  $effect(() => {
    try {
      const raw = sessionStorage.getItem('__pendingToast');
      if (!raw) return;
      sessionStorage.removeItem('__pendingToast');
      const parsed = JSON.parse(raw) as { tone: Toast['type']; message: string };
      if (parsed?.message) addToast(parsed.tone || 'success', parsed.message);
    } catch { /* malformed or sessionStorage unavailable */ }
  });

  const iconMap: Record<Toast['type'], string> = {
    success: 'M5 13l4 4L19 7',
    error: 'M6 18L18 6M6 6l12 12',
    warning: 'M12 9v2m0 4h.01M12 2a10 10 0 100 20 10 10 0 000-20z',
    info: 'M13 16h-1v-4h-1m1-4h.01M12 2a10 10 0 100 20 10 10 0 000-20z',
  };

  const colorMap: Record<Toast['type'], string> = {
    success: 'bg-green-500',
    error: 'bg-red-500',
    warning: 'bg-amber-500',
    info: 'bg-blue-500',
  };
</script>

<div class="fixed top-4 right-4 z-[9999] flex flex-col gap-2 pointer-events-none">
  {#each $toasts as toast (toast.id)}
    <div
      role="alert"
      class="pointer-events-auto flex items-center gap-3 px-4 py-3 rounded-lg shadow-lg text-white text-sm {colorMap[toast.type]}"
      in:fly={{ x: 300, duration: 300 }}
      out:fade={{ duration: 200 }}
    >
      <svg class="w-5 h-5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d={iconMap[toast.type]} />
      </svg>
      <span class="flex-1">{toast.message}</span>
      <button
        type="button"
        aria-label="Close notification"
        class="flex-shrink-0 opacity-70 hover:opacity-100"
        onclick={() => dismissToast(toast.id)}
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  {/each}
</div>
