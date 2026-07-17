import { mountIsland } from '../mount';
import ToastContainer from '$lib/components/toast/ToastContainer.svelte';
import { addToast } from '$lib/stores/toast';

// Mount the toast container
mountIsland('toast-container', ToastContainer);

// Expose global bridge for legacy gohtml pages
if (typeof window !== 'undefined') {
  (window as any).showNotification = (
    type: 'success' | 'error' | 'warning' | 'info',
    message: string,
    duration?: number,
  ) => {
    addToast(type, message, duration);
  };
}
