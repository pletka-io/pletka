import 'virtual:pletka-frontend-contributions';

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
