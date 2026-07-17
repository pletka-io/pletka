import { mountIsland } from '../mount';
import ConfirmDialog from '$lib/components/confirm/ConfirmDialog.svelte';

// Mount the singleton confirm dialog. Same lifecycle as toast: one
// instance per page, fed by the confirm store. Any code can call
// confirmAction() from $lib/stores/confirm and get a Promise<boolean>.
mountIsland('confirm-dialog', ConfirmDialog);
