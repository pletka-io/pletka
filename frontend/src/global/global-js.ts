import 'virtual:pletka-frontend-contributions';
import { triggerSessionExpired } from '$lib/stores/session-expired';

// Stale-asset recovery: when a deploy ships new hashed chunks the
// browser may still hold an HTML page that references the previous
// generation. Vite emits a `vite:preloadError` event when the
// modulepreload for one of those chunks fails (the "Unable
// to preload CSS for /assets/EntityListView-…css" symptom on the
// Arches tab after a fresh deploy). Reload once with a sentinel so
// the failing chunk doesn't loop us forever, then forget the sentinel
// after the reload succeeds.
window.addEventListener('vite:preloadError', (event) => {
  const FLAG = '__pletka_reload_for_preload__';
  if (sessionStorage.getItem(FLAG)) return;
  sessionStorage.setItem(FLAG, '1');
  // eslint-disable-next-line no-console
  console.warn('vite:preloadError — reloading to pick up fresh assets', event);
  window.location.reload();
});

// Clear the reload sentinel once the page is interactive — if we made
// it here without throwing, the fresh assets loaded fine, and a future
// preload failure should be allowed to trigger one more reload.
if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', () => {
    sessionStorage.removeItem('__pletka_reload_for_preload__');
  }, { once: true });
}

// Global session-expiry detection. Any mutation that returns 401 with the
// canonical envelope code "unauthorized" means the session lapsed (backend:
// pkg/weave/project writeEditDenied). Surface the shared prompt once — the
// caller's own inline error handling is untouched. Scoped OUT: auth
// endpoints, where a 401 is an expected credential rejection (a
// wrong-password /login, an API-key /mcp), not a lapsed session.
function isAuthEndpoint(url: string): boolean {
  try {
    const path = new URL(url, window.location.origin).pathname;
    return path === '/login' || path.startsWith('/auth/') || path === '/mcp';
  } catch {
    return false;
  }
}

const nativeFetch = window.fetch.bind(window);
window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const res = await nativeFetch(input, init);
  if (res.status === 401) {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (!isAuthEndpoint(url)) {
      // clone() so reading the body here does not consume it for the caller.
      const body = await res.clone().json().catch(() => null);
      if (body && body.code === 'unauthorized') {
        triggerSessionExpired();
      }
    }
  }
  return res;
};
