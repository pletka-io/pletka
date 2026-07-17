import { writable } from 'svelte/store';

/**
 * Promise-based confirmation dialog store. Mirrors the toast pattern:
 * a singleton component (ConfirmDialog.svelte) subscribes to the store,
 * any code can call confirmAction() to get a Promise<boolean>.
 *
 * Usage:
 *   const ok = await confirmAction({
 *     title: 'Remove ontology',
 *     message: 'This drops all enabled extensions too.',
 *     confirmLabel: 'Remove',
 *     danger: true,
 *   });
 *   if (!ok) return;
 *
 * Failure mode: when no ConfirmDialog instance is mounted (island
 * missing, bundle not loaded, base layout missing the mount root),
 * confirmAction would otherwise hang forever — there's no subscriber
 * to react to the pushed request. Two safeguards detect that:
 *
 *   1. ConfirmDialog calls registerConfirmDialog() on mount.
 *      confirmAction reads the live mount count synchronously and,
 *      when zero, console.errors loudly and falls back to
 *      window.confirm so the caller's flow does not hang.
 *   2. Even with a registered dialog, a watchdog timer fires after
 *      CONFIRM_RESPONSE_TIMEOUT_MS without a resolve and resolves
 *      the promise as cancelled. Real users always answer well
 *      within that window; the timer is purely defensive against a
 *      stuck or unmounted-mid-flight subscriber.
 */

export interface ConfirmRequest {
  id: number;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  /** Style the confirm button as destructive (red). Default: false. */
  danger?: boolean;
  /**
   * If set, the dialog renders a text input and disables the confirm
   * button until the user types this exact string. Use for high-risk
   * irreversible actions where a single click is too cheap (e.g.
   * destroying a database or instance — operator types the name to
   * acknowledge what they are about to delete).
   */
  requireType?: string;
}

interface InternalRequest extends ConfirmRequest {
  resolve: (ok: boolean) => void;
}

const { subscribe, update, set } = writable<InternalRequest | null>(null);

export const confirmRequest = { subscribe };

let nextId = 0;
let mountedCount = 0;

const CONFIRM_RESPONSE_TIMEOUT_MS = 60_000;

/**
 * Called by ConfirmDialog on mount. Returns a teardown function the
 * component must invoke on unmount. The counter lets confirmAction
 * detect "no dialog mounted" before pushing a request that would
 * otherwise hang waiting for a subscriber.
 */
export function registerConfirmDialog(): () => void {
  mountedCount++;
  return () => {
    if (mountedCount > 0) mountedCount--;
  };
}

/** Visible-for-tests. */
export function _confirmDialogMountCount(): number {
  return mountedCount;
}

export function confirmAction(opts: Omit<ConfirmRequest, 'id'>): Promise<boolean> {
  if (mountedCount === 0) {
    console.error(
      '[confirmAction] No <ConfirmDialog> mounted. Falling back to window.confirm. ' +
        'Check that the host page includes <div data-island="confirm-dialog"> ' +
        'and the "confirm" island bundle is loaded via {{islandScripts "confirm"}}.',
      { title: opts.title, message: opts.message },
    );
    if (typeof window !== 'undefined' && typeof window.confirm === 'function') {
      const text = `${opts.title}\n\n${opts.message}`;
      return Promise.resolve(window.confirm(text));
    }
    return Promise.resolve(false);
  }

  return new Promise<boolean>((resolve) => {
    const id = nextId++;
    let settled = false;
    const wrappedResolve = (ok: boolean) => {
      if (settled) return;
      settled = true;
      clearTimeout(watchdog);
      resolve(ok);
    };
    const watchdog = setTimeout(() => {
      if (settled) return;
      console.error(
        `[confirmAction] No response in ${CONFIRM_RESPONSE_TIMEOUT_MS}ms — dialog stuck or unmounted mid-flight. Resolving as cancelled.`,
        { id, title: opts.title },
      );
      wrappedResolve(false);
      update((curr) => (curr && curr.id === id ? null : curr));
    }, CONFIRM_RESPONSE_TIMEOUT_MS);
    update(() => ({ id, ...opts, resolve: wrappedResolve }));
  });
}

/** Resolve the active request and close the dialog. Used internally by ConfirmDialog. */
export function resolveConfirm(id: number, ok: boolean) {
  update((curr) => {
    if (curr && curr.id === id) {
      curr.resolve(ok);
      return null;
    }
    return curr;
  });
}

/** Cancel any active request without resolving. Useful during teardown. */
export function clearConfirm() {
  update((curr) => {
    if (curr) curr.resolve(false);
    return null;
  });
  set(null);
}
