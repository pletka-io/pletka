<script lang="ts">
  /**
   * Dispatches each block in a PageSchema to its widget. Picks a
   * page-level template (Article or Hero) based on schema.template.
   */
  import type { PageSchema } from './types';
  import { widgetRegistry } from './widget-registry';
  import Article from './templates/Article.svelte';
  import Hero from './templates/Hero.svelte';

  // Server seeds the schema as a JSON string in data-prop-schema. The
  // mount.ts hydration parses it for us when the prop arrives as an
  // object; when it arrives as a string we parse here defensively.
  let { schema }: { schema: PageSchema | string } = $props();

  const parsed: PageSchema = $derived(
    typeof schema === 'string' ? JSON.parse(schema) : schema,
  );

  const TemplateComponent = $derived(
    parsed.template === 'hero' ? Hero : Article,
  );
</script>

<TemplateComponent schema={parsed} {widgetRegistry} />
