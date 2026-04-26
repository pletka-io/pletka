<script lang="ts">
  import type { PageSchema } from "./types/schema";

  /**
   * v0.1.0-dev placeholder.
   *
   * Until concrete widgets exist, the renderer simply pretty-prints the
   * incoming schema. Replace with a WidgetDispatcher once the first widget
   * lands. The contract `PageRenderer` honours stays the same:
   *   - it receives a single PageSchema prop
   *   - it never fetches data or constructs URLs
   *   - it never branches on user identity or role
   */
  let { schema }: { schema: PageSchema } = $props();
</script>

<main class="mx-auto max-w-3xl p-8">
  <header class="mb-6">
    <h1 class="text-2xl font-semibold">{schema.title ?? "Pletka"}</h1>
    <p class="text-sm text-gray-500">
      schema {schema.schemaVersion} · root widget type
      <code class="rounded bg-gray-100 px-1 py-0.5">{schema.body.type}</code>
    </p>
  </header>

  <section>
    <h2 class="mb-2 text-lg font-medium">Page schema</h2>
    <pre class="overflow-auto rounded bg-gray-900 p-4 text-xs text-gray-100">{JSON.stringify(
      schema,
      null,
      2,
    )}</pre>
  </section>

  <footer class="mt-8 text-xs text-gray-400">
    pletka renderer v0.1.0-dev — replace this placeholder once the first widget is wired up.
  </footer>
</main>
