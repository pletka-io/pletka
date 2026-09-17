<script lang="ts">
  import type { EntityListSchema } from '$lib/types/entity-list-schema';
  import ExampleWorkspace from '$lib/components/example/ExampleWorkspace.svelte';

  let {
    schema,
    mode,
    itemId = '',
    lang,
    onsuccess,
    oncancel,
  }: {
    schema: EntityListSchema;
    mode: 'create' | 'edit';
    itemId?: string;
    lang: string;
    onsuccess?: () => void | Promise<void>;
    oncancel?: () => void;
  } = $props();

  function scopedTarget(): { entityType: string; entityId: string } | null {
    try {
      const url = new URL(schema.data_url, window.location.origin);
      const entityType = url.searchParams.get('entity_type') || '';
      const entityId = url.searchParams.get('entity_id') || '';
      if (!entityType || !entityId) return null;
      return { entityType, entityId };
    } catch {
      return null;
    }
  }

  const target = $derived(scopedTarget());

  // Schema-provided endpoint URLs for the workspace (schema-driven API
  // rule: the frontend never constructs an API URL).
  const endpoints = $derived({
    list_url: schema.editor?.endpoints?.list_url ?? '',
    detail_url_template: schema.editor?.endpoints?.detail_url_template ?? '',
    form_schema_url: schema.editor?.endpoints?.form_schema_url ?? '',
    page_url_template: schema.editor?.endpoints?.page_url_template ?? '',
  });
</script>

<ExampleWorkspace
  projectId={schema.project_id}
  {endpoints}
  {mode}
  exampleId={mode === 'edit' ? itemId : ''}
  initialTargetEntityType={mode === 'create' ? target?.entityType ?? '' : ''}
  initialTargetEntityId={mode === 'create' ? target?.entityId ?? '' : ''}
  {lang}
  {onsuccess}
  {oncancel}
/>
