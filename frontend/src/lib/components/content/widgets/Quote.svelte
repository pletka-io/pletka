<script lang="ts">
  /**
   * Pull-quote block. Default style is decorative (cyan border, gradient
   * background, big quote mark). Use `style: "plain"` for a minimal
   * blockquote look.
   */
  let {
    block,
  }: {
    block: {
      text?: string;
      attribution?: string;
      style?: 'decorative' | 'plain';
    };
  } = $props();

  const style = $derived(block.style ?? 'decorative');
</script>

<figure class="max-w-3xl mx-auto py-8">
  {#if style === 'decorative'}
    <blockquote class="pull-quote">
      <p class="text-lg text-gray-700 italic leading-relaxed">{block.text}</p>
    </blockquote>
  {:else}
    <blockquote class="border-l-4 border-pletka-primary pl-6 italic text-xl text-gray-700 leading-relaxed">
      {block.text}
    </blockquote>
  {/if}
  {#if block.attribution}
    <figcaption class="mt-3 pl-6 text-sm text-gray-500">— {block.attribution}</figcaption>
  {/if}
</figure>

<style>
  .pull-quote {
    position: relative;
    padding: 2rem 2.5rem;
    border-left: 4px solid var(--pletka-primary, #92ccc8);
    background: linear-gradient(135deg, rgba(146, 204, 200, 0.06), rgba(245, 128, 49, 0.04));
    border-radius: 0.25rem;
  }
  .pull-quote::before {
    content: '\201C';
    position: absolute;
    top: -0.25rem;
    left: 0.75rem;
    font-family: Georgia, serif;
    font-size: 4rem;
    color: var(--pletka-primary, #92ccc8);
    opacity: 0.3;
    line-height: 1;
  }
</style>
