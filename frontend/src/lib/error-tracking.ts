/**
 * Frontend error reporter.
 *
 * Captures uncaught browser exceptions and unhandled promise rejections
 * and POSTs them to /errors/client. Same Postgres table the server-side
 * middleware writes to (weave_error_events) — one queryable surface for
 * everything that goes wrong while users test the app.
 *
 * Usage:
 *   import { installGlobalHandlers } from '$lib/error-tracking';
 *   installGlobalHandlers();   // call once, as early as possible
 *
 * Explicit reporting from a try/catch site:
 *   captureException(err, { island: 'ProjectSettings', section: 'about' });
 *
 * The reporter is best-effort: any failure inside it is swallowed so a
 * broken endpoint never breaks the page.
 */

const ENDPOINT = '/errors/client';

type Context = Record<string, unknown>;

interface Breadcrumb {
  ts: string;
  type: string;
  target?: string;
}

interface ClientErrorEvent {
  occurred_at: string;
  page_url: string;
  island?: string;
  error: {
    class: string;
    message: string;
    stack: string;
  };
  context?: Context;
  breadcrumbs?: Breadcrumb[];
}

let installed = false;
const breadcrumbs: Breadcrumb[] = [];
const MAX_BREADCRUMBS = 20;

/**
 * Register window.onerror + unhandledrejection listeners. Idempotent —
 * subsequent calls are no-ops.
 */
export function installGlobalHandlers(): void {
  if (installed) return;
  installed = true;

  window.addEventListener('error', (event) => {
    const err = event.error;
    report({
      occurred_at: new Date().toISOString(),
      page_url: location.pathname + location.search,
      error: {
        class: err?.name ?? 'Error',
        message: event.message || String(err?.message ?? ''),
        stack: err?.stack ?? '',
      },
      breadcrumbs: snapshotBreadcrumbs(),
    });
  });

  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason;
    const err = reason instanceof Error ? reason : null;
    report({
      occurred_at: new Date().toISOString(),
      page_url: location.pathname + location.search,
      error: {
        class: err?.name ?? 'UnhandledRejection',
        message: err?.message ?? String(reason),
        stack: err?.stack ?? '',
      },
      breadcrumbs: snapshotBreadcrumbs(),
    });
  });

  // Light-touch breadcrumb collection: clicks on buttons + form
  // submissions. Helps reconstruct what the user was doing when the
  // exception fired.
  document.addEventListener('click', (event) => {
    const target = event.target;
    if (!(target instanceof HTMLElement)) return;
    const button = target.closest('button, a');
    if (!button) return;
    pushBreadcrumb({
      ts: new Date().toISOString(),
      type: 'click',
      target: describeElement(button as HTMLElement),
    });
  }, { capture: true });
}

/**
 * Explicit report from inside a try/catch.
 */
export function captureException(err: unknown, context?: Context): void {
  const e = err instanceof Error ? err : null;
  report({
    occurred_at: new Date().toISOString(),
    page_url: location.pathname + location.search,
    error: {
      class: e?.name ?? 'Error',
      message: e?.message ?? String(err),
      stack: e?.stack ?? '',
    },
    context,
    breadcrumbs: snapshotBreadcrumbs(),
  });
}

function pushBreadcrumb(b: Breadcrumb): void {
  breadcrumbs.push(b);
  if (breadcrumbs.length > MAX_BREADCRUMBS) {
    breadcrumbs.shift();
  }
}

function snapshotBreadcrumbs(): Breadcrumb[] | undefined {
  if (breadcrumbs.length === 0) return undefined;
  return breadcrumbs.slice();
}

function describeElement(el: HTMLElement): string {
  const tag = el.tagName.toLowerCase();
  const id = el.id ? `#${el.id}` : '';
  const classes = el.className && typeof el.className === 'string'
    ? '.' + el.className.split(/\s+/).slice(0, 2).join('.')
    : '';
  const text = el.textContent?.trim().slice(0, 40) ?? '';
  return `${tag}${id}${classes}${text ? ` "${text}"` : ''}`;
}

const MAX_STACK = 12_000;
const MAX_MESSAGE = 2_000;

async function report(event: ClientErrorEvent): Promise<void> {
  try {
    // Bound the payload so the reporter never trips the ingest endpoint's
    // body-size limit. A Svelte reactive-render error stack can be tens of KB;
    // an oversized body was rejected (400) and the error lost — exactly the
    // case that hid the ReuseTab $state collision.
    if (event.error?.stack && event.error.stack.length > MAX_STACK) {
      event.error.stack = event.error.stack.slice(0, MAX_STACK) + '\n…[truncated]';
    }
    if (event.error?.message && event.error.message.length > MAX_MESSAGE) {
      event.error.message = event.error.message.slice(0, MAX_MESSAGE) + '…';
    }
    await fetch(ENDPOINT, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(event),
      keepalive: true,
      credentials: 'same-origin',
    });
  } catch {
    // never let the reporter itself raise
  }
}
