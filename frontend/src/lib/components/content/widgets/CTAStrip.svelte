<script lang="ts">
  let {
    block,
  }: {
    block: {
      heading?: string;
      body?: string;
      cta?: { label?: string; href?: string };
      style?: 'light' | 'primary';
    };
  } = $props();

  const isPrimary = $derived(block.style !== 'light');
</script>

<section
  class="rounded-lg px-6 py-12 text-center {isPrimary
    ? 'bg-gradient-to-r from-pletka-primary to-pletka-secondary text-white'
    : 'bg-gray-100 text-gray-900'}"
>
  {#if block.heading}
    <h2 class="text-2xl sm:text-3xl font-bold">{block.heading}</h2>
  {/if}
  {#if block.body}
    <p class="mt-3 text-base sm:text-lg {isPrimary ? 'text-white/90' : 'text-gray-600'} max-w-2xl mx-auto">
      {block.body}
    </p>
  {/if}
  {#if block.cta?.href && block.cta?.label}
    <div class="mt-6">
      <a
        href={block.cta.href}
        class="inline-block px-6 py-3 rounded-md font-medium shadow transition-colors {isPrimary
          ? 'bg-white text-pletka-primary hover:bg-gray-100'
          : 'bg-pletka-primary text-white hover:bg-pletka-secondary'}"
      >
        {block.cta.label}
      </a>
    </div>
  {/if}
</section>
