<script lang="ts">
  import type { FormSchema } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import { addToast } from '$lib/stores/toast';
  import { confirmAction } from '$lib/stores/confirm';
  import FormSection from './FormSection.svelte';
  import { initialValueForWidget } from './widget-registry';

  let {
    schemaUrl,
    formValues = $bindable<Record<string, any>>({}),
    onSuccessAction = 'reload',
    lang: initialLang = 'en',
    onsuccess,
    oncancel,
  }: {
    schemaUrl: string;
    formValues?: Record<string, any>;
    onSuccessAction?: string;
    lang?: string;
    onsuccess?: () => void;
    oncancel?: () => void;
  } = $props();

  let schema = $state<FormSchema | null>(null);
  let errors = $state<Record<string, string[]>>({});
  let lang = $state('en');
  let loading = $state(true);
  let submitting = $state(false);
  let deleting = $state(false);
  let errorMessage = $state('');
  // One-time secret reveal (schema.ui.reveal_field), e.g. a generated
  // password shown once after create.
  let revealedSecret = $state<string | null>(null);
  let revealCopied = $state(false);
  let pendingAfterReveal: (() => void) | null = null;
  let showConfirm = $state(false);

  // Dynamic options fetched from options_url per field; overlays field.options when present.
  let dynamicOptions = $state<Record<string, { value: string; label: string }[]>>({});
  // Cache of the last-seen dep-key per field, so we only re-fetch when dependencies actually change.
  let depCache: Record<string, string> = {};
  // Fields whose `hidden_until_filled` dependencies are not yet satisfied.
  let hiddenFields = $state<Set<string>>(new Set());
  // Per-field snapshot (JSON-stringified value) taken at submit time, so the
  // auto-clear effect can distinguish "user changed this field since the
  // server rejected it" from "user already had a value when they submitted".
  // Without this snapshot a server 422 on a filled field is wiped before the
  // DOM can show the inline error.
  let lastSubmittedValues = $state<Record<string, string>>({});

  let isDanger = $derived(schema?.theme === 'danger');

  $effect(() => {
    lang = initialLang;
  });

  $effect(() => {
    fetchSchema();
  });

  // isFilled returns true when the form value for `name` is non-empty.
  // Multilingual fields are objects keyed by language code; non-empty means
  // any language has content.
  function isFilled(name: string): boolean {
    const v = formValues[name];
    if (v == null) return false;
    if (typeof v === 'string') return v.length > 0;
    if (Array.isArray(v)) return v.length > 0;
    if (typeof v === 'object') {
      return Object.values(v as Record<string, unknown>).some((x) => x != null && x !== '');
    }
    return true;
  }

  // Clear server-side validation errors only after the user *changes* the
  // field since the last submit. A previous version cleared on any non-empty
  // value, which meant a 422 on a field the user had filled (e.g. value-shape
  // mismatch between client and server) wiped the error before the DOM
  // showed it — the user saw nothing inline and had to read the console.
  $effect(() => {
    if (Object.keys(errors).length === 0) return;
    const _ = formValues; // re-run when any field changes
    const next: Record<string, string[]> = {};
    let changed = false;
    for (const [k, msgs] of Object.entries(errors)) {
      const submitted = lastSubmittedValues[k];
      const current = JSON.stringify(formValues[k] ?? null);
      if (submitted !== undefined && current !== submitted) {
        changed = true;
        continue;
      }
      next[k] = msgs;
    }
    if (changed) errors = next;
  });

  /** Substitute {field_name} tokens in a URL template using current form values. */
  function substituteURL(template: string, values: Record<string, any>): string {
    return template.replace(/\{(\w+)\}/g, (_m, key) => encodeURIComponent(String(values[key] ?? '')));
  }

  /** Compute the set of field names that should be hidden given current values. */
  function computeHidden(currentSchema: FormSchema | null, currentValues: Record<string, any>): Set<string> {
    const hidden = new Set<string>();
    if (!currentSchema) return hidden;
    for (const section of currentSchema.sections) {
      for (const field of section.fields) {
        if (!field.hidden_until_filled?.length) continue;
        const anyEmpty = field.hidden_until_filled.some((k: string) => {
          const v = currentValues[k];
          return v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0);
        });
        if (anyEmpty) hidden.add(field.name);
      }
    }
    return hidden;
  }

  /** Fetch options for a dependent field from its options_url, with {token} substitution. */
  async function refreshDynamicOptions(
    field: { name: string; depends_on?: string[]; options_url?: string },
    values: Record<string, any>,
  ) {
    if (!field.options_url) return;
    try {
      const url = substituteURL(field.options_url, values);
      const res = await fetch(url);
      if (!res.ok) {
        dynamicOptions = { ...dynamicOptions, [field.name]: [] };
        return;
      }
      const data = await res.json();
      // Endpoints either return a bare array of {value,label} entries
      // or {options:[...],total:N}. Support both so we don't tie the
      // field schema to one envelope shape.
      let opts: any[] = [];
      if (Array.isArray(data)) opts = data;
      else if (Array.isArray(data?.options)) opts = data.options;
      else if (Array.isArray(data?.results)) opts = data.results;
      dynamicOptions = { ...dynamicOptions, [field.name]: opts };
    } catch {
      dynamicOptions = { ...dynamicOptions, [field.name]: [] };
    }
  }

  $effect(() => {
    if (!schema) return;
    const values = formValues;
    hiddenFields = computeHidden(schema, values);

    for (const section of schema.sections) {
      for (const field of section.fields) {
        if (!field.options_url && !field.search_url) continue;
        const deps = field.depends_on ?? [];
        const depKey = deps.map((k: string) => String(values[k] ?? '')).join('|');
        const previousDepKey = depCache[field.name];
        if (previousDepKey === depKey) continue;

        depCache = { ...depCache, [field.name]: depKey };

        // Clear stale chosen values only after the first dependency snapshot.
        // Edit forms hydrate dependent fields from the API; clearing on the
        // initial pass would erase stored values before the user sees them.
        const existing = values[field.name];
        if (previousDepKey !== undefined && existing !== null && existing !== undefined && existing !== '') {
          formValues = { ...formValues, [field.name]: null };
        }

        // If any dep value is empty, blank the options and skip the fetch.
        const anyDepEmpty = deps.some((k: string) => {
          const v = values[k];
          return v === undefined || v === null || v === '';
        });
        if (anyDepEmpty) {
          dynamicOptions = { ...dynamicOptions, [field.name]: [] };
          continue;
        }

        if (field.options_url) {
          void refreshDynamicOptions(field, values);
        }
      }
    }
  });

  async function fetchSchema() {
    loading = true;
    try {
      const res = await fetch(schemaUrl);
      if (!res.ok) throw new Error(`Failed to load form: ${res.status}`);
      const data = await res.json();
      // Defensive: a 200 response that doesn't shape like a FormSchema must
      // not be assigned — initFormValues iterates `sections` and would crash.
      if (!data || !Array.isArray(data.sections) || !data.ui) {
        throw new Error('Server returned an unexpected form schema shape.');
      }
      schema = data;
      lang = schema!.ui.primary_language || initialLang;
      initFormValues();
    } catch (e: any) {
      errorMessage = e.message;
      schema = null;
    } finally {
      loading = false;
    }
  }

  function initFormValues() {
    if (!schema) return;
    const values: Record<string, any> = {};
    for (const section of schema.sections) {
      for (const field of section.fields) {
        values[field.name] = field.value ?? initialValueForWidget(field.widget);
      }
    }
    formValues = values;
  }

  function requestSubmit() {
    if (!schema?.endpoint || submitting) return;
    if (schema.endpoint.confirm) {
      showConfirm = true;
      return;
    }
    handleSubmit();
  }

  function confirmAndSubmit() {
    showConfirm = false;
    handleSubmit();
  }

  /**
   * Schema-driven delete. Confirms via native dialog using the schema's
   * Confirm config (or a generic message if absent), issues an HTTP
   * DELETE to schema.delete.url, and on success either calls onsuccess
   * or navigates to the configured success_redirect_url. The button
   * itself is rendered only when schema.delete is present, so the
   * caller doesn't need to gate visibility — capability-driven.
   */
  async function handleDelete() {
    if (!schema?.delete || deleting) return;
    const cfg = schema.delete.confirm;
    const ok = await confirmAction({
      title: cfg ? tr(cfg.title, lang) : 'Delete',
      message: cfg ? tr(cfg.message, lang) : 'Delete this entity? This cannot be undone.',
      confirmLabel: 'Delete',
      danger: true,
    });
    if (!ok) return;
    deleting = true;
    errorMessage = '';
    try {
      const res = await fetch(schema.delete.url, {
        method: 'DELETE',
        credentials: 'same-origin',
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
      if (!res.ok && res.status !== 204) {
        const data = await res.json().catch(() => null);
        errorMessage = data?.error || data?.message || `Delete failed (${res.status})`;
        return;
      }
      const okMsg = tr(schema.delete.success_message, lang);
      if (okMsg) addToast('success', okMsg);
      const target = schema.delete.success_redirect_url;
      if (target) {
        window.location.assign(target);
        return;
      }
      if (onsuccess) onsuccess();
    } catch (e: any) {
      errorMessage = e?.message ?? 'Network error during delete';
    } finally {
      deleting = false;
    }
  }

  async function handleSubmit() {
    if (!schema?.endpoint || submitting) return;
    submitting = true;
    errors = {};
    errorMessage = '';

    try {
      // Do not submit hidden fields — their dependencies are unsatisfied.
      const payload: Record<string, any> = {};
      for (const [k, v] of Object.entries(formValues)) {
        if (hiddenFields.has(k)) continue;
        payload[k] = v;
      }

      // Snapshot per-field values at submit time so the auto-clear effect
      // can tell whether the user has edited a field since the server's
      // 422 — only then do we drop the inline error.
      const snapshot: Record<string, string> = {};
      for (const [k, v] of Object.entries(formValues)) {
        snapshot[k] = JSON.stringify(v ?? null);
      }
      lastSubmittedValues = snapshot;

      const res = await fetch(schema.endpoint.url, {
        method: schema.endpoint.method,
        headers: {
          'Content-Type': 'application/json',
          'X-Requested-With': 'XMLHttpRequest',
        },
        body: JSON.stringify(payload),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => null);
        if (data?.errors) {
          errors = data.errors;
        } else {
          errorMessage = data?.message || `Error: ${res.status}`;
        }
        return;
      }

      // Success
      const successMsg = tr(schema.ui.success_message, lang);
      addToast('success', successMsg);

      // Read the response body once; used for both reveal and redirect.
      const body = await res.json().catch(() => null);

      const ui = schema!.ui;
      const finish = () => {
        const redirectTpl = ui.success_redirect_url_template;
        if (redirectTpl) {
          const id = body?.id ?? body?.project_id ?? '';
          window.location.assign(redirectTpl.replace('{id}', encodeURIComponent(String(id))));
          return;
        }
        if (onsuccess) {
          onsuccess();
        } else if (onSuccessAction === 'reload') {
          window.location.reload();
        }
      };

      // Schema-declared one-time secret (e.g. a generated password): show it
      // once in a copy dialog, then continue (reload/redirect) on dismiss.
      const revealField = ui.reveal_field;
      const secret = revealField ? body?.[revealField] : '';
      if (secret) {
        revealCopied = false;
        revealedSecret = String(secret);
        pendingAfterReveal = finish;
        return;
      }

      finish();
    } catch (e: any) {
      errorMessage = e.message;
    } finally {
      submitting = false;
    }
  }

  async function copyRevealedSecret() {
    if (!revealedSecret) return;
    try {
      await navigator.clipboard.writeText(revealedSecret);
      revealCopied = true;
    } catch {
      revealCopied = false;
    }
  }

  function dismissReveal() {
    revealedSecret = null;
    const after = pendingAfterReveal;
    pendingAfterReveal = null;
    if (after) after();
  }
</script>

{#if revealedSecret}
  <div class="fixed inset-0 z-[60] flex items-center justify-center bg-black/40" role="dialog" aria-modal="true">
    <div class="bg-white rounded-lg shadow-xl w-[440px] max-w-[90vw] p-6 space-y-4">
      <h3 class="text-base font-semibold text-gray-900">
        {schema && schema.ui.reveal_label ? tr(schema.ui.reveal_label, lang) : 'Copy this — shown only once'}
      </h3>
      <div class="flex items-center gap-2">
        <code class="flex-1 font-mono text-sm bg-gray-100 border border-gray-200 rounded px-3 py-2 break-all select-all">{revealedSecret}</code>
        <button
          type="button"
          class="px-3 py-2 rounded-md bg-pletka-primary text-white text-sm font-medium hover:bg-pletka-secondary"
          onclick={copyRevealedSecret}
        >{revealCopied ? 'Copied' : 'Copy'}</button>
      </div>
      <p class="text-xs text-gray-500">This value will not be shown again. Copy it now.</p>
      <div class="flex justify-end">
        <button
          type="button"
          class="px-4 py-2 rounded-md bg-gray-800 text-white text-sm font-medium hover:bg-gray-900"
          onclick={dismissReveal}
        >Done</button>
      </div>
    </div>
  </div>
{/if}

{#if loading}
  <div class="animate-pulse space-y-4 p-4">
    <div class="h-4 bg-gray-200 rounded w-1/4"></div>
    <div class="h-10 bg-gray-200 rounded"></div>
    <div class="h-10 bg-gray-200 rounded"></div>
  </div>
{:else if errorMessage && !schema}
  <div class="text-red-600 p-4">{errorMessage}</div>
{:else if schema}
  <form onsubmit={(e) => { e.preventDefault(); requestSubmit(); }}>
    {#if errorMessage}
      <div class="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded mb-4">
        {errorMessage}
      </div>
    {/if}

    {#each schema.sections as section (section.id)}
      {#if !section.visible_when || formValues[section.visible_when.field] === section.visible_when.equals}
      <FormSection
        {section}
        bind:formValues
        {lang}
        languages={schema.ui.languages}
        {errors}
        {dynamicOptions}
        {hiddenFields}
      />
      {/if}
    {/each}

    {#if schema.endpoint}
      <div class="flex justify-between gap-3 mt-4">
        <div>
          {#if schema.delete}
            <button type="button"
              disabled={submitting || deleting}
              onclick={handleDelete}
              class="px-4 py-2 text-sm font-medium text-red-700 bg-white border border-red-300 rounded-md hover:bg-red-50 disabled:opacity-50">
              {#if deleting}
                Deleting...
              {:else}
                {tr(schema.delete.label, lang) || 'Delete'}
              {/if}
            </button>
          {/if}
        </div>
        <div class="flex gap-3">
          {#if schema.ui.cancel_label}
            <button type="button"
              onclick={() => {
                if (oncancel) {
                  oncancel();
                }
              }}
              class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50">
              {tr(schema.ui.cancel_label, lang)}
            </button>
          {/if}
          <button type="submit"
            disabled={submitting || deleting}
            class="px-4 py-2 text-sm font-medium text-white border border-transparent rounded-md disabled:opacity-50
              {isDanger ? 'bg-red-600 hover:bg-red-700' : 'bg-pletka-primary hover:bg-pletka-secondary'}">
            {#if submitting}
              Saving...
            {:else}
              {tr(schema.ui.submit_label, lang) || 'Save'}
            {/if}
          </button>
        </div>
      </div>
    {/if}
  </form>

  <!-- Confirmation dialog -->
  {#if showConfirm && schema?.endpoint?.confirm}
    <div class="fixed inset-0 z-50 flex items-center justify-center">
      <button
        type="button"
        class="absolute inset-0 bg-black/50"
        aria-label="Cancel confirmation"
        onclick={() => { showConfirm = false; }}
      ></button>
      <div class="relative bg-white rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
        <h3 class="text-lg font-semibold text-gray-900 mb-2">
          {tr(schema.endpoint.confirm.title, lang)}
        </h3>
        <p class="text-sm text-gray-600 mb-6">
          {tr(schema.endpoint.confirm.message, lang)}
        </p>
        <div class="flex justify-end gap-3">
          <button type="button"
            onclick={() => { showConfirm = false; }}
            class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50">
            Cancel
          </button>
          <button type="button"
            onclick={confirmAndSubmit}
            class="px-4 py-2 text-sm font-medium text-white bg-red-600 border border-transparent rounded-md hover:bg-red-700">
            {tr(schema.endpoint.confirm.confirm_label, lang)}
          </button>
        </div>
      </div>
    </div>
  {/if}
{/if}
