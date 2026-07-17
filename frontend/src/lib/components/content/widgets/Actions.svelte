<script lang="ts">
  /**
   * Inline row of action buttons. Used at the bottom of zen + about
   * for the "Read more / Explore" link rows.
   *
   * Each item: { label, href, style? } where style is "primary" or
   * "secondary" (default). Wraps on mobile.
   */
  type Action = {
    label: string;
    href: string;
    style?: 'primary' | 'secondary';
  };

  let {
    block,
  }: {
    block: {
      align?: 'left' | 'center' | 'right';
      items?: Action[];
    };
  } = $props();

  const items = $derived(block.items ?? []);
  const align = $derived(block.align ?? 'center');
  const justify = $derived(
    align === 'left' ? 'justify-start' : align === 'right' ? 'justify-end' : 'justify-center',
  );
</script>

<section class="py-4">
  <div class="flex flex-col sm:flex-row gap-4 {justify}">
    {#each items as item}
      <a
        href={item.href}
        class="inline-flex items-center px-6 py-3 text-base font-medium rounded-md transition-colors {item.style ===
        'primary'
          ? 'bg-pletka-primary text-white hover:bg-pletka-secondary'
          : 'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50'}"
      >
        {item.label}
      </a>
    {/each}
  </div>
</section>
