import { writable } from 'svelte/store';

export interface Toast {
  id: number;
  type: 'success' | 'error' | 'warning' | 'info';
  message: string;
  duration: number;
}

let nextId = 0;
let mountedCount = 0;
const timeoutMap = new Map<number, ReturnType<typeof setTimeout>>();

const { subscribe, update } = writable<Toast[]>([]);

export const toasts = { subscribe };

/**
 * Called by ToastContainer on mount. Returns a teardown function the
 * component must invoke on unmount. The counter lets addToast detect
 * "no container mounted" and loud-fail instead of silently dropping
 * user-facing feedback (success / error toasts).
 */
export function registerToastContainer(): () => void {
  mountedCount++;
  return () => {
    if (mountedCount > 0) mountedCount--;
  };
}

/** Visible-for-tests. */
export function _toastContainerMountCount(): number {
  return mountedCount;
}

export function addToast(
  type: Toast['type'],
  message: string,
  duration?: number,
) {
  if (mountedCount === 0) {
    console.error(
      '[addToast] No <ToastContainer> mounted. Toast dropped. ' +
        'Check that the host page includes <div data-island="toast-container"> ' +
        'and the "toast" island bundle is loaded via {{islandScripts "toast"}}.',
      { type, message },
    );
    return -1;
  }

  const defaults: Record<Toast['type'], number> = {
    success: 3000,
    error: 5000,
    warning: 4000,
    info: 3000,
  };
  const id = nextId++;
  const toast: Toast = { id, type, message, duration: duration ?? defaults[type] };

  update((all) => [...all, toast]);

  if (toast.duration > 0) {
    const timeoutId = setTimeout(() => dismissToast(id), toast.duration);
    timeoutMap.set(id, timeoutId);
  }

  return id;
}

export function dismissToast(id: number) {
  const timeoutId = timeoutMap.get(id);
  if (timeoutId) {
    clearTimeout(timeoutId);
    timeoutMap.delete(id);
  }
  update((all) => all.filter((t) => t.id !== id));
}
