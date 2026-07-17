/**
 * UI language utilities.
 *
 * Single source of truth for the active display language.
 * Currently reads from localStorage with a fallback to 'en'.
 * When we add a reactive language switcher, update this module
 * and all consumers pick it up automatically.
 */

const STORAGE_KEY = 'pletka-ui-lang';
const FALLBACK_LANG = 'en';

/** Return the current UI language code (e.g. 'en', 'nl'). */
export function getUILang(): string {
  if (typeof window === 'undefined') return FALLBACK_LANG;
  return localStorage.getItem(STORAGE_KEY) ?? FALLBACK_LANG;
}

/** Persist the chosen UI language. */
export function setUILang(lang: string) {
  localStorage.setItem(STORAGE_KEY, lang);
}
