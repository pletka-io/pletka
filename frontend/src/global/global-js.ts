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

// Global session-expiry detection. The auth middleware sets an
// "X-Session-Expired: 1" response header whenever a request arrives with a
// stale session cookie (expired/invalidated). On any failed response carrying
// it, surface the shared prompt — this works regardless of the status the
// route returns (401 from writeEditDenied, or a 404 from the existence-hiding
// project-read gate on a private project). The caller's own inline error
// handling is untouched; only headers are read, so the body is never consumed.
// Scoped OUT: the /api/v1/auth/* credential endpoints, where an anonymous
// request with a leftover cookie is expected (logging in), not a lapse.
function isAuthEndpoint(url: string): boolean {
  try {
    const path = new URL(url, window.location.origin).pathname;
    return path.startsWith('/api/v1/auth/');
  } catch {
    return false;
  }
}

const nativeFetch = window.fetch.bind(window);
window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const res = await nativeFetch(input, init);
  if (!res.ok && res.headers.get('X-Session-Expired') === '1') {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (!isAuthEndpoint(url)) {
      triggerSessionExpired();
    }
  }
  return res;
};
