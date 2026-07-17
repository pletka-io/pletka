<script lang="ts">
  /**
   * Numbered-step list. Each item gets a circular gradient badge with
   * its index, an optional accent color (cyan/orange), a heading, and
   * pre-rendered HTML body. Used for walkthroughs (e.g. /vision Appendix A).
   *
   * Body comes via inline `body` markdown which the loader renders to
   * `html`. If absent, only heading + intro renders.
   */
  type StepItem = {
    title?: string;
    intro?: string;
    html?: string;
    color?: 'cyan' | 'orange';
  };

  let {
    block,
  }: {
    block: { items?: StepItem[] };
  } = $props();

  const items = $derived(block.items ?? []);

  function badgeClass(color?: string) {
    if (color === 'orange') return 'bg-gradient-to-br from-orange-400 to-orange-600';
    return 'bg-gradient-to-br from-pletka-primary to-pletka-secondary';
  }
</script>

<section class="max-w-3xl mx-auto space-y-12">
  {#each items as item, i}
    <div>
      <h3 class="text-xl font-bold text-gray-900 mb-4 flex items-center gap-3">
        <span
          class="w-8 h-8 {badgeClass(item.color)} text-white rounded-full flex items-center justify-center text-sm font-bold flex-shrink-0"
        >
          {i + 1}
        </span>
        <span>{item.title ?? ''}</span>
      </h3>
      {#if item.intro}
        <p class="text-gray-600 leading-relaxed mb-4">{item.intro}</p>
      {/if}
      {#if item.html}
        <article class="prose prose-slate max-w-none">
          {@html item.html}
        </article>
      {/if}
    </div>
  {/each}
</section>
