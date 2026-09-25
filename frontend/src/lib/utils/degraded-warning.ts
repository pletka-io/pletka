// Module-level, so the once-per-page-load lifetime is shared by every caller
// and every component instance: a repeatable field mounts one picker per
// occurrence, and a concept-list page can hold several pickers reading the
// same vocabulary. A component-local set would warn once per instance.
const warned = new Set<string>();

/**
 * warnVocabularyDegraded reports to the console, at most once per `key` per
 * page load, that a vocabulary lookup degraded: the search route answered 200
 * with a `degraded` flag, meaning the vocabulary service did not answer and
 * the suggestions shown are incomplete.
 *
 * `key` dedupes (the search URL, or the source id); `label` names the
 * vocabulary in human terms.
 *
 * Nothing is rendered. A degraded lookup means fewer suggestions, not a broken
 * page, and a curator has no action to take — this line exists for whoever is
 * looking at the console or a browser error report, and must never reach the
 * DOM as a banner, a toast, or any piece of component state.
 */
export function warnVocabularyDegraded(key: string, label: string): void {
  if (warned.has(key)) return;
  warned.add(key);
  console.warn(
    `[pletka] vocabulary lookup degraded for ${label}: ` +
      'suggestions from this vocabulary are incomplete because the vocabulary service did not answer. ' +
      'This warning appears once per vocabulary per page load, so this is logged only once even though the lookup keeps failing.',
  );
}
