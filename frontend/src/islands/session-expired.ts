import { mountIsland } from '../mount';
import SessionExpiredDialog from '$lib/components/session/SessionExpiredDialog.svelte';

// Singleton session-expired prompt. One per page, driven by the
// session-expired store, fired by the global fetch interceptor.
mountIsland('session-expired-dialog', SessionExpiredDialog);
