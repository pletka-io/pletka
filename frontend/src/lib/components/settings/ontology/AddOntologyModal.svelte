<script lang="ts">
  import FormRenderer from '$lib/components/form/FormRenderer.svelte';

  let {
    formSchemaUrl,
    open = $bindable(false),
    lang = 'en',
    onsuccess,
  }: {
    // Server-emitted form-schema URL (PaneView.actions.form_schema_url).
    // Modal is now URL-agnostic — caller hands it down so we honour the
    // schema-as-contract rule (.claude/rules/api-patterns.md). The
    // FormRenderer's submit URL is in turn read out of the form schema
    // itself, so the whole flow is server-driven.
    formSchemaUrl: string;
    open?: boolean;
    lang?: string;
    onsuccess?: () => void;
  } = $props();

  function close() {
    open = false;
  }

  function handleSuccess() {
    open = false;
    onsuccess?.();
  }
</script>

{#if open}
  <div class="fixed inset-0 z-50 overflow-y-auto" role="dialog" aria-modal="true">
    <!-- Backdrop -->
    <div
      class="fixed inset-0 bg-gray-500/75 transition-opacity"
      aria-hidden="true"
      onclick={close}
      role="presentation"
    ></div>

    <!-- Modal panel -->
    <div class="flex min-h-screen items-end justify-center px-4 pb-20 pt-4 text-center sm:block sm:p-0">
      <span class="hidden sm:inline-block sm:h-screen sm:align-middle" aria-hidden="true">&#8203;</span>

      <div class="relative inline-block w-full max-w-2xl transform overflow-hidden rounded-lg bg-white px-4 pb-4 pt-5 text-left align-bottom shadow-xl transition-all sm:my-8 sm:align-middle sm:p-6">
        <div class="flex items-start justify-between mb-4">
          <h3 class="text-lg font-medium text-gray-900">Add ontology</h3>
          <button
            type="button"
            onclick={close}
            class="text-gray-400 hover:text-gray-600"
            aria-label="Close"
          >
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <FormRenderer
          schemaUrl={formSchemaUrl}
          onSuccessAction="none"
          onsuccess={handleSuccess}
          oncancel={close}
        />
      </div>
    </div>
  </div>
{/if}
