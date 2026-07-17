<script lang="ts">
  let {
    block,
  }: {
    block: {
      eyebrow?: string;
      title?: string;
      subtitle?: string;
      subtitle_html?: string;
      cta?: { label?: string; href?: string };
    };
  } = $props();
</script>

<section class="bg-gradient-to-br from-pletka-primary to-pletka-secondary text-white">
  <div class="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-20 text-center">
    {#if block.eyebrow}
      <p class="text-xs uppercase tracking-widest text-white/70 mb-3">{block.eyebrow}</p>
    {/if}
    {#if block.title}
      <h1 class="text-4xl sm:text-5xl font-bold tracking-tight">{block.title}</h1>
    {/if}
    {#if block.subtitle_html}
      <p class="mt-4 text-lg sm:text-xl text-white/90 max-w-3xl mx-auto">
        {@html block.subtitle_html}
      </p>
    {:else if block.subtitle}
      <p class="mt-4 text-lg sm:text-xl text-white/90 max-w-3xl mx-auto">{block.subtitle}</p>
    {/if}
    {#if block.cta?.href && block.cta?.label}
      <div class="mt-8">
        <a
          href={block.cta.href}
          class="inline-block px-6 py-3 bg-white text-pletka-primary rounded-md font-medium shadow hover:bg-gray-100 transition-colors"
        >
          {block.cta.label}
        </a>
      </div>
    {/if}
  </div>
</section>
