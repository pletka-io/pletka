<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  let { field, value }: { field: FieldDef; value: unknown } = $props();
</script>

<!-- FormRenderer retains the typed value for JSON submission. -->
<input type="hidden" name={field.name} value={typeof value === 'object' ? JSON.stringify(value) : String(value ?? '')} />
