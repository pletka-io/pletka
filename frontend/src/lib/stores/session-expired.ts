import { writable } from 'svelte/store';

/**
 * Session-expiry singleton. The global fetch interceptor
 * (global/global-js.ts) calls triggerSessionExpired() when any mutation
 * returns 401 code=unauthorized; SessionExpiredDialog subscribes and shows
 * the "sign in again" prompt. Idempotent: a burst of 401s shows one prompt.
 */
const { subscribe, set } = writable<boolean>(false);

export const sessionExpired = { subscribe };

export function triggerSessionExpired(): void {
	set(true);
}

export function dismissSessionExpired(): void {
	set(false);
}
