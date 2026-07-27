<script lang="ts">
  import { sessionExpired, dismissSessionExpired } from '$lib/stores/session-expired';

  function signIn() {
    window.open('/login', '_blank', 'noopener');
  }
</script>

{#if $sessionExpired}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true" aria-labelledby="session-expired-title">
    <div class="max-w-md w-full rounded-lg bg-white p-6 shadow-xl">
      <h2 id="session-expired-title" class="text-lg font-semibold text-gray-900">Your session has expired</h2>
      <p class="mt-2 text-sm text-gray-600">
        Your work on this page is kept. Sign in again, then repeat your last action to save it.
      </p>
      <div class="mt-6 flex justify-end gap-3">
        <button
          type="button"
          class="px-4 py-2 text-sm font-medium text-gray-700 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
          onclick={dismissSessionExpired}
        >Dismiss</button>
        <button
          type="button"
          class="rounded-md bg-pletka-primary px-4 py-2 text-sm font-medium text-white hover:bg-pletka-secondary focus:outline-none focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
          onclick={signIn}
        >Sign in</button>
      </div>
    </div>
  </div>
{/if}
