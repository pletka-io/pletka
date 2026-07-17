<script lang="ts">
  /**
   * Prose block. Renders pre-rendered HTML from the markdown body
   * (or an inline `body` field that the loader rendered through
   * goldmark). Optional decorative chrome:
   *   - `eyebrow`: small uppercase label above the heading (e.g. "Section I")
   *   - `heading`: section title
   *   - `rule`:    decorative cyan bar between heading and body
   *   - `id`:      anchor id for TOC linking
   */
  let {
    block,
  }: {
    block: { eyebrow?: string; heading?: string; rule?: boolean; id?: string; html?: string };
  } = $props();
</script>

<section class="max-w-3xl mx-auto" id={block.id}>
  {#if block.eyebrow}
    <p class="text-xs font-semibold uppercase tracking-widest text-pletka-primary mb-3">
      {block.eyebrow}
    </p>
  {/if}
  {#if block.heading}
    <h2 class="text-3xl font-bold text-gray-900 mb-4">{block.heading}</h2>
  {/if}
  {#if block.rule}
    <div class="w-12 h-0.5 bg-pletka-primary mb-8"></div>
  {/if}
  {#if block.html}
    <article class="prose prose-slate max-w-none">
      {@html block.html}
    </article>
  {/if}
</section>

<style>
  .prose :global(h1) {
    font-size: 2.25rem;
    font-weight: 700;
    margin-top: 0;
    margin-bottom: 1.5rem;
    color: #111827;
  }
  .prose :global(h2) {
    font-size: 1.5rem;
    font-weight: 700;
    margin-top: 2rem;
    margin-bottom: 1rem;
    color: #111827;
  }
  .prose :global(h3) {
    font-size: 1.25rem;
    font-weight: 600;
    margin-top: 1.5rem;
    margin-bottom: 0.75rem;
    color: #1f2937;
  }
  .prose :global(p) {
    margin: 0.75rem 0;
    color: #374151;
    line-height: 1.7;
  }
  .prose :global(ul),
  .prose :global(ol) {
    margin: 0.75rem 0;
    padding-left: 1.5rem;
    color: #374151;
  }
  .prose :global(li) {
    margin: 0.25rem 0;
  }
  .prose :global(a) {
    color: #2563eb;
    text-decoration: underline;
  }
  .prose :global(code) {
    background: #f3f4f6;
    padding: 0.1rem 0.3rem;
    border-radius: 0.25rem;
    font-size: 0.9em;
  }
  .prose :global(pre) {
    background: #1f2937;
    color: #f3f4f6;
    padding: 1rem;
    border-radius: 0.5rem;
    overflow-x: auto;
  }
  .prose :global(blockquote) {
    border-left: 4px solid #d1d5db;
    padding-left: 1rem;
    margin: 1rem 0;
    color: #4b5563;
    font-style: italic;
  }
  .prose :global(hr) {
    border: 0;
    border-top: 1px solid #e5e7eb;
    margin: 2rem 0;
  }
</style>
