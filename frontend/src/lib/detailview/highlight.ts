// Shiki wrapper for derivative tabs.
//
// Uses shiki/core with explicit per-language dynamic imports so the
// initial bundle stays small (no emacs-lisp / cpp / etc. tagged
// along). Each language grammar loads on first use and is cached for
// subsequent calls.
//
// Theme is github-light per platform-wide light-only design.

import { createHighlighterCore, type HighlighterCore } from 'shiki/core';
import { createOnigurumaEngine } from 'shiki/engine/oniguruma';

const THEME = 'github-light';

// Shiki language identifiers we surface in derivative tabs.
export type DerivativeLang =
  | 'turtle'
  | 'json'
  | 'sparql'
  | 'xml'
  | 'yaml'
  | 'plaintext';

let highlighterPromise: Promise<HighlighterCore> | null = null;
const loadedLangs = new Set<DerivativeLang>();

async function getHighlighter(): Promise<HighlighterCore> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighterCore({
      themes: [import('shiki/themes/github-light.mjs')],
      langs: [],
      engine: createOnigurumaEngine(import('shiki/wasm')),
    });
  }
  return highlighterPromise;
}

async function ensureLang(lang: DerivativeLang): Promise<void> {
  if (loadedLangs.has(lang) || lang === 'plaintext') {
    return;
  }
  const hl = await getHighlighter();
  switch (lang) {
    case 'turtle':
      await hl.loadLanguage(import('shiki/langs/turtle.mjs'));
      break;
    case 'json':
      await hl.loadLanguage(import('shiki/langs/json.mjs'));
      break;
    case 'sparql':
      await hl.loadLanguage(import('shiki/langs/sparql.mjs'));
      break;
    case 'xml':
      await hl.loadLanguage(import('shiki/langs/xml.mjs'));
      break;
    case 'yaml':
      await hl.loadLanguage(import('shiki/langs/yaml.mjs'));
      break;
  }
  loadedLangs.add(lang);
}

/**
 * highlight returns a sanitized HTML <pre><code>…</code></pre> string
 * that callers can drop into a Svelte `{@html …}` slot. Falls back to
 * an HTML-escaped <pre> when Shiki fails (network glitch, unsupported
 * grammar, etc.) so the tab never renders empty.
 */
export async function highlight(
  source: string,
  lang: DerivativeLang,
): Promise<string> {
  if (!source) return '';
  if (lang === 'plaintext') {
    return `<pre class="shiki-plain">${escapeHtml(source)}</pre>`;
  }
  try {
    await ensureLang(lang);
    const hl = await getHighlighter();
    return hl.codeToHtml(source, { lang, theme: THEME });
  } catch (err) {
    // Defensive: any Shiki failure (loadLanguage rejecting, grammar
    // bug, …) should fall back to readable plain text rather than
    // breaking the whole tab.
    console.warn('shiki highlight failed', { lang, err });
    return `<pre class="shiki-plain">${escapeHtml(source)}</pre>`;
  }
}

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}
